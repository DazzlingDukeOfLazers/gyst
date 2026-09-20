package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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

const upsertCurrentFile = `
	INSERT INTO current_files
		(source_id, locator, latest_seq, content_digest_hex, size_bytes,
		 observed_at, present, native_version_value)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	ON CONFLICT (source_id, locator) DO UPDATE SET
		latest_seq           = EXCLUDED.latest_seq,
		content_digest_hex   = EXCLUDED.content_digest_hex,
		size_bytes           = EXCLUDED.size_bytes,
		observed_at          = EXCLUDED.observed_at,
		present              = EXCLUDED.present,
		native_version_value = EXCLUDED.native_version_value
	WHERE current_files.latest_seq < EXCLUDED.latest_seq`

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
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return st, err
		}
		for _, f := range batch {
			if !fileState(f) {
				last = f.Seq
				continue
			}
			present := f.ClaimType != "artifact.absent"
			if _, err := tx.Exec(ctx, upsertCurrentFile,
				f.SourceID, f.Locator, f.Seq, f.DigestHex, f.SizeBytes, f.ObservedAt, present, f.NativeVer); err != nil {
				tx.Rollback(ctx)
				return st, err
			}
			last = f.Seq
			st.Applied++
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO projector_state (projector, last_seq, updated_at)
			VALUES ($1,$2,$3)
			ON CONFLICT (projector) DO UPDATE SET last_seq=EXCLUDED.last_seq, updated_at=EXCLUDED.updated_at`,
			ProjectorName, last, time.Now().UTC()); err != nil {
			tx.Rollback(ctx)
			return st, err
		}
		if err := tx.Commit(ctx); err != nil {
			return st, err
		}
	}
	st.ToSeq = last
	return st, nil
}

func (s *Store) projectorSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := s.pool.QueryRow(ctx,
		`SELECT last_seq FROM projector_state WHERE projector=$1`, ProjectorName).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// ClearProjection drops current_files and rewinds the projector, so the
// next apply replays the whole log.
func (s *Store) ClearProjection(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM current_files`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE projector_state SET last_seq=0 WHERE projector=$1`, ProjectorName)
	return err
}

// ProjectionFingerprint hashes the whole projection into one digest, so
// two projections can be compared without diffing them row by row.
// latest_seq is deliberately excluded: two logs that arrived in a different
// order can hold the same current state, and the projection is a claim
// about state, not about arrival.
func (s *Store) ProjectionFingerprint(ctx context.Context) (string, int64, error) {
	return fingerprintRows(ctx, s.pool)
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func fingerprintRows(ctx context.Context, q querier) (string, int64, error) {
	rows, err := q.Query(ctx, `
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM current_files`); err != nil {
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE projector_state SET last_seq=0 WHERE projector=$1`, ProjectorName); err != nil {
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
		for _, f := range batch {
			if !fileState(f) {
				last = f.Seq
				continue
			}
			present := f.ClaimType != "artifact.absent"
			if _, e := tx.Exec(ctx, upsertCurrentFile,
				f.SourceID, f.Locator, f.Seq, f.DigestHex, f.SizeBytes, f.ObservedAt, present, f.NativeVer); e != nil {
				err = e
				return
			}
			last = f.Seq
		}
	}
	after, _, err = fingerprintRows(ctx, tx)
	return
}

func sinceIn(ctx context.Context, q querier, seq int64, limit int) ([]LoggedFile, error) {
	rows, err := q.Query(ctx, `
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
			&f.SizeBytes, &f.ObservedAt, &f.ClaimType, &f.NativeVer, &f.Kind); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
