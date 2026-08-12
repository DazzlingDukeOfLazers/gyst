package localfolder

import (
	"testing"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

func known(locators ...string) map[string]observe.KnownState {
	m := map[string]observe.KnownState{}
	for i, l := range locators {
		m[l] = observe.KnownState{NativeVersion: "100:42", Seq: int64(i + 1)}
	}
	return m
}

func complete(seen ...string) *Result {
	r := &Result{Complete: true, Seen: map[string]bool{}, IgnoredPaths: map[string]bool{}}
	for _, s := range seen {
		r.Seen[s] = true
	}
	return r
}

func TestTombstonesForMissingFiles(t *testing.T) {
	res := complete("a.txt")
	d := Tombstones(res, Options{SourceID: "src"}, known("a.txt", "b.txt"))

	if !d.Eligible {
		t.Fatalf("expected eligible, got: %s", d.Reason)
	}
	if len(d.Tombstones) != 1 {
		t.Fatalf("expected 1 tombstone, got %d", len(d.Tombstones))
	}
	tomb := d.Tombstones[0]
	if tomb.Subject.Location.Locator != "b.txt" {
		t.Errorf("tombstoned %q, want b.txt", tomb.Subject.Location.Locator)
	}
	if tomb.Claim.Type != "artifact.absent" {
		t.Errorf("claim type = %q", tomb.Claim.Type)
	}
	if tomb.Extractor.Confidence >= 1.0 {
		t.Errorf("absence recorded at confidence %.2f; a complete pass failing to "+
			"find a file is strong evidence but not proof (moved, unmounted, "+
			"permissions changed)", tomb.Extractor.Confidence)
	}
}

// A truncated pass saw only a prefix of the tree. Everything past the cutoff is
// missing for the wrong reason.
func TestNoTombstonesFromTruncatedScan(t *testing.T) {
	res := complete("a.txt")
	res.Complete = false
	d := Tombstones(res, Options{SourceID: "src"}, known("a.txt", "b.txt"))
	if d.Eligible || len(d.Tombstones) > 0 {
		t.Fatalf("truncated scan produced %d tombstones; unseen is not absent", len(d.Tombstones))
	}
}

// A resumed pass deliberately skips everything before its cursor.
func TestNoTombstonesFromResumedScan(t *testing.T) {
	res := complete("z.txt")
	d := Tombstones(res, Options{SourceID: "src", Cursor: "m.txt"}, known("a.txt", "z.txt"))
	if d.Eligible || len(d.Tombstones) > 0 {
		t.Fatalf("resumed scan produced %d tombstones", len(d.Tombstones))
	}
}

// An unreadable directory is a hole in coverage. Files beneath it are
// unobserved, not gone.
func TestNoTombstonesWhenEntriesWereSkipped(t *testing.T) {
	res := complete("a.txt")
	res.Skipped = 1
	d := Tombstones(res, Options{SourceID: "src"}, known("a.txt", "b.txt"))
	if d.Eligible || len(d.Tombstones) > 0 {
		t.Fatalf("scan with read errors produced %d tombstones", len(d.Tombstones))
	}
}

// A newly ignored file vanishes from a scan exactly like a deleted one, but it
// was removed from view, not from the source.
func TestNewlyIgnoredFilesAreNotTombstoned(t *testing.T) {
	res := complete("a.txt")
	res.IgnoredPaths["secret.key"] = true
	d := Tombstones(res, Options{SourceID: "src"}, known("a.txt", "secret.key"))

	if !d.Eligible {
		t.Fatalf("expected eligible, got: %s", d.Reason)
	}
	if len(d.Tombstones) != 0 {
		t.Errorf("tombstoned an ignored file: %q", d.Tombstones[0].Subject.Location.Locator)
	}
	if d.Suppressed != 1 {
		t.Errorf("Suppressed = %d, want 1", d.Suppressed)
	}
}

// An ignored directory hides its whole subtree.
func TestFilesUnderIgnoredDirectoryAreNotTombstoned(t *testing.T) {
	res := complete("a.txt")
	res.IgnoredPaths["scratch"] = true
	d := Tombstones(res, Options{SourceID: "src"}, known("a.txt", "scratch/tmp.log", "scratch/deep/x.log"))

	if len(d.Tombstones) != 0 {
		t.Errorf("tombstoned %d file(s) beneath an ignored directory, first %q",
			len(d.Tombstones), d.Tombstones[0].Subject.Location.Locator)
	}
	if d.Suppressed != 2 {
		t.Errorf("Suppressed = %d, want 2", d.Suppressed)
	}
}

// Regression, matching the DeriveID one. Delete, restore, delete again: the
// second tombstone must not collide with the first, or the projection shows the
// file present forever.
func TestTombstoneIDDistinguishesRepeatedDeletion(t *testing.T) {
	first := observe.TombstoneID("src", "a.txt", 5)
	second := observe.TombstoneID("src", "a.txt", 41)
	if first == second {
		t.Fatal("a second deletion after a restore derived the same tombstone id; " +
			"the insert would collide and the file would stay present")
	}
	if again := observe.TombstoneID("src", "a.txt", 5); again != first {
		t.Fatal("tombstone id is not stable for the same prior state")
	}
}

// A path that is ignored but shares a prefix with a real directory must not be
// suppressed by accident.
func TestIgnorePrefixMatchIsPathAware(t *testing.T) {
	if isIgnored("scratchpad/keep.txt", map[string]bool{"scratch": true}) {
		t.Error("'scratch' suppressed 'scratchpad/keep.txt'; prefix matching must " +
			"respect path boundaries")
	}
	if !isIgnored("scratch/tmp.log", map[string]bool{"scratch": true}) {
		t.Error("'scratch' failed to suppress 'scratch/tmp.log'")
	}
}
