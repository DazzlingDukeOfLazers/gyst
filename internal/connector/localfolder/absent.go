package localfolder

import (
	"fmt"
	"sort"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// TombstoneDecision explains whether deletion detection was possible at all.
//
// Absence is the one claim a scanner makes about something it did not see, so
// it is the one claim that has to justify itself. A file missing from a partial
// pass is not evidence of anything.
type TombstoneDecision struct {
	Eligible bool
	Reason   string

	Tombstones []observe.Observation
	Suppressed int
}

// Tombstones derives absence observations for locators that were known to exist
// but were not seen by a scan.
//
// Four conditions must all hold before absence counts as evidence:
//
//   - The pass completed. A scan stopped by --max-files saw only a prefix of the
//     tree, and everything after that point is missing for the wrong reason.
//   - The pass was not resumed. A resumed scan deliberately skips everything at
//     or before its cursor, so most of the tree is legitimately unseen.
//   - Nothing was skipped. An unreadable directory is a hole in coverage; files
//     beneath it are unobserved, not gone.
//   - The locator is not newly ignored. A file excluded by a new .gystignore rule
//     vanishes from the scan exactly like a deleted one, but asserting it was
//     removed from the source would be false.
//
// The first three are properties of the pass and disable tombstoning entirely.
// The fourth is per-locator and merely suppresses individual files.
func Tombstones(res *Result, opts Options, known map[string]observe.KnownState) TombstoneDecision {
	switch {
	case !res.Complete:
		return TombstoneDecision{Reason: "scan was truncated by --max-files; unseen files are unobserved, not absent"}
	case opts.Cursor != "":
		return TombstoneDecision{Reason: "scan resumed from a cursor and deliberately skipped earlier paths"}
	case res.Skipped > 0:
		return TombstoneDecision{Reason: fmt.Sprintf(
			"%d entries could not be read; coverage has holes, so absence is not evidence", res.Skipped)}
	}

	policy := observe.Policy{
		ContentLevel:           opts.ContentLevel,
		Egress:                 opts.Egress,
		EffectivePolicyVersion: opts.PolicyVersion,
	}
	if policy.ContentLevel == "" {
		policy.ContentLevel = observe.ContentFingerprint
	}
	if policy.Egress == "" {
		policy.Egress = "device"
	}

	// The scan's clock reading, not a fresh one: tombstones must land in the
	// same pass as the arrivals they may pair with.
	now := res.Pass
	if now.IsZero() {
		now = time.Now().UTC()
	}
	decision := TombstoneDecision{Eligible: true, Reason: "complete unresumed pass with no read errors"}

	// Sorted so a run is deterministic and diffable.
	missing := make([]string, 0, 8)
	for locator := range known {
		if res.Seen[locator] {
			continue
		}
		if isIgnored(locator, res.IgnoredPaths) {
			decision.Suppressed++
			continue
		}
		missing = append(missing, locator)
	}
	sort.Strings(missing)

	for _, locator := range missing {
		decision.Tombstones = append(decision.Tombstones,
			observe.Tombstone(opts.SourceID, locator, known[locator], policy, now))
	}
	return decision
}

// isIgnored reports whether a locator is excluded, either directly or because
// an ancestor directory is.
func isIgnored(locator string, ignored map[string]bool) bool {
	if ignored[locator] {
		return true
	}
	for dir := range ignored {
		if len(locator) > len(dir) && locator[:len(dir)] == dir && locator[len(dir)] == '/' {
			return true
		}
	}
	return false
}
