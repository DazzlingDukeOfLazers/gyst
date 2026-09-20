package localfolder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoverageOfCompletePass(t *testing.T) {
	c := (&Result{Complete: true}).Coverage(false)
	if c.Status != CoverageComplete {
		t.Fatalf("complete unresumed pass with no skips classified %q: %s", c.Status, c.Detail)
	}
}

// Each way a pass can fall short must be reported as partial, and must say
// which one it was: a stale "last seen" reads differently depending on why the
// file was not seen.
func TestCoverageOfShortfalls(t *testing.T) {
	cases := []struct {
		name    string
		res     *Result
		resumed bool
	}{
		{"truncated by --max-files", &Result{Complete: false}, false},
		{"resumed from cursor", &Result{Complete: true}, true},
		{"unreadable entries", &Result{Complete: true, Skipped: 3}, false},
	}
	for _, tc := range cases {
		c := tc.res.Coverage(tc.resumed)
		if c.Status != CoveragePartial {
			t.Errorf("%s: status %q, want partial", tc.name, c.Status)
		}
		if c.Detail == "" {
			t.Errorf("%s: partial coverage with no explanation", tc.name)
		}
	}
}

// Coverage and tombstone eligibility are one decision. If they ever disagree,
// a pass could be recorded as complete while refusing to tombstone, or worse,
// tombstone from a pass recorded as partial.
func TestTombstonesFollowCoverage(t *testing.T) {
	for _, res := range []*Result{
		{Complete: true, Seen: map[string]bool{}, IgnoredPaths: map[string]bool{}},
		{Complete: false, Seen: map[string]bool{}, IgnoredPaths: map[string]bool{}},
		{Complete: true, Skipped: 1, Seen: map[string]bool{}, IgnoredPaths: map[string]bool{}},
	} {
		for _, cursor := range []string{"", "m/n.txt"} {
			cov := res.Coverage(cursor != "")
			d := Tombstones(res, Options{SourceID: "src", Cursor: cursor}, known("a.txt"))
			if d.Eligible != (cov.Status == CoverageComplete) {
				t.Errorf("complete=%v skipped=%d cursor=%q: coverage %q but tombstones eligible=%v",
					res.Complete, res.Skipped, cursor, cov.Status, d.Eligible)
			}
			if !d.Eligible && d.Reason != cov.Detail {
				t.Errorf("tombstone reason %q differs from coverage detail %q", d.Reason, cov.Detail)
			}
		}
	}
}

// A root that cannot be opened is not an empty folder. Discover must refuse
// rather than return a complete pass over nothing.
func TestDiscoverRefusesMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-mounted")
	_, err := Discover(Options{Root: missing, SourceID: "src"})
	if !IsUnavailable(err) {
		t.Fatalf("missing root: err = %v, want UnavailableError", err)
	}
}

func TestDiscoverRefusesFileAsRoot(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Discover(Options{Root: f, SourceID: "src"})
	if !IsUnavailable(err) {
		t.Fatalf("file as root: err = %v, want UnavailableError", err)
	}
}

// The clock a caller hands in must be the one every observation carries, or
// the pass row and the log disagree about when the pass happened.
func TestDiscoverUsesCallerClock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Push the mtime back so the stability window does not delay the test.
	past := timeAgo(t, dir, "old.txt")
	res, err := Discover(Options{Root: dir, SourceID: "src", Now: past})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pass.Equal(past) {
		t.Errorf("res.Pass = %v, want caller's %v", res.Pass, past)
	}
	for _, o := range res.Observations {
		if !o.ObservedAt.Equal(past) {
			t.Errorf("observation %s observed_at %v, want %v", o.Subject.Location.Locator, o.ObservedAt, past)
		}
	}
}
