package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// SourceStateRow is what the latest pass says about a source, with the
// newest observation from it for a finding to cite.
type SourceStateRow struct {
	SourceID       string
	LocationKind   string
	CadenceSeconds int
	PassStatus     string
	PassStarted    time.Time
	PassDetail     string
	LatestObsID    string
}

// SourceStates lists every source with its latest pass and observation.
// Correlated subqueries rather than LATERAL joins, so the spelling is the
// same on every engine. A source never scanned has a zero PassStarted.
func (s *Store) SourceStates(ctx context.Context) ([]SourceStateRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT src.source_id, src.location_kind, src.cadence_seconds,
		       coalesce(p.status,''), p.started_at, coalesce(p.detail,''),
		       coalesce((SELECT o.observation_id FROM observations o
		                 WHERE o.source_id = src.source_id ORDER BY o.seq DESC LIMIT 1), '')
		FROM sources src
		LEFT JOIN scan_passes p ON p.source_id = src.source_id
		     AND p.started_at = (SELECT max(sp.started_at) FROM scan_passes sp WHERE sp.source_id = src.source_id)
		ORDER BY src.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceStateRow
	for rows.Next() {
		var r SourceStateRow
		var started *time.Time
		if err := rows.Scan(&r.SourceID, &r.LocationKind, &r.CadenceSeconds, &r.PassStatus,
			tsp(&started), &r.PassDetail, &r.LatestObsID); err != nil {
			return nil, err
		}
		if started != nil {
			r.PassStarted = *started
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FindingRow mirrors the findings table. JSON columns stay as bytes; the
// findings package owns their shape.
type FindingRow struct {
	FindingID   string
	RuleID      string
	RuleVersion string
	Severity    string
	Status      string
	Subjects    []byte
	Evidence    []string
	DetectedAt  time.Time
	LastSeenAt  time.Time
	ResolvedAt  *time.Time
	Confidence  float64
	Summary     string
	Remediation []byte
	Waiver      []byte
	DisposedBy  string
}

// UpsertFinding records a detection. It returns the finding's status
// before this call, or nil for a brand-new finding. A re-detected finding
// keeps its disposition unless it was resolved, or its waiver has expired,
// in which case it reopens. The decision is made here in Go from the prior
// row, inside one transaction, rather than in a dialect-specific CASE.
func (s *Store) UpsertFinding(ctx context.Context, f FindingRow, now time.Time) (*string, error) {
	tx, err := s.db.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var prior *string
	var priorStatus string
	var waiver []byte
	err = tx.queryRow(ctx, `SELECT status, waiver FROM findings WHERE finding_id=$1`, f.FindingID).
		Scan(&priorStatus, &waiver)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// new
	case err != nil:
		return nil, err
	default:
		p := priorStatus
		prior = &p
	}

	status := "open"
	if prior != nil {
		status = *prior
		switch {
		case status == "resolved":
			status = "open"
		case status == "waived" && waiverExpired(waiver, now):
			status = "open"
		}
	}
	// The inserted row always says open. A proposed row saying waived with
	// no waiver would fail the check constraint before the conflict branch
	// ran, on either engine; the computed status is applied only to the
	// existing row, which carries its waiver.
	if _, err := tx.exec(ctx, `
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
			status       = $11`,
		f.FindingID, f.RuleID, f.RuleVersion, f.Severity, f.Subjects,
		f.Evidence, now, f.Confidence, f.Summary, nullableBytes(f.Remediation), status); err != nil {
		return nil, err
	}
	return prior, tx.Commit()
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// waiverExpired reads expires_at from a waiver record, if present.
func waiverExpired(waiver []byte, now time.Time) bool {
	if len(waiver) == 0 {
		return false
	}
	var w struct {
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if json.Unmarshal(waiver, &w) != nil || w.ExpiresAt == nil {
		return false
	}
	return w.ExpiresAt.Before(now)
}

// ResolveUnseenFindings resolves every finding not re-detected at now.
func (s *Store) ResolveUnseenFindings(ctx context.Context, now time.Time) (int, error) {
	tag, err := s.db.exec(ctx, `
		UPDATE findings SET status='resolved', resolved_at=$1
		WHERE status <> 'resolved' AND last_seen_at < $1`, now)
	if err != nil {
		return 0, err
	}
	return int(affected(tag)), nil
}

// CountOpenFindings counts open and acknowledged findings, optionally for
// one source. The per-source count walks subjects in Go: findings are few
// and a JSON containment operator is not portable.
func (s *Store) CountOpenFindings(ctx context.Context, sourceID string) (int, error) {
	if sourceID == "" {
		var n int
		err := s.db.queryRow(ctx, `SELECT count(*) FROM findings WHERE status IN ('open','acknowledged')`).Scan(&n)
		return n, err
	}
	rows, err := s.ListFindings(ctx, false)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if subjectsName(r.Subjects, sourceID, "") {
			n++
		}
	}
	return n, nil
}

// subjectsName reports whether a subjects array names a source, and a
// locator when one is given.
func subjectsName(subjects []byte, sourceID, locator string) bool {
	var subs []struct {
		Location struct {
			SourceID string `json:"source_id"`
			Locator  string `json:"locator"`
		} `json:"location"`
	}
	if json.Unmarshal(subjects, &subs) != nil {
		return false
	}
	for _, sj := range subs {
		if sj.Location.SourceID == sourceID && (locator == "" || sj.Location.Locator == locator) {
			return true
		}
	}
	return false
}

// AcknowledgeFinding moves an open finding to acknowledged.
func (s *Store) AcknowledgeFinding(ctx context.Context, id, by string) (bool, error) {
	tag, err := s.db.exec(ctx, `
		UPDATE findings SET status='acknowledged', disposed_by=$2, disposed_at=$3
		WHERE finding_id=$1 AND status='open'`, id, by, time.Now().UTC())
	return affected(tag) > 0, err
}

// WaiveFinding records a waiver on an open or acknowledged finding.
func (s *Store) WaiveFinding(ctx context.Context, id, by string, waiver []byte) (bool, error) {
	tag, err := s.db.exec(ctx, `
		UPDATE findings SET status='waived', waiver=$2, disposed_by=$3, disposed_at=$4
		WHERE finding_id=$1 AND status IN ('open','acknowledged')`, id, waiver, by, time.Now().UTC())
	return affected(tag) > 0, err
}

const selectFinding = `
	SELECT finding_id, rule_id, rule_version, severity, status, subjects, evidence,
	       detected_at, last_seen_at, resolved_at, confidence, summary, remediation, waiver, disposed_by
	FROM findings `

// ListFindings returns findings by severity then status; with all=false,
// only open and acknowledged.
func (s *Store) ListFindings(ctx context.Context, all bool) ([]FindingRow, error) {
	rows, err := s.db.query(ctx, selectFinding+`
		WHERE $1 OR status IN ('open','acknowledged')
		ORDER BY CASE severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END,
		         status, detected_at DESC`, all)
	if err != nil {
		return nil, err
	}
	return scanFindings(rows)
}

// FindingsForFile returns non-resolved findings naming a file as a subject,
// filtered in Go for the same reason as CountOpenFindings.
func (s *Store) FindingsForFile(ctx context.Context, sourceID, locator string) ([]FindingRow, error) {
	rows, err := s.db.query(ctx, selectFinding+`WHERE status <> 'resolved' ORDER BY severity, finding_id`)
	if err != nil {
		return nil, err
	}
	all, err := scanFindings(rows)
	if err != nil {
		return nil, err
	}
	var out []FindingRow
	for _, r := range all {
		if subjectsName(r.Subjects, sourceID, locator) {
			out = append(out, r)
		}
	}
	return out, nil
}

func scanFindings(rows *sql.Rows) ([]FindingRow, error) {
	defer rows.Close()
	var out []FindingRow
	for rows.Next() {
		var r FindingRow
		if err := rows.Scan(&r.FindingID, &r.RuleID, &r.RuleVersion, &r.Severity, &r.Status,
			&r.Subjects, jsl(&r.Evidence), ts(&r.DetectedAt), ts(&r.LastSeenAt), tsp(&r.ResolvedAt),
			&r.Confidence, &r.Summary, &r.Remediation, &r.Waiver, &r.DisposedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
