package findings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
	"github.com/jackc/pgx/v5"
)

// Stats reports what a detection pass did.
type Stats struct {
	Detected, New, Reopened, Resolved int
	Open                              int // open + acknowledged after the pass
}

// Project loads the inputs, runs every rule, and reconciles the result with
// what is already recorded. New findings open; re-detected ones keep their
// disposition unless a waiver has expired; findings no longer produced are
// resolved with a timestamp, never deleted.
func Project(ctx context.Context, s *store.Store, now time.Time) (Stats, error) {
	var st Stats
	in, err := load(ctx, s, now)
	if err != nil {
		return st, err
	}
	found := Detect(in)
	st.Detected = len(found)

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return st, err
	}
	defer tx.Rollback(ctx)

	for _, f := range found {
		subjects, _ := json.Marshal(f.Subjects)
		var remediation []byte
		if f.Remediation != nil {
			remediation, _ = json.Marshal(f.Remediation)
		}
		// The CTE reads the prior status before the insert touches the row,
		// so a new finding reports NULL and a re-detected one its old status.
		var prior *string
		err := tx.QueryRow(ctx, `
			WITH prior AS (SELECT status FROM findings WHERE finding_id = $1)
			INSERT INTO findings (finding_id, rule_id, rule_version, severity, status, subjects,
				evidence, detected_at, last_seen_at, confidence, summary, remediation)
			VALUES ($1,$2,$3,$4,'open',$5,$6,$7,$7,$8,$9,$10)
			ON CONFLICT (finding_id) DO UPDATE SET
				last_seen_at = EXCLUDED.last_seen_at,
				severity     = EXCLUDED.severity,
				subjects     = EXCLUDED.subjects,
				evidence     = EXCLUDED.evidence,
				confidence   = EXCLUDED.confidence,
				summary      = EXCLUDED.summary,
				remediation  = EXCLUDED.remediation,
				resolved_at  = NULL,
				status = CASE
					WHEN findings.status = 'resolved' THEN 'open'
					WHEN findings.status = 'waived'
					     AND findings.waiver ? 'expires_at'
					     AND (findings.waiver->>'expires_at')::timestamptz < EXCLUDED.last_seen_at
					THEN 'open'
					ELSE findings.status END
			RETURNING (SELECT status FROM prior)`,
			f.FindingID, f.Rule.ID, f.Rule.Version, f.Severity, subjects,
			f.Evidence, now, f.Confidence, f.Summary, nullableJSON(remediation)).Scan(&prior)
		if err != nil {
			return st, err
		}
		switch {
		case prior == nil:
			st.New++
		case *prior == StatusResolved:
			st.Reopened++
		}
	}
	// Anything not produced by this pass is no longer a finding.
	tag, err := tx.Exec(ctx, `
		UPDATE findings SET status='resolved', resolved_at=$1
		WHERE status <> 'resolved' AND last_seen_at < $1`, now)
	if err != nil {
		return st, err
	}
	st.Resolved = int(tag.RowsAffected())
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM findings WHERE status IN ('open','acknowledged')`).Scan(&st.Open); err != nil {
		return st, err
	}
	if err := tx.Commit(ctx); err != nil {
		return st, err
	}
	return st, nil
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func load(ctx context.Context, s *store.Store, now time.Time) (Inputs, error) {
	in := Inputs{Now: now}

	rows, err := s.Pool().Query(ctx, `
		SELECT cf.source_id, cf.locator, coalesce(cf.content_digest_hex,''), cf.native_version_value,
		       coalesce(cf.size_bytes,0), o.observation_id
		FROM current_files cf JOIN observations o ON o.seq = cf.latest_seq
		WHERE cf.present ORDER BY cf.source_id, cf.locator`)
	if err != nil {
		return in, err
	}
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.Digest, &f.NativeVersion, &f.Size, &f.ObsID); err != nil {
			rows.Close()
			return in, err
		}
		in.Files = append(in.Files, f)
	}
	rows.Close()

	// Ambiguities: compare-set-with relations whose gone side is still
	// absent and whose candidates are still present. One group per gone
	// file. The candidate ordering is by locator so the finding is stable.
	rows, err = s.Pool().Query(ctx, `
		SELECT r.to_source, r.to_locator, t.native_version_value, t.observation_id,
		       r.from_source, r.from_locator, cf.native_version_value, coalesce(cf.size_bytes,0),
		       coalesce(cf.content_digest_hex,''), o.observation_id, r.evidence, r.confidence
		FROM relations r
		JOIN current_files gone ON gone.source_id=r.to_source AND gone.locator=r.to_locator AND NOT gone.present
		JOIN observations t ON t.seq = gone.latest_seq
		JOIN current_files cf ON cf.source_id=r.from_source AND cf.locator=r.from_locator AND cf.present
		JOIN observations o ON o.seq = cf.latest_seq
		WHERE r.type = 'compare-set-with'
		ORDER BY r.to_source, r.to_locator, r.from_source, r.from_locator`)
	if err != nil {
		return in, err
	}
	var cur *Ambiguity
	for rows.Next() {
		var gone, cand File
		var evidence []string
		var conf float64
		if err := rows.Scan(&gone.SourceID, &gone.Locator, &gone.NativeVersion, &gone.ObsID,
			&cand.SourceID, &cand.Locator, &cand.NativeVersion, &cand.Size, &cand.Digest, &cand.ObsID,
			&evidence, &conf); err != nil {
			rows.Close()
			return in, err
		}
		if cur == nil || cur.Gone.SourceID != gone.SourceID || cur.Gone.Locator != gone.Locator {
			in.Ambiguities = append(in.Ambiguities, Ambiguity{Gone: gone, Confidence: conf})
			cur = &in.Ambiguities[len(in.Ambiguities)-1]
		}
		cur.Candidates = append(cur.Candidates, cand)
		cur.Evidence = appendUnique(cur.Evidence, evidence...)
	}
	rows.Close()

	rows, err = s.Pool().Query(ctx, `
		SELECT o.source_id, o.locator, cf.native_version_value, coalesce(cf.size_bytes,0),
		       o.observation_id, coalesce(o.claim_payload->>'error','')
		FROM (
			SELECT DISTINCT ON (source_id, locator) source_id, locator, observation_id, claim_payload
			FROM observations WHERE claim_type='project.manifest'
			ORDER BY source_id, locator, seq DESC
		) o
		JOIN current_files cf ON cf.source_id=o.source_id AND cf.locator=o.locator AND cf.present
		WHERE (o.claim_payload->>'valid')::boolean = false
		ORDER BY o.source_id, o.locator`)
	if err != nil {
		return in, err
	}
	for rows.Next() {
		var m BadManifest
		if err := rows.Scan(&m.File.SourceID, &m.File.Locator, &m.File.NativeVersion, &m.File.Size,
			&m.File.ObsID, &m.Error); err != nil {
			rows.Close()
			return in, err
		}
		in.Manifests = append(in.Manifests, m)
	}
	rows.Close()

	rows, err = s.Pool().Query(ctx, `
		SELECT src.source_id, src.location_kind, src.cadence_seconds,
		       coalesce(p.status,''), coalesce(p.started_at, to_timestamp(0)), coalesce(p.detail,''),
		       coalesce(latest.observation_id,'')
		FROM sources src
		LEFT JOIN LATERAL (
			SELECT status, started_at, detail FROM scan_passes sp
			WHERE sp.source_id = src.source_id ORDER BY sp.started_at DESC LIMIT 1) p ON true
		LEFT JOIN LATERAL (
			SELECT observation_id FROM observations o
			WHERE o.source_id = src.source_id ORDER BY o.seq DESC LIMIT 1) latest ON true
		ORDER BY src.source_id`)
	if err != nil {
		return in, err
	}
	defer rows.Close()
	for rows.Next() {
		var ss SourceState
		var cadence int
		if err := rows.Scan(&ss.SourceID, &ss.LocationKind, &cadence, &ss.PassStatus,
			&ss.PassStarted, &ss.PassDetail, &ss.LatestObsID); err != nil {
			return in, err
		}
		ss.Cadence = time.Duration(cadence) * time.Second
		in.Sources = append(in.Sources, ss)
	}
	return in, rows.Err()
}

func appendUnique(dst []string, xs ...string) []string {
	seen := map[string]bool{}
	for _, d := range dst {
		seen[d] = true
	}
	for _, x := range xs {
		if !seen[x] {
			dst = append(dst, x)
			seen[x] = true
		}
	}
	return dst
}

// Acknowledge records that a person has seen a finding.
func Acknowledge(ctx context.Context, s *store.Store, id, by string) error {
	if by == "" {
		return fmt.Errorf("--by is required: an acknowledgement is a person's act")
	}
	tag, err := s.Pool().Exec(ctx, `
		UPDATE findings SET status='acknowledged', disposed_by=$2, disposed_at=now()
		WHERE finding_id=$1 AND status='open'`, id, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no open finding %s", id)
	}
	return nil
}

// Waive records a person's decision that a finding does not need action.
// Only a person may do this; the schema and this function both refuse a
// rule or any other actor kind.
func Waive(ctx context.Context, s *store.Store, id string, w Waiver) error {
	if w.Actor.Kind != "user" {
		return fmt.Errorf("only a person may waive a finding; actor kind %q refused", w.Actor.Kind)
	}
	if w.Actor.ID == "" || w.Reason == "" {
		return fmt.Errorf("a waiver needs who (--by) and why (--reason)")
	}
	body, _ := json.Marshal(w)
	tag, err := s.Pool().Exec(ctx, `
		UPDATE findings SET status='waived', waiver=$2, disposed_by=$3, disposed_at=now()
		WHERE finding_id=$1 AND status IN ('open','acknowledged')`, id, body, w.Actor.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no open or acknowledged finding %s", id)
	}
	return nil
}

// Row is a finding as read back, with its bookkeeping.
type Row struct {
	Finding
	LastSeenAt time.Time
	ResolvedAt *time.Time
	DisposedBy string
}

// List returns findings. With all=false, only open and acknowledged.
func List(ctx context.Context, s *store.Store, all bool) ([]Row, error) {
	return query(ctx, s, `
		SELECT finding_id, rule_id, rule_version, severity, status, subjects, evidence,
		       detected_at, last_seen_at, resolved_at, confidence, summary, remediation, waiver, disposed_by
		FROM findings
		WHERE $1 OR status IN ('open','acknowledged')
		ORDER BY CASE severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END,
		         status, detected_at DESC`, all)
}

// ForFile returns every non-resolved finding that names a file as a subject.
func ForFile(ctx context.Context, s *store.Store, sourceID, locator string) ([]Row, error) {
	needle, _ := json.Marshal([]map[string]any{{"location": map[string]string{"source_id": sourceID, "locator": locator}}})
	return query(ctx, s, `
		SELECT finding_id, rule_id, rule_version, severity, status, subjects, evidence,
		       detected_at, last_seen_at, resolved_at, confidence, summary, remediation, waiver, disposed_by
		FROM findings WHERE status <> 'resolved' AND subjects @> $1::jsonb
		ORDER BY severity, finding_id`, needle)
}

func query(ctx context.Context, s *store.Store, sql string, args ...any) ([]Row, error) {
	rows, err := s.Pool().Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var subjects, remediation, waiver []byte
		if err := rows.Scan(&r.FindingID, &r.Rule.ID, &r.Rule.Version, &r.Severity, &r.Status,
			&subjects, &r.Evidence, &r.DetectedAt, &r.LastSeenAt, &r.ResolvedAt,
			&r.Confidence, &r.Summary, &remediation, &waiver, &r.DisposedBy); err != nil {
			return nil, err
		}
		r.SchemaVersion = observe.SchemaVersion
		if err := json.Unmarshal(subjects, &r.Subjects); err != nil {
			return nil, err
		}
		if len(remediation) > 0 {
			r.Remediation = &Remediation{}
			_ = json.Unmarshal(remediation, r.Remediation)
		}
		if len(waiver) > 0 {
			r.Waiver = &Waiver{}
			_ = json.Unmarshal(waiver, r.Waiver)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	return out, nil
}
