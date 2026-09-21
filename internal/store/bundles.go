package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// TrustedKey is a sender the receiver has chosen to believe.
type TrustedKey struct {
	SenderID  string
	PublicKey string
	AddedAt   time.Time
	AddedBy   string
	Note      string
}

// TrustKey records a sender's public key. A sender already known keeps its
// key: replacing one is a deliberate act, done by removing first.
func (s *Store) TrustKey(ctx context.Context, k TrustedKey) error {
	_, err := s.db.exec(ctx, `
		INSERT INTO trusted_keys (sender_id, public_key, added_at, added_by, note)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (sender_id) DO NOTHING`,
		k.SenderID, k.PublicKey, k.AddedAt, k.AddedBy, k.Note)
	return err
}

// TrustedKeyOf returns a sender's key, or ErrNotFound.
func (s *Store) TrustedKeyOf(ctx context.Context, senderID string) (TrustedKey, error) {
	var k TrustedKey
	err := s.db.queryRow(ctx, `SELECT sender_id, public_key, added_at, added_by, note FROM trusted_keys WHERE sender_id=$1`, senderID).
		Scan(&k.SenderID, &k.PublicKey, ts(&k.AddedAt), &k.AddedBy, &k.Note)
	return k, notFound(err)
}

// TrustedKeys lists every trusted sender.
func (s *Store) TrustedKeys(ctx context.Context) ([]TrustedKey, error) {
	rows, err := s.db.query(ctx, `SELECT sender_id, public_key, added_at, added_by, note FROM trusted_keys ORDER BY sender_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrustedKey
	for rows.Next() {
		var k TrustedKey
		if err := rows.Scan(&k.SenderID, &k.PublicKey, ts(&k.AddedAt), &k.AddedBy, &k.Note); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// BundleRow records a received bundle.
type BundleRow struct {
	BundleID     string
	SenderID     string
	CreatedAt    time.Time
	ReceivedAt   time.Time
	Egress       string
	BodySHA256   string
	Observations int
	Appended     int
	Sources      []byte
}

// RecordBundle notes a receipt. Receiving the same bundle twice is a
// no-op here and in the log, since observation ids are content-derived.
func (s *Store) RecordBundle(ctx context.Context, b BundleRow) error {
	_, err := s.db.exec(ctx, `
		INSERT INTO bundles (bundle_id, sender_id, created_at, received_at, egress, body_sha256, observations, appended, sources)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (bundle_id) DO NOTHING`,
		b.BundleID, b.SenderID, b.CreatedAt, b.ReceivedAt, b.Egress, b.BodySHA256, b.Observations, b.Appended, b.Sources)
	return err
}

// Bundles lists received bundles, newest first.
func (s *Store) Bundles(ctx context.Context) ([]BundleRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT bundle_id, sender_id, created_at, received_at, egress, body_sha256, observations, appended, sources
		FROM bundles ORDER BY received_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BundleRow
	for rows.Next() {
		var b BundleRow
		if err := rows.Scan(&b.BundleID, &b.SenderID, ts(&b.CreatedAt), ts(&b.ReceivedAt), &b.Egress, &b.BodySHA256,
			&b.Observations, &b.Appended, &b.Sources); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkImported records which sender a source arrived from.
func (s *Store) MarkImported(ctx context.Context, sourceID, senderID string) error {
	_, err := s.db.exec(ctx, `UPDATE sources SET imported_from=$2 WHERE source_id=$1`, sourceID, senderID)
	return err
}

// ImportedFrom returns the sender of a source, "" when it is local.
func (s *Store) ImportedFrom(ctx context.Context, sourceID string) (string, error) {
	var from string
	err := s.db.queryRow(ctx, `SELECT imported_from FROM sources WHERE source_id=$1`, sourceID).Scan(&from)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	return from, notFound(err)
}

// Observations streams a source's observations as full envelopes, in log
// order, calling fn for each. This is the export side of a bundle: the
// row goes back out exactly as it came in.
func (s *Store) Observations(ctx context.Context, sourceID string, sinceSeq int64, fn func(o observe.Observation) error) (int, error) {
	rows, err := s.db.query(ctx, `
		SELECT observation_id, schema_version, source_id, connector, connector_version, cursor,
		       observed_at, subject_kind, locator, native_version_scheme, native_version_value,
		       content_digest_algo, content_digest_hex, size_bytes,
		       claim_type, claim_payload, extractor, policy, visibility, corrects
		FROM observations WHERE source_id=$1 AND seq > $2 ORDER BY seq`, sourceID, sinceSeq)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var o observe.Observation
		var cursor, algo, hex, corrects *string
		var size *int64
		var payload, extractor, policy, visibility []byte
		if err := rows.Scan(&o.ObservationID, &o.SchemaVersion, &o.Source.SourceID, &o.Source.Connector, &o.Source.ConnectorVersion,
			&cursor, ts(&o.ObservedAt), &o.Subject.Kind, &o.Subject.Location.Locator,
			&o.Subject.Location.NativeVersion.Scheme, &o.Subject.Location.NativeVersion.Value,
			&algo, &hex, &size, &o.Claim.Type, &payload, &extractor, &policy, &visibility, &corrects); err != nil {
			return n, err
		}
		o.Source.Cursor, o.Corrects = deref(cursor), deref(corrects)
		o.Subject.Location.SourceID = o.Source.SourceID
		if size != nil || hex != nil {
			o.Subject.Version = &observe.Version{}
			if size != nil {
				o.Subject.Version.SizeBytes = *size
			}
			if hex != nil {
				o.Subject.Version.ContentDigest = &observe.Digest{Algo: deref(algo), Hex: *hex}
			}
		}
		if err := json.Unmarshal(payload, &o.Claim.Payload); err != nil {
			return n, err
		}
		if err := json.Unmarshal(extractor, &o.Extractor); err != nil {
			return n, err
		}
		if err := json.Unmarshal(policy, &o.Policy); err != nil {
			return n, err
		}
		if err := json.Unmarshal(visibility, &o.Visibility); err != nil {
			return n, err
		}
		if err := fn(o); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}
