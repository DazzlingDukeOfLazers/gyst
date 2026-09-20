package store

import (
	"context"
	"encoding/json"
	"time"
)

// InventoryRow is a current_files row, present or absent, joined to its
// observation's policy and placeholder flag, for the report.
type InventoryRow struct {
	SourceID      string
	Locator       string
	Present       bool
	Size          *int64
	Digest        *string
	NativeVersion string
	ObservedAt    time.Time
	ObsID         string
	ContentLevel  string
	Placeholder   bool
}

// Inventory lists every locator the projection knows, present or not.
func (s *Store) Inventory(ctx context.Context) ([]InventoryRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cf.source_id, cf.locator, cf.present, cf.size_bytes, cf.content_digest_hex,
		       cf.native_version_value, cf.observed_at, o.observation_id, o.policy, o.claim_payload
		FROM current_files cf JOIN observations o ON o.seq = cf.latest_seq
		ORDER BY cf.source_id, cf.locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InventoryRow
	for rows.Next() {
		var r InventoryRow
		var policy, payload []byte
		if err := rows.Scan(&r.SourceID, &r.Locator, &r.Present, &r.Size, &r.Digest, &r.NativeVersion,
			&r.ObservedAt, &r.ObsID, &policy, &payload); err != nil {
			return nil, err
		}
		r.ContentLevel = jsonString(policy, "content_level")
		var p struct {
			Placeholder bool `json:"placeholder"`
		}
		_ = json.Unmarshal(payload, &p)
		r.Placeholder = p.Placeholder
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountPresentFiles counts present files in a source.
func (s *Store) CountPresentFiles(ctx context.Context, sourceID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM current_files WHERE source_id=$1 AND present`, sourceID).Scan(&n)
	return n, err
}

// SourceCadence returns a source's configured cadence in seconds, 0 when
// unset.
func (s *Store) SourceCadence(ctx context.Context, sourceID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT cadence_seconds FROM sources WHERE source_id=$1`, sourceID).Scan(&n)
	return n, notFound(err)
}
