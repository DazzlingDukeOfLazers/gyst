package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The fixture's expectations were hand-authored on day 1, before any of this
// code existed. Checking against them is the point: they are an independent
// claim about what correct grouping looks like, not a restatement of what the
// implementation happens to do.
type inventory struct {
	Files []struct {
		Path     string            `json:"path"`
		Profiles map[string]string `json:"profiles"`
		Note     string            `json:"note"`
	} `json:"files"`
}

func loadInventory(t *testing.T) inventory {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "expected-inventory.json")
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Skipf("fixture not generated (run testdata/generate.py): %v", err)
	}
	var inv inventory
	if err := json.Unmarshal(blob, &inv); err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestClassifyMatchesFixtureExpectations(t *testing.T) {
	inv := loadInventory(t)
	checked := 0
	for _, f := range inv.Files {
		for profile, want := range f.Profiles {
			got := Classify(Profile(profile), f.Path)
			if got.GroupingKey != want {
				t.Errorf("Classify(%s, %q)\n  grouping key = %q\n  fixture wants  %q\n  rule: %s\n  note: %s",
					profile, f.Path, got.GroupingKey, want, got.Rule, f.Note)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no profile expectations found in the fixture")
	}
	t.Logf("checked %d profile expectations", checked)
}

// The trap from the fixture. connector_123 and connector_124 are distinct part
// numbers. Under suffix-as-version they collapse to one grouping key, which is
// the profile working as designed -- but nothing may conclude that 124
// supersedes 123, because a bare trailing number is not a revision scheme.
func TestBareNumericSuffixNeverAssertsSupersession(t *testing.T) {
	a := Classify(SuffixAsVersion, "engineering/connectors/connector_123.pdf")
	b := Classify(SuffixAsVersion, "engineering/connectors/connector_124.pdf")

	if a.GroupingKey != b.GroupingKey {
		t.Fatalf("expected the same grouping key under suffix-as-version, got %q and %q",
			a.GroupingKey, b.GroupingKey)
	}
	if a.VersionLabel != "123" || b.VersionLabel != "124" {
		t.Fatalf("expected version labels 123/124, got %q/%q", a.VersionLabel, b.VersionLabel)
	}
	if a.Confidence >= SupersedesThreshold {
		t.Errorf("bare numeric suffix reached confidence %.2f, at or above the %.2f "+
			"supersedes threshold; connector_124 would appear to supersede connector_123",
			a.Confidence, SupersedesThreshold)
	}

	rel, conf, why := RelationFor(SuffixAsVersion, b, a)
	if rel != "compare-set-with" {
		t.Fatalf("relation between connector_124 and connector_123 = %q at confidence %.2f, "+
			"want compare-set-with; explanation: %s", rel, conf, why)
	}
}

// The other side of the same threshold: an explicit revision marker is exactly
// the case supersession is for, and it must not be suppressed.
func TestExplicitRevisionMarkerDoesAssertSupersession(t *testing.T) {
	v2 := Classify(SuffixAsVersion, "engineering/widget/widget_rev2.pdf")
	v3 := Classify(SuffixAsVersion, "engineering/widget/widget_rev3.pdf")

	if v2.GroupingKey != v3.GroupingKey {
		t.Fatalf("rev2 and rev3 did not group together: %q vs %q", v2.GroupingKey, v3.GroupingKey)
	}
	rel, conf, why := RelationFor(SuffixAsVersion, v3, v2)
	if rel != "supersedes" {
		t.Fatalf("rev3 -> rev2 = %q at confidence %.2f, want supersedes; explanation: %s",
			rel, conf, why)
	}
}

// A revision marker must be its own token. This is the regression for reading
// the trailing "r" of "connector_123" as a revision marker, which produced
// "version 123 of connecto".
func TestRevisionMarkerRequiresSeparator(t *testing.T) {
	for _, locator := range []string{
		"engineering/connectors/connector_123.pdf",
		"parts/resistor_100.pdf",
		"docs/chapter7.pdf",
	} {
		m := Classify(SuffixAsVersion, locator)
		if m.Rule == "explicit-revision-marker" {
			t.Errorf("%q matched the explicit revision rule as version %q of %q; "+
				"a marker embedded in a word is not a marker",
				locator, m.VersionLabel, m.GroupingKey)
		}
	}
}

// "FINAL" is not a revision scheme. Strict suffix-as-version must leave it
// alone; only compare-set, which never orders anything, may group across it.
func TestQualifierWordsOnlyGroupUnderCompareSet(t *testing.T) {
	plain := "engineering/vendor-drop/Assembly Notes v2.pdf"
	final := "engineering/vendor-drop/Assembly Notes v2 FINAL.pdf"

	if a, b := Classify(SuffixAsVersion, plain), Classify(SuffixAsVersion, final); a.GroupingKey == b.GroupingKey {
		t.Errorf("suffix-as-version grouped across a qualifier word: both became %q. "+
			"Stripping FINAL is a guess that a strict profile must not make", a.GroupingKey)
	}
	if a, b := Classify(CompareSet, plain), Classify(CompareSet, final); a.GroupingKey != b.GroupingKey {
		t.Errorf("compare-set failed to group candidates for side-by-side review: %q vs %q",
			a.GroupingKey, b.GroupingKey)
	}
}

// Switching profiles must change grouping and nothing else. This is the shape
// of the day 3 exit criterion at the unit level; the end-to-end version checks
// the observation log is untouched.
func TestProfileSwitchChangesOnlyGrouping(t *testing.T) {
	const locator = "engineering/connectors/connector_124.pdf"
	seen := map[string]bool{}
	for _, p := range Profiles() {
		m := Classify(p, locator)
		if m.Explanation == "" {
			t.Errorf("profile %s produced no explanation; grouping must be explainable "+
				"before activation", p)
		}
		seen[m.GroupingKey] = true
	}
	if len(seen) < 2 {
		t.Error("every profile produced the same grouping key; the profiles are not distinct")
	}
}
