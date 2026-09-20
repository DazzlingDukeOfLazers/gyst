// Package store is the append-only observation log and its cursor bookkeeping.
//
// Nothing here updates or deletes an observation. The database enforces that
// with triggers, so a bug in this package surfaces as an error rather than as
// quietly rewritten history.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// Store is the one door to the database, whichever engine is behind it.
type Store struct{ db *engine }

// DSN is where the store lives. GYST_DATABASE_URL names it explicitly:
// postgres:// for the team profile, sqlite:<path> for a file. Unset, Gyst
// keeps a SQLite file in the user's data directory and needs nothing else
// installed or running. That is the solo profile (ADR 004 stage 4).
func DSN() string {
	if v := os.Getenv("GYST_DATABASE_URL"); v != "" {
		return v
	}
	return "sqlite:" + filepath.Join(DataDir(), "gyst.db")
}

// DataDir is where Gyst keeps its own files when not told otherwise:
// GYST_DATA_DIR if set, else the platform's per-user application data
// directory with a gyst folder inside it.
func DataDir() string {
	if v := os.Getenv("GYST_DATA_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "linux" {
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "gyst")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "gyst")
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "gyst")
	}
	return "gyst-data"
}

func Open(ctx context.Context) (*Store, error) {
	return OpenDSN(ctx, DSN())
}

// OpenDSN opens a specific database. See openEngine for the schemes.
func OpenDSN(ctx context.Context, dsn string) (*Store, error) {
	e, err := openEngine(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{db: e}, nil
}

func (s *Store) Close() { s.db.db.Close() }

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
	tx, err := s.db.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	recorded := time.Now().UTC()
	rows := make([][]any, 0, len(obs))
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
		rows = append(rows, []any{
			o.ObservationID, o.SchemaVersion, o.Source.SourceID, o.Source.Connector,
			o.Source.ConnectorVersion, nullable(o.Source.Cursor), o.ObservedAt, recorded,
			o.Subject.Kind, o.Subject.Location.Locator,
			o.Subject.Location.NativeVersion.Scheme, o.Subject.Location.NativeVersion.Value,
			digestAlgo, digestHex, size,
			o.Claim.Type, payload, extractor, policy, visibility, nullable(o.Corrects),
		})
	}
	n, err := tx.insertRows(ctx, "observations", []string{
		"observation_id", "schema_version", "source_id", "connector", "connector_version",
		"cursor", "observed_at", "recorded_at", "subject_kind", "locator",
		"native_version_scheme", "native_version_value",
		"content_digest_algo", "content_digest_hex", "size_bytes",
		"claim_type", "claim_payload", "extractor", "policy", "visibility", "corrects",
	}, rows, "ON CONFLICT (observation_id) DO NOTHING")
	if err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) Cursor(ctx context.Context, sourceID string) (string, error) {
	var c string
	err := s.db.queryRow(ctx,
		`SELECT cursor FROM source_cursors WHERE source_id=$1`, sourceID).Scan(&c)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return c, err
}

func (s *Store) SetCursor(ctx context.Context, sourceID, cursor string) error {
	_, err := s.db.exec(ctx, `
		INSERT INTO source_cursors (source_id, cursor, updated_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (source_id) DO UPDATE SET cursor=EXCLUDED.cursor, updated_at=EXCLUDED.updated_at`,
		sourceID, cursor, time.Now().UTC())
	return err
}

// KnownState loads what the projection believes about every locator in a
// source, as one query rather than a lookup per file.
//
// A scan of 100k files must not become 100k round trips, and the map is small:
// a locator and two scalars per row.
func (s *Store) KnownState(ctx context.Context, sourceID string) (map[string]observe.KnownState, error) {
	rows, err := s.db.query(ctx, `
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

// RegisterSource records where a source's locators are rooted and where that
// root physically lives. Without the root a locator cannot be resolved to a
// filesystem path, and two sources observing the same file cannot be
// recognised as such. Without the location a share and a local folder look
// the same, and they are not governed the same way.
func (s *Store) RegisterSource(ctx context.Context, sourceID, kind, root string, loc location.Location) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	_, err = s.db.exec(ctx, `
		INSERT INTO sources (source_id, kind, root,
			location_kind, location_provider, location_mount, location_evidence, location_confidence, last_seen, first_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		ON CONFLICT (source_id) DO UPDATE SET
			kind=EXCLUDED.kind, root=EXCLUDED.root, last_seen=EXCLUDED.last_seen,
			location_kind=EXCLUDED.location_kind, location_provider=EXCLUDED.location_provider,
			location_mount=EXCLUDED.location_mount, location_evidence=EXCLUDED.location_evidence,
			location_confidence=EXCLUDED.location_confidence`,
		sourceID, kind, abs,
		string(loc.Kind), loc.Provider, loc.Mount, loc.Evidence, loc.Confidence, time.Now().UTC())
	return err
}

// SetCadence records how often a source is expected to be scanned.
func (s *Store) SetCadence(ctx context.Context, sourceID string, d time.Duration) error {
	_, err := s.db.exec(ctx, `UPDATE sources SET cadence_seconds=$2 WHERE source_id=$1`,
		sourceID, int(d.Seconds()))
	return err
}

// SourceRow is a registered source as read back.
type SourceRow struct {
	SourceID     string
	Kind         string
	Root         string
	Location     location.Location
	ImportedFrom string // sender id when the source arrived in a bundle
}

// Sources lists every registered source.
func (s *Store) Sources(ctx context.Context) ([]SourceRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT source_id, kind, root,
		       location_kind, location_provider, location_mount, location_evidence, location_confidence, imported_from
		FROM sources ORDER BY source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRow
	for rows.Next() {
		var r SourceRow
		var kind string
		if err := rows.Scan(&r.SourceID, &r.Kind, &r.Root, &kind, &r.Location.Provider,
			&r.Location.Mount, &r.Location.Evidence, &r.Location.Confidence, &r.ImportedFrom); err != nil {
			return nil, err
		}
		r.Location.Kind = location.Kind(kind)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.queryRow(ctx, `SELECT count(*) FROM observations`).Scan(&n)
	return n, err
}

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
	rows, err := s.db.query(ctx, `
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
