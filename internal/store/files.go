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

// ErrNotFound is returned by point lookups that matched nothing. Callers
// outside this package compare against it rather than against a driver
// error, so the engine can change without them noticing.
var ErrNotFound = errors.New("not found")

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// PresentFile is a row of the current inventory joined to the observation
// it reflects. Every projection that walks the inventory starts here.
type PresentFile struct {
	SourceID      string
	Locator       string
	LatestSeq     int64
	Digest        string // "" when policy withheld it
	Size          int64
	NativeVersion string
	ObservedAt    time.Time
	ObsID         string
}

// PresentFiles lists every present file in source, locator order.
func (s *Store) PresentFiles(ctx context.Context) ([]PresentFile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cf.source_id, cf.locator, cf.latest_seq, coalesce(cf.content_digest_hex,''),
		       coalesce(cf.size_bytes,0), cf.native_version_value, cf.observed_at, coalesce(o.observation_id,'')
		FROM current_files cf LEFT JOIN observations o ON o.seq = cf.latest_seq
		WHERE cf.present ORDER BY cf.source_id, cf.locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PresentFile
	for rows.Next() {
		var f PresentFile
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.LatestSeq, &f.Digest, &f.Size,
			&f.NativeVersion, &f.ObservedAt, &f.ObsID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FindPresentFile matches a needle exactly or as a locator suffix, shortest
// match first, so a person can paste a bare filename.
func (s *Store) FindPresentFile(ctx context.Context, needle string) (PresentFile, error) {
	var f PresentFile
	var digest, obs *string
	var size *int64
	err := s.pool.QueryRow(ctx, `
		SELECT cf.source_id, cf.locator, cf.latest_seq, cf.content_digest_hex, cf.size_bytes,
		       cf.native_version_value, cf.observed_at, o.observation_id
		FROM current_files cf LEFT JOIN observations o ON o.seq = cf.latest_seq
		WHERE cf.present AND (cf.locator = $1 OR cf.locator LIKE '%' || $1)
		ORDER BY length(cf.locator) LIMIT 1`, needle).
		Scan(&f.SourceID, &f.Locator, &f.LatestSeq, &digest, &size, &f.NativeVersion, &f.ObservedAt, &obs)
	if err != nil {
		return f, notFound(err)
	}
	f.Digest, f.ObsID = deref(digest), deref(obs)
	if size != nil {
		f.Size = *size
	}
	return f, nil
}

// PresentFileAt is the exact-locator lookup.
func (s *Store) PresentFileAt(ctx context.Context, sourceID, locator string) (PresentFile, error) {
	var f PresentFile
	var digest, obs *string
	var size *int64
	err := s.pool.QueryRow(ctx, `
		SELECT cf.source_id, cf.locator, cf.latest_seq, cf.content_digest_hex, cf.size_bytes,
		       cf.native_version_value, cf.observed_at, o.observation_id
		FROM current_files cf LEFT JOIN observations o ON o.seq = cf.latest_seq
		WHERE cf.source_id=$1 AND cf.locator=$2 AND cf.present`, sourceID, locator).
		Scan(&f.SourceID, &f.Locator, &f.LatestSeq, &digest, &size, &f.NativeVersion, &f.ObservedAt, &obs)
	if err != nil {
		return f, notFound(err)
	}
	f.Digest, f.ObsID = deref(digest), deref(obs)
	if size != nil {
		f.Size = *size
	}
	return f, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ObservationRow is one log entry as explain shows it.
type ObservationRow struct {
	ObservationID    string
	Seq              int64
	ObservedAt       time.Time
	ClaimType        string
	Digest           string
	NativeVersion    string
	ContentLevel     string
	Connector        string
	ConnectorVersion string
	SourceID         string
	Locator          string
	Size             int64
}

// ObservationsOf returns every observation of one locator in log order.
func (s *Store) ObservationsOf(ctx context.Context, sourceID, locator string) ([]ObservationRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT observation_id, seq, observed_at, claim_type,
		       coalesce(content_digest_hex,''), native_version_value,
		       coalesce(policy->>'content_level',''), connector, connector_version
		FROM observations WHERE source_id=$1 AND locator=$2 ORDER BY seq`, sourceID, locator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObservationRow
	for rows.Next() {
		var o ObservationRow
		if err := rows.Scan(&o.ObservationID, &o.Seq, &o.ObservedAt, &o.ClaimType, &o.Digest,
			&o.NativeVersion, &o.ContentLevel, &o.Connector, &o.ConnectorVersion); err != nil {
			return nil, err
		}
		o.SourceID, o.Locator = sourceID, locator
		out = append(out, o)
	}
	return out, rows.Err()
}

// RecentObservations returns the newest observations since a cutoff.
func (s *Store) RecentObservations(ctx context.Context, since time.Time, limit int) ([]ObservationRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT observed_at, claim_type, source_id, locator, coalesce(content_digest_hex,'-'), coalesce(size_bytes,0)
		FROM observations WHERE observed_at >= $1 ORDER BY seq DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObservationRow
	for rows.Next() {
		var o ObservationRow
		if err := rows.Scan(&o.ObservedAt, &o.ClaimType, &o.SourceID, &o.Locator, &o.Digest, &o.Size); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// LatestObservationOf returns the id of the newest observation from a
// source, or "" when it has none.
func (s *Store) LatestObservationOf(ctx context.Context, sourceID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT observation_id FROM observations WHERE source_id=$1 ORDER BY seq DESC LIMIT 1`, sourceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// LogFingerprint hashes the observation log. Comparing it before and after
// a profile switch is the day 3 exit criterion: grouping may change,
// evidence may not.
func (s *Store) LogFingerprint(ctx context.Context) (string, int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT observation_id, locator, native_version_value, coalesce(content_digest_hex,''), claim_type
		FROM observations ORDER BY seq`)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	h := sha256.New()
	var n int64
	for rows.Next() {
		var id, loc, nv, digest, claim string
		if err := rows.Scan(&id, &loc, &nv, &digest, &claim); err != nil {
			return "", 0, err
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\n", id, loc, nv, digest, claim)
		n++
	}
	return hex.EncodeToString(h.Sum(nil)), n, rows.Err()
}
