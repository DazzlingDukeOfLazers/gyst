package localfolder

import (
	"errors"
	"fmt"
)

// Coverage statuses. Interrupted is deliberately absent: a Result is only
// produced by a pass that ended, so "began and never ended" is something the
// store concludes about a pass that has no Result, not something a Result
// can say about itself.
const (
	CoverageComplete    = "complete"
	CoveragePartial     = "partial"
	CoverageUnavailable = "unavailable"
)

// Coverage says how much of the source a pass actually looked at.
//
// Observations from a partial pass are just as true as those from a complete
// one. What differs is what can be concluded from *not* seeing something, and
// how a person should read "last seen": a file unobserved by a pass that
// stopped early is unobserved, not stale and not gone.
type Coverage struct {
	Status string
	Detail string
}

// Coverage classifies a finished pass. resumed reports whether the pass began
// from a stored cursor, which the Result cannot know on its own.
//
// This is the single definition of "the pass looked everywhere". Tombstones
// consults it rather than restating the conditions, so the two cannot drift.
func (r *Result) Coverage(resumed bool) Coverage {
	switch {
	case !r.Complete:
		return Coverage{CoveragePartial,
			"scan was truncated by --max-files; unseen files are unobserved, not absent"}
	case resumed:
		return Coverage{CoveragePartial,
			"scan resumed from a cursor and deliberately skipped earlier paths"}
	case r.Skipped > 0:
		return Coverage{CoveragePartial, fmt.Sprintf(
			"%d entries could not be read; coverage has holes, so absence is not evidence", r.Skipped)}
	default:
		return Coverage{CoverageComplete, "complete unresumed pass with no read errors"}
	}
}

// UnavailableError reports that a root could not be opened at all. It is
// distinct from an unreadable directory inside a scan: nothing was observed,
// so nothing about the source has changed and no cursor moves.
//
// An unmounted network share is the expected producer. It must not read as an
// empty folder: a complete pass over nothing would tombstone every known file.
type UnavailableError struct {
	Root string
	Err  error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("root %s unavailable: %v", e.Root, e.Err)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

// IsUnavailable reports whether err means the root could not be opened.
func IsUnavailable(err error) bool {
	var u *UnavailableError
	return errors.As(err, &u)
}
