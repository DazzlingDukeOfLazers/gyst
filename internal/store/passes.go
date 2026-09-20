package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"
)

// Pass statuses. See migrations/0005_scan_passes.sql for their meaning.
const (
	PassRunning     = "running"
	PassComplete    = "complete"
	PassPartial     = "partial"
	PassInterrupted = "interrupted"
	PassUnavailable = "unavailable"
)

// PassStart is what is known about a pass before it does anything.
type PassStart struct {
	SourceID  string
	Connector string
	StartedAt time.Time
	Resumed   bool
}

// PassResult is what a pass reports when it ends. A pass that never reports
// one is what "interrupted" means.
type PassResult struct {
	Status string
	Detail string

	Scanned, Unchanged, Skipped, Ignored, Unstable int
	Bytes, HashedBytes                             int64
	Appended                                       int

	AbsenceChecked bool
	AbsenceReason  string
}

// Pass is a recorded pass as read back for display.
type Pass struct {
	PassID     string
	SourceID   string
	Connector  string
	Kind       string // source kind, joined from sources
	Root       string
	Location   string // location kind/provider, joined from sources
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string
	Detail     string
	Resumed    bool
	Scanned    int
	Unchanged  int
	Skipped    int
	Appended   int
}

// PassID derives a pass identifier from the source and its clock reading.
// Deterministic so a retried begin cannot create a second row for one pass.
func PassID(sourceID string, startedAt time.Time) string {
	h := sha256.Sum256([]byte(sourceID + "\x00" + startedAt.UTC().Format(time.RFC3339Nano)))
	return "pass_" + hex.EncodeToString(h[:])[:24]
}

// BeginPass records that a pass has started and returns its id.
//
// Any earlier pass on the same source still marked running is concluded to be
// interrupted: a scanner starting is the moment we learn the previous one is
// not going to finish. Two genuinely concurrent scanners on one source would
// misreport each other, and that is accepted rather than guarded; the log
// itself is safe under concurrency, this table is only bookkeeping.
func (s *Store) BeginPass(ctx context.Context, p PassStart) (string, error) {
	id := PassID(p.SourceID, p.StartedAt)
	tx, err := s.db.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if _, err := tx.exec(ctx, `
		UPDATE scan_passes
		SET status=$2, finished_at=$4,
		    detail='a later pass on this source began before this one reported a result'
		WHERE source_id=$1 AND status=$3`,
		p.SourceID, PassInterrupted, PassRunning, time.Now().UTC()); err != nil {
		return "", err
	}
	if _, err := tx.exec(ctx, `
		INSERT INTO scan_passes (pass_id, source_id, connector, started_at, status, resumed)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (pass_id) DO NOTHING`,
		id, p.SourceID, p.Connector, p.StartedAt, PassRunning, p.Resumed); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// FinishPass records how a pass ended.
func (s *Store) FinishPass(ctx context.Context, passID string, r PassResult) error {
	_, err := s.db.exec(ctx, `
		UPDATE scan_passes SET
			finished_at=$14, status=$2, detail=$3,
			scanned=$4, unchanged=$5, skipped=$6, ignored=$7, unstable=$8,
			bytes=$9, hashed_bytes=$10, appended=$11,
			absence_checked=$12, absence_reason=$13
		WHERE pass_id=$1`,
		passID, r.Status, r.Detail,
		r.Scanned, r.Unchanged, r.Skipped, r.Ignored, r.Unstable,
		r.Bytes, r.HashedBytes, r.Appended,
		r.AbsenceChecked, r.AbsenceReason, time.Now().UTC())
	return err
}

// LatestPasses returns the most recent pass for every registered source. A
// source with no pass at all is returned with an empty Status: "never
// scanned" is a state the report needs, not an absent row.
func (s *Store) LatestPasses(ctx context.Context) ([]Pass, error) {
	rows, err := s.db.query(ctx, `
		SELECT src.source_id, src.kind, src.root,
		       src.location_kind || CASE WHEN src.location_provider='' THEN '' ELSE '/' || src.location_provider END,
		       coalesce(p.pass_id,''), coalesce(p.connector,''),
		       p.started_at, p.finished_at,
		       coalesce(p.status,''), coalesce(p.detail,''), coalesce(p.resumed,false),
		       coalesce(p.scanned,0), coalesce(p.unchanged,0), coalesce(p.skipped,0),
		       coalesce(p.appended,0)
		FROM sources src
		LEFT JOIN scan_passes p ON p.source_id = src.source_id
		     AND p.started_at = (SELECT max(sp.started_at) FROM scan_passes sp WHERE sp.source_id = src.source_id)
		ORDER BY src.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Pass
	for rows.Next() {
		var p Pass
		var started *time.Time
		if err := rows.Scan(&p.SourceID, &p.Kind, &p.Root, &p.Location,
			&p.PassID, &p.Connector, tsp(&started), tsp(&p.FinishedAt),
			&p.Status, &p.Detail, &p.Resumed,
			&p.Scanned, &p.Unchanged, &p.Skipped, &p.Appended); err != nil {
			return nil, err
		}
		if started != nil {
			p.StartedAt = *started
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return out, nil
}
