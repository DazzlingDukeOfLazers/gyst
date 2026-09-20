package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ProjectorName is the projector_state key for current_files.
const ProjectorName = "current_files"

const projectionBatch = 5000

// ProjectionStats reports what a fold did.
type ProjectionStats struct {
	Applied int
	FromSeq int64
	ToSeq   int64
}

// fileState reports whether an observation states a file's condition on
// disk, which is what current_files holds. A second claim about the same
// locator -- a parsed manifest, say -- carries no digest and must not
// overwrite the fingerprint that does.
func fileState(f LoggedFile) bool {
	if f.Kind != "file" {
		return false
	}
	switch f.ClaimType {
	case "file.metadata", "file.content_fingerprint", "artifact.absent":
		return true
	}
	return false
}

var currentFileColumns = []string{"source_id", "locator", "latest_seq", "content_digest_hex", "size_bytes",
	"observed_at", "present", "native_version_value"}

const upsertCurrentFileSuffix = `
	ON CONFLICT (source_id, locator) DO UPDATE SET
		latest_seq           = EXCLUDED.latest_seq,
		content_digest_hex   = EXCLUDED.content_digest_hex,
		size_bytes           = EXCLUDED.size_bytes,
		observed_at          = EXCLUDED.observed_at,
		present              = EXCLUDED.present,
		native_version_value = EXCLUDED.native_version_value
	WHERE current_files.latest_seq < EXCLUDED.latest_seq`

// foldBatch upserts one batch of file-state observations as a few multi-row
// statements. A multi-row upsert may not touch one key twice, so within
// the batch only the newest observation per locator is kept: the sequential
// upserts it replaces would have ended in the same state, because each one
// only ever advances.
func foldBatch(ctx context.Context, tx *tx, batch []LoggedFile) (applied int, last int64, err error) {
	latest := map[string]int{}
	var rows [][]any
	for _, f := range batch {
		last = f.Seq
		if !fileState(f) {
			continue
		}
		applied++
		row := []any{f.SourceID, f.Locator, f.Seq, f.DigestHex, f.SizeBytes, f.ObservedAt,
			f.ClaimType != "artifact.absent", f.NativeVer}
		key := f.SourceID + "\x00" + f.Locator
		if i, seen := latest[key]; seen {
			rows[i] = row
			continue
		}
		latest[key] = len(rows)
		rows = append(rows, row)
	}
	_, err = tx.insertRows(ctx, "current_files", currentFileColumns, rows, upsertCurrentFileSuffix)
	return applied, last, err
}

// ApplyCurrentFiles consumes new observations and folds them into
// current_files.
//
// The fold is idempotent in two independent ways, because one is not
// enough. Applying the same seq range twice is a no-op, since each row is
// an upsert keyed on (source_id, locator) that only advances. And an
// out-of-order or replayed observation cannot regress the projection,
// because the upsert refuses any seq below the one already recorded.
func (s *Store) ApplyCurrentFiles(ctx context.Context) (ProjectionStats, error) {
	var st ProjectionStats
	last, err := s.projectorSeq(ctx)
	if err != nil {
		return st, err
	}
	st.FromSeq = last
	for {
		batch, err := s.Since(ctx, last, projectionBatch)
		if err != nil {
			return st, err
		}
		if len(batch) == 0 {
			break
		}
		tx, err := s.db.begin(ctx)
		if err != nil {
			return st, err
		}
		applied, newLast, err := foldBatch(ctx, tx, batch)
		if err != nil {
			tx.Rollback()
			return st, err
		}
		st.Applied += applied
		last = newLast
		if _, err := tx.exec(ctx, `
			INSERT INTO projector_state (projector, last_seq, updated_at)
			VALUES ($1,$2,$3)
			ON CONFLICT (projector) DO UPDATE SET last_seq=EXCLUDED.last_seq, updated_at=EXCLUDED.updated_at`,
			ProjectorName, last, time.Now().UTC()); err != nil {
			tx.Rollback()
			return st, err
		}
		if err := tx.Commit(); err != nil {
			return st, err
		}
	}
	st.ToSeq = last
	return st, nil
}

func (s *Store) projectorSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := s.db.queryRow(ctx,
		`SELECT last_seq FROM projector_state WHERE projector=$1`, ProjectorName).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// ClearProjection drops current_files and rewinds the projector, so the
// next apply replays the whole log.
func (s *Store) ClearProjection(ctx context.Context) error {
	if _, err := s.db.exec(ctx, `DELETE FROM current_files`); err != nil {
		return err
	}
	_, err := s.db.exec(ctx, `UPDATE projector_state SET last_seq=0 WHERE projector=$1`, ProjectorName)
	return err
}

// ProjectionFingerprint hashes the whole projection into one digest, so
// two projections can be compared without diffing them row by row.
// latest_seq is deliberately excluded: two logs that arrived in a different
// order can hold the same current state, and the projection is a claim
// about state, not about arrival.
func (s *Store) ProjectionFingerprint(ctx context.Context) (string, int64, error) {
	return fingerprintRows(ctx, s.db)
}

func fingerprintRows(ctx context.Context, q querier) (string, int64, error) {
	rows, err := q.query(ctx, `
		SELECT source_id, locator, content_digest_hex, size_bytes, present
		FROM current_files ORDER BY source_id, locator`)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	h := sha256.New()
	var n int64
	for rows.Next() {
		var src, loc string
		var digest *string
		var size *int64
		var present bool
		if err := rows.Scan(&src, &loc, &digest, &size, &present); err != nil {
			return "", 0, err
		}
		fmt.Fprintf(h, "%s\x00%s\x00%v\x00%v\x00%t\n", src, loc, derefAny(digest), derefAny(size), present)
		n++
	}
	return hex.EncodeToString(h.Sum(nil)), n, rows.Err()
}

func derefAny[T any](p *T) any {
	if p == nil {
		return "-"
	}
	return *p
}

// VerifyProjection proves the projection is reproducible: fingerprint it,
// drop it, replay the entire log from seq 0, and compare. The rebuild runs
// in a transaction that is always rolled back, so running it never costs
// the live projection even if replay produces something different.
func (s *Store) VerifyProjection(ctx context.Context) (before, after string, rows int64, err error) {
	before, rows, err = s.ProjectionFingerprint(ctx)
	if err != nil {
		return
	}
	tx, err := s.db.begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err = tx.exec(ctx, `DELETE FROM current_files`); err != nil {
		return
	}
	if _, err = tx.exec(ctx, `UPDATE projector_state SET last_seq=0 WHERE projector=$1`, ProjectorName); err != nil {
		return
	}
	var last int64
	for {
		batch, e := sinceIn(ctx, tx, last, projectionBatch)
		if e != nil {
			err = e
			return
		}
		if len(batch) == 0 {
			break
		}
		if _, last, err = foldBatch(ctx, tx, batch); err != nil {
			return
		}
	}
	after, _, err = fingerprintRows(ctx, tx)
	return
}

func sinceIn(ctx context.Context, q querier, seq int64, limit int) ([]LoggedFile, error) {
	rows, err := q.query(ctx, `
		SELECT seq, source_id, locator, content_digest_hex, size_bytes, observed_at,
		       claim_type, native_version_value, subject_kind
		FROM observations WHERE seq > $1 ORDER BY seq LIMIT $2`, seq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoggedFile
	for rows.Next() {
		var f LoggedFile
		if err := rows.Scan(&f.Seq, &f.SourceID, &f.Locator, &f.DigestHex,
			&f.SizeBytes, ts(&f.ObservedAt), &f.ClaimType, &f.NativeVer, &f.Kind); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
