package project

import "testing"

const (
	digestA = "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888"
	digestB = "1111aaaa2222bbbb3333cccc4444dddd5555eeee6666ffff7777aaaa8888bbbb"
	pass1   = "2024-08-12 09:00:00+00"
	pass2   = "2024-08-12 10:00:00+00"
)

func gone(locator, digest string, size int64, pass string) Gone {
	return Gone{SourceID: "src", Locator: locator, Digest: digest, Size: size,
		ObsID: "obs_gone_" + locator, Pass: pass}
}

func arrived(locator, digest string, size int64, pass string) Arrived {
	return Arrived{SourceID: "src", Locator: locator, Digest: digest, Size: size,
		ObsID: "obs_new_" + locator, Pass: pass}
}

func TestUnambiguousRenameIsCalled(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("old/spec.pdf", digestA, 1200, pass1)},
		[]Arrived{arrived("new/spec.pdf", digestA, 1200, pass1)},
	)
	if rep.Renamed != 1 || len(rep.Calls) != 1 {
		t.Fatalf("expected 1 rename, got %d calls (%d renamed)", len(rep.Calls), rep.Renamed)
	}
	c := rep.Calls[0]
	if c.Type != "renamed-from" {
		t.Errorf("type = %q, want renamed-from", c.Type)
	}
	if c.To.Locator != "new/spec.pdf" || c.From.Locator != "old/spec.pdf" {
		t.Errorf("direction wrong: %s renamed-from %s", c.To.Locator, c.From.Locator)
	}
	if c.Confidence < 0.9 {
		t.Errorf("confidence %.2f is low for an unambiguous match", c.Confidence)
	}
}

// Every empty file shares one digest. Matching on it pairs unrelated files.
func TestEmptyFilesAreNeverMatched(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("a/empty.txt", EmptySHA256, 0, pass1)},
		[]Arrived{arrived("b/other-empty.txt", EmptySHA256, 0, pass1)},
	)
	if len(rep.Calls) != 0 {
		t.Fatalf("matched empty files: %s renamed-from %s",
			rep.Calls[0].To.Locator, rep.Calls[0].From.Locator)
	}
	if rep.SkippedNoContent != 1 {
		t.Errorf("SkippedNoContent = %d, want 1", rep.SkippedNoContent)
	}
}

// A zero-size file with some other recorded digest is still contentless.
func TestZeroSizeIsSkippedRegardlessOfDigest(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("a/x", digestA, 0, pass1)},
		[]Arrived{arrived("b/y", digestA, 0, pass1)},
	)
	if len(rep.Calls) != 0 {
		t.Fatal("matched a zero-length file")
	}
}

// The fixture's widget_bom / widget_bom (copy) case. Two files already hold
// identical bytes; deleting one and adding a third elsewhere gives no basis for
// deciding which moved.
func TestDuplicateContentProducesCandidatesNotACall(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("widget_bom.xlsx", digestA, 1715, pass1)},
		[]Arrived{
			arrived("archive/bom-a.xlsx", digestA, 1715, pass1),
			arrived("archive/bom-b.xlsx", digestA, 1715, pass1),
		},
	)
	if rep.Renamed != 0 {
		t.Fatal("asserted a rename despite two equally good destinations")
	}
	if rep.Ambiguous != 1 {
		t.Errorf("Ambiguous = %d, want 1", rep.Ambiguous)
	}
	if len(rep.Calls) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(rep.Calls))
	}
	for _, c := range rep.Calls {
		if c.Type != "compare-set-with" {
			t.Errorf("ambiguous case emitted %q", c.Type)
		}
		if c.Confidence >= 0.5 {
			t.Errorf("ambiguous candidate at confidence %.2f", c.Confidence)
		}
	}
}

// A directory reorganisation is not a set of renames.
func TestManyToManyIsNotResolved(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("old/1.bin", digestA, 500, pass1), gone("old/2.bin", digestA, 500, pass1)},
		[]Arrived{arrived("new/1.bin", digestA, 500, pass1), arrived("new/2.bin", digestA, 500, pass1)},
	)
	if rep.Renamed != 0 {
		t.Fatal("resolved a many-to-many reorganisation into renames")
	}
	if rep.Ambiguous != 2 {
		t.Errorf("Ambiguous = %d, want 2", rep.Ambiguous)
	}
}

// A deletion with no matching arrival stays a deletion.
func TestPlainDeletionIsNotARename(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("gone.pdf", digestA, 900, pass1)},
		[]Arrived{arrived("unrelated.pdf", digestB, 900, pass1)},
	)
	if len(rep.Calls) != 0 {
		t.Fatal("invented a rename between files with different content")
	}
	if rep.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", rep.Unmatched)
	}
}

// A file deleted in one pass and an identical one created much later is not a
// move in any useful sense.
func TestMatchingIsScopedToOnePass(t *testing.T) {
	rep := DetectRenames(
		[]Gone{gone("old.pdf", digestA, 700, pass1)},
		[]Arrived{arrived("new.pdf", digestA, 700, pass2)},
	)
	if len(rep.Calls) != 0 {
		t.Fatal("matched across scan passes")
	}
	if rep.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", rep.Unmatched)
	}
}

// Locators are rooted per source, so identical content in two sources is a
// coincidence, not a move.
func TestRenamesDoNotCrossSources(t *testing.T) {
	g := gone("a.pdf", digestA, 400, pass1)
	a := arrived("a.pdf", digestA, 400, pass1)
	a.SourceID = "other_src"

	rep := DetectRenames([]Gone{g}, []Arrived{a})
	if len(rep.Calls) != 0 {
		t.Fatal("matched a rename across two sources")
	}
}

// An ambiguous arrival must not produce an unbounded candidate list.
func TestCandidateListIsBounded(t *testing.T) {
	var arrivals []Arrived
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		arrivals = append(arrivals, arrived("new/"+n, digestA, 300, pass1))
	}
	rep := DetectRenames([]Gone{gone("old.bin", digestA, 300, pass1)}, arrivals)

	if len(rep.Calls) != maxCandidates {
		t.Fatalf("emitted %d candidates, want the cap of %d", len(rep.Calls), maxCandidates)
	}
	if rep.Truncated != len(arrivals)-maxCandidates {
		t.Errorf("Truncated = %d, want %d; a silently dropped candidate reads as "+
			"'considered and rejected'", rep.Truncated, len(arrivals)-maxCandidates)
	}
}

// Output must not depend on input ordering or map iteration.
func TestDetectionIsDeterministic(t *testing.T) {
	g := []Gone{gone("old/b.bin", digestB, 800, pass1), gone("old/a.bin", digestA, 900, pass1)}
	a := []Arrived{arrived("new/a.bin", digestA, 900, pass1), arrived("new/b.bin", digestB, 800, pass1)}

	first := DetectRenames(g, a)
	second := DetectRenames([]Gone{g[1], g[0]}, []Arrived{a[1], a[0]})

	if len(first.Calls) != len(second.Calls) {
		t.Fatalf("call count differs by input order: %d vs %d", len(first.Calls), len(second.Calls))
	}
	for i := range first.Calls {
		if first.Calls[i].To.Locator != second.Calls[i].To.Locator {
			t.Fatalf("call %d differs by input order: %s vs %s",
				i, first.Calls[i].To.Locator, second.Calls[i].To.Locator)
		}
	}
}
