package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Bookkeeping retention (ADR 005, the part that touches no evidence).
// Passes and resolved findings are records about Gyst's own activity, not
// about the sources; bounding them changes nothing a finding, relation, or
// release cites.

// PruneScanPasses keeps the newest keep passes for a source plus the very
// first one, and removes the rest. The first pass is kept because "when did
// Gyst first see this source" is worth answering forever.
func (s *Store) PruneScanPasses(ctx context.Context, sourceID string, keep int) (int, error) {
	if keep < 1 {
		keep = 1
	}
	rows, err := s.db.query(ctx, `SELECT pass_id FROM scan_passes WHERE source_id=$1 ORDER BY started_at DESC`, sourceID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) <= keep+1 {
		return 0, nil
	}
	victims := ids[keep : len(ids)-1] // newest keep are ids[:keep]; the oldest is ids[len-1]
	removed := 0
	const chunk = 500
	for i := 0; i < len(victims); i += chunk {
		end := i + chunk
		if end > len(victims) {
			end = len(victims)
		}
		part := victims[i:end]
		args := make([]any, len(part))
		ph := make([]string, len(part))
		for j, id := range part {
			args[j] = id
			ph[j] = fmt.Sprintf("$%d", j+1)
		}
		res, err := s.db.exec(ctx, `DELETE FROM scan_passes WHERE pass_id IN (`+strings.Join(ph, ",")+`)`, args...)
		if err != nil {
			return removed, err
		}
		removed += int(affected(res))
	}
	return removed, nil
}

// PruneResolvedFindings removes findings resolved before the cutoff that
// carry no waiver. A waiver is an audit record of a person's decision and
// is kept whatever its age.
func (s *Store) PruneResolvedFindings(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.exec(ctx, `
		DELETE FROM findings WHERE status='resolved' AND resolved_at < $1 AND waiver IS NULL`, before)
	if err != nil {
		return 0, err
	}
	return int(affected(res)), nil
}
