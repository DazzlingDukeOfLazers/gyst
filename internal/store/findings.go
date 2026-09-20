package store

import (
	"context"
	"encoding/json"
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
func (s *Store) SourceStates(ctx context.Context) ([]SourceStateRow, error) {
	rows, err := s.pool.Query(ctx, `
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
		return nil, err
	}
	defer rows.Close()
	var out []SourceStateRow
	for rows.Next() {
		var r SourceStateRow
		if err := rows.Scan(&r.SourceID, &r.LocationKind, &r.CadenceSeconds, &r.PassStatus,
			&r.PassStarted, &r.PassDetail, &r.LatestObsID); err != nil {
			return nil, err
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
// in which case it reopens.
func (s *Store) UpsertFinding(ctx context.Context, f FindingRow, now time.Time) (*string, error) {
	var prior *string
	err := s.pool.QueryRow(ctx, `
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
		f.FindingID, f.RuleID, f.RuleVersion, f.Severity, f.Subjects,
		f.Evidence, now, f.Confidence, f.Summary, nullableBytes(f.Remediation)).Scan(&prior)
	return prior, err
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// ResolveUnseenFindings resolves every finding not re-detected at now.
func (s *Store) ResolveUnseenFindings(ctx context.Context, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE findings SET status='resolved', resolved_at=$1
		WHERE status <> 'resolved' AND last_seen_at < $1`, now)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// CountOpenFindings counts open and acknowledged findings, optionally for
// one source.
func (s *Store) CountOpenFindings(ctx context.Context, sourceID string) (int, error) {
	var n int
	var err error
	if sourceID == "" {
		err = s.pool.QueryRow(ctx, `SELECT count(*) FROM findings WHERE status IN ('open','acknowledged')`).Scan(&n)
	} else {
		err = s.pool.QueryRow(ctx, `
			SELECT count(*) FROM findings f
			WHERE f.status IN ('open','acknowledged')
			  AND EXISTS (SELECT 1 FROM jsonb_array_elements(f.subjects) sj
			              WHERE sj->'location'->>'source_id' = $1)`, sourceID).Scan(&n)
	}
	return n, err
}

// AcknowledgeFinding moves an open finding to acknowledged.
func (s *Store) AcknowledgeFinding(ctx context.Context, id, by string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE findings SET status='acknowledged', disposed_by=$2, disposed_at=now()
		WHERE finding_id=$1 AND status='open'`, id, by)
	return tag.RowsAffected() > 0, err
}

// WaiveFinding records a waiver on an open or acknowledged finding.
func (s *Store) WaiveFinding(ctx context.Context, id, by string, waiver []byte) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE findings SET status='waived', waiver=$2, disposed_by=$3, disposed_at=now()
		WHERE finding_id=$1 AND status IN ('open','acknowledged')`, id, waiver, by)
	return tag.RowsAffected() > 0, err
}

const selectFinding = `
	SELECT finding_id, rule_id, rule_version, severity, status, subjects, evidence,
	       detected_at, last_seen_at, resolved_at, confidence, summary, remediation, waiver, disposed_by
	FROM findings `

// ListFindings returns findings by severity then status; with all=false,
// only open and acknowledged.
func (s *Store) ListFindings(ctx context.Context, all bool) ([]FindingRow, error) {
	rows, err := s.pool.Query(ctx, selectFinding+`
		WHERE $1 OR status IN ('open','acknowledged')
		ORDER BY CASE severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 WHEN 'low' THEN 2 ELSE 3 END,
		         status, detected_at DESC`, all)
	if err != nil {
		return nil, err
	}
	return scanFindings(rows)
}

// FindingsForFile returns non-resolved findings naming a file as a subject.
func (s *Store) FindingsForFile(ctx context.Context, sourceID, locator string) ([]FindingRow, error) {
	needle, _ := json.Marshal([]map[string]any{{"location": map[string]string{"source_id": sourceID, "locator": locator}}})
	rows, err := s.pool.Query(ctx, selectFinding+`
		WHERE status <> 'resolved' AND subjects @> $1::jsonb ORDER BY severity, finding_id`, needle)
	if err != nil {
		return nil, err
	}
	return scanFindings(rows)
}

func scanFindings(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]FindingRow, error) {
	defer rows.Close()
	var out []FindingRow
	for rows.Next() {
		var r FindingRow
		if err := rows.Scan(&r.FindingID, &r.RuleID, &r.RuleVersion, &r.Severity, &r.Status,
			&r.Subjects, &r.Evidence, &r.DetectedAt, &r.LastSeenAt, &r.ResolvedAt,
			&r.Confidence, &r.Summary, &r.Remediation, &r.Waiver, &r.DisposedBy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
