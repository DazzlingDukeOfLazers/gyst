// Package project rebuilds query models from the observation log.
//
// Every table this package writes is disposable. Drop it, reset the projector
// cursor to zero, replay, and the result must be identical -- that equivalence
// is the day 2 exit criterion, and Verify below is what checks it.
//
// The SQL lives in internal/store (ADR 004); this package holds the
// domain logic and delegates the folds.
package project

import (
	"context"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const Name = store.ProjectorName

type Stats = store.ProjectionStats

// Apply consumes new observations and folds them into current_files.
func Apply(ctx context.Context, s *store.Store) (Stats, error) {
	return s.ApplyCurrentFiles(ctx)
}

// Fingerprint hashes the whole projection into one digest.
func Fingerprint(ctx context.Context, s *store.Store) (string, int64, error) {
	return s.ProjectionFingerprint(ctx)
}

// Verify proves the projection is reproducible: fingerprint it, drop it,
// replay the entire log from seq 0, and compare.
func Verify(ctx context.Context, s *store.Store) (before, after string, rows int64, err error) {
	return s.VerifyProjection(ctx)
}
