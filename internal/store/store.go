// Package store is the append-only observation log and its cursor bookkeeping.
//
// Nothing here updates or deletes an observation. The database enforces that
// with triggers, so a bug in this package surfaces as an error rather than as
// quietly rewritten history.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func DSN() string {
	if v := os.Getenv("GYST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres:///gyst"
}

func Open(ctx context.Context) (*Store, error) {
	pool, err := pgxpool.New(ctx, DSN())
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect %s: %w", DSN(), err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Append writes observations, ignoring any whose id is already present.
//
// ON CONFLICT is a safety net, not the idempotency mechanism. Unchanged files
// are filtered against KnownState before an observation is ever built, so in
// normal operation nothing reaches here that would collide. Relying on the
// collision instead was the earlier design, and it silently swallowed a revert
// to a byte-identical earlier state -- see observe.DeriveID.
//
// What the clause still buys: two agents scanning the same source concurrently,
// or a retried batch after a partial failure, cannot double-append.
func (s *Store) Append(ctx context.Context, obs []observe.Observation) (inserted int, err error) {
	if len(obs) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for i := range obs {
		o := &obs[i]
		var digestAlgo, digestHex *string
		var size *int64
		if o.Subject.Version != nil {
			size = &o.Subject.Version.SizeBytes
			if d := o.Subject.Version.ContentDigest; d != nil {
				digestAlgo, digestHex = &d.Algo, &d.Hex
			}
		}
		payload, _ := json.Marshal(o.Claim.Payload)
		extractor, _ := json.Marshal(o.Extractor)
		policy, _ := json.Marshal(o.Policy)
		visibility, _ := json.Marshal(o.Visibility)

		batch.Queue(`
			INSERT INTO observations (
				observation_id, schema_version, source_id, connector, connector_version,
				cursor, observed_at, subject_kind, locator,
				native_version_scheme, native_version_value,
				content_digest_algo, content_digest_hex, size_bytes,
				claim_type, claim_payload, extractor, policy, visibility, corrects)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
			ON CONFLICT (observation_id) DO NOTHING`,
			o.ObservationID, o.SchemaVersion, o.Source.SourceID, o.Source.Connector,
			o.Source.ConnectorVersion, nullable(o.Source.Cursor), o.ObservedAt,
			o.Subject.Kind, o.Subject.Location.Locator,
			o.Subject.Location.NativeVersion.Scheme, o.Subject.Location.NativeVersion.Value,
			digestAlgo, digestHex, size,
			o.Claim.Type, payload, extractor, policy, visibility, nullable(o.Corrects),
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range obs {
		tag, e := results.Exec()
		if e != nil {
			return inserted, e
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) Cursor(ctx context.Context, sourceID string) (string, error) {
	var c string
	err := s.pool.QueryRow(ctx,
		`SELECT cursor FROM source_cursors WHERE source_id=$1`, sourceID).Scan(&c)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return c, err
}

func (s *Store) SetCursor(ctx context.Context, sourceID, cursor string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO source_cursors (source_id, cursor, updated_at)
		VALUES ($1,$2,now())
		ON CONFLICT (source_id) DO UPDATE SET cursor=EXCLUDED.cursor, updated_at=now()`,
		sourceID, cursor)
	return err
}

// KnownState loads what the projection believes about every locator in a
// source, as one query rather than a lookup per file.
//
// A scan of 100k files must not become 100k round trips, and the map is small:
// a locator and two scalars per row.
func (s *Store) KnownState(ctx context.Context, sourceID string) (map[string]observe.KnownState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT locator, native_version_value, latest_seq
		FROM current_files WHERE source_id=$1 AND present`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]observe.KnownState, 1024)
	for rows.Next() {
		var loc string
		var st observe.KnownState
		if err := rows.Scan(&loc, &st.NativeVersion, &st.Seq); err != nil {
			return nil, err
		}
		out[loc] = st
	}
	return out, rows.Err()
}

func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM observations`).Scan(&n)
	return n, err
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// LoggedFile is one observation as the projector consumes it.
type LoggedFile struct {
	Seq        int64
	SourceID   string
	Locator    string
	DigestHex  *string
	SizeBytes  *int64
	ObservedAt time.Time
	ClaimType  string
	NativeVer  string
	Kind       string
}

// Since streams observations after seq in log order. Order is by seq alone:
// observed_at is the observer's clock and several agents do not share one.
func (s *Store) Since(ctx context.Context, seq int64, limit int) ([]LoggedFile, error) {
	rows, err := s.pool.Query(ctx, `
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
