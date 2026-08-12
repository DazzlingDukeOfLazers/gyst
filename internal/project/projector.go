// Package project rebuilds query models from the observation log.
//
// Every table this package writes is disposable. Drop it, reset the projector
// cursor to zero, replay, and the result must be identical -- that equivalence
// is the day 2 exit criterion, and Verify below is what checks it.
package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
	"github.com/jackc/pgx/v5"
)

const Name = "current_files"

const batchSize = 5000

type Stats struct {
	Applied int
	FromSeq int64
	ToSeq   int64
}

// Apply consumes new observations and folds them into current_files.
//
// The fold is idempotent in two independent ways, because one is not enough.
// Applying the same seq range twice is a no-op, since each row is an upsert
// keyed on (source_id, locator) that only advances. And an out-of-order or
// replayed observation cannot regress the projection, because the WHERE clause
// on the upsert refuses any seq below the one already recorded.
func Apply(ctx context.Context, s *store.Store) (Stats, error) {
	var st Stats
	last, err := lastSeq(ctx, s)
	if err != nil {
		return st, err
	}
	st.FromSeq = last

	for {
		batch, err := s.Since(ctx, last, batchSize)
		if err != nil {
			return st, err
		}
		if len(batch) == 0 {
			break
		}
		tx, err := s.Pool().Begin(ctx)
		if err != nil {
			return st, err
		}
		for _, f := range batch {
			present := f.ClaimType != "artifact.absent"
			_, err := tx.Exec(ctx, `
				INSERT INTO current_files
					(source_id, locator, latest_seq, content_digest_hex, size_bytes,
					 observed_at, present, native_version_value)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (source_id, locator) DO UPDATE SET
					latest_seq         = EXCLUDED.latest_seq,
					content_digest_hex = EXCLUDED.content_digest_hex,
					size_bytes         = EXCLUDED.size_bytes,
					observed_at          = EXCLUDED.observed_at,
					present              = EXCLUDED.present,
					native_version_value = EXCLUDED.native_version_value
				WHERE current_files.latest_seq < EXCLUDED.latest_seq`,
				f.SourceID, f.Locator, f.Seq, f.DigestHex, f.SizeBytes, f.ObservedAt, present, f.NativeVer)
			if err != nil {
				tx.Rollback(ctx)
				return st, err
			}
			last = f.Seq
			st.Applied++
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO projector_state (projector, last_seq, updated_at)
			VALUES ($1,$2,now())
			ON CONFLICT (projector) DO UPDATE SET last_seq=EXCLUDED.last_seq, updated_at=now()`,
			Name, last); err != nil {
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

func lastSeq(ctx context.Context, s *store.Store) (int64, error) {
	var seq int64
	err := s.Pool().QueryRow(ctx,
		`SELECT last_seq FROM projector_state WHERE projector=$1`, Name).Scan(&seq)
	if err != nil && err.Error() == "no rows in result set" {
		return 0, nil
	}
	if err != nil {
		return 0, nil
	}
	return seq, nil
}

// Fingerprint hashes the whole projection into one digest, so two projections
// can be compared without diffing them row by row.
func Fingerprint(ctx context.Context, s *store.Store) (string, int64, error) {
	rows, err := s.Pool().Query(ctx, `
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
		// latest_seq is deliberately excluded. Two logs that arrived in a
		// different order can hold the same current state, and the projection
		// is a claim about state, not about arrival.
		fmt.Fprintf(h, "%s\x00%s\x00%v\x00%v\x00%t\n", src, loc, deref(digest), deref(size), present)
		n++
	}
	return hex.EncodeToString(h.Sum(nil)), n, rows.Err()
}

func deref[T any](p *T) any {
	if p == nil {
		return "-"
	}
	return *p
}

// Verify proves the projection is reproducible: fingerprint it, drop it, replay
// the entire log from seq 0, and compare.
//
// It rebuilds in a transaction that is always rolled back, so running it never
// costs the live projection even if replay produces something different.
func Verify(ctx context.Context, s *store.Store) (before, after string, rows int64, err error) {
	before, rows, err = Fingerprint(ctx, s)
	if err != nil {
		return
	}

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `DELETE FROM current_files`); err != nil {
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE projector_state SET last_seq=0 WHERE projector=$1`, Name); err != nil {
		return
	}

	var last int64
	for {
		batch, e := txSince(ctx, tx, last, batchSize)
		if e != nil {
			err = e
			return
		}
		if len(batch) == 0 {
			break
		}
		for _, f := range batch {
			present := f.ClaimType != "artifact.absent"
			if _, e := tx.Exec(ctx, `
				INSERT INTO current_files
					(source_id, locator, latest_seq, content_digest_hex, size_bytes,
					 observed_at, present, native_version_value)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT (source_id, locator) DO UPDATE SET
					latest_seq         = EXCLUDED.latest_seq,
					content_digest_hex = EXCLUDED.content_digest_hex,
					size_bytes         = EXCLUDED.size_bytes,
					observed_at          = EXCLUDED.observed_at,
					present              = EXCLUDED.present,
					native_version_value = EXCLUDED.native_version_value
				WHERE current_files.latest_seq < EXCLUDED.latest_seq`,
				f.SourceID, f.Locator, f.Seq, f.DigestHex, f.SizeBytes, f.ObservedAt, present, f.NativeVer); e != nil {
				err = e
				return
			}
			last = f.Seq
		}
	}

	h := sha256.New()
	rebuilt, e := tx.Query(ctx, `
		SELECT source_id, locator, content_digest_hex, size_bytes, present
		FROM current_files ORDER BY source_id, locator`)
	if e != nil {
		err = e
		return
	}
	for rebuilt.Next() {
		var src, loc string
		var digest *string
		var size *int64
		var present bool
		if e := rebuilt.Scan(&src, &loc, &digest, &size, &present); e != nil {
			rebuilt.Close()
			err = e
			return
		}
		fmt.Fprintf(h, "%s\x00%s\x00%v\x00%v\x00%t\n", src, loc, deref(digest), deref(size), present)
	}
	rebuilt.Close()
	after = hex.EncodeToString(h.Sum(nil))
	return
}

func txSince(ctx context.Context, tx pgx.Tx, seq int64, limit int) ([]store.LoggedFile, error) {
	rows, err := tx.Query(ctx, `
		SELECT seq, source_id, locator, content_digest_hex, size_bytes, observed_at,
		       claim_type, native_version_value
		FROM observations WHERE seq > $1 ORDER BY seq LIMIT $2`, seq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.LoggedFile
	for rows.Next() {
		var f store.LoggedFile
		if err := rows.Scan(&f.Seq, &f.SourceID, &f.Locator, &f.DigestHex,
			&f.SizeBytes, &f.ObservedAt, &f.ClaimType, &f.NativeVer); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
