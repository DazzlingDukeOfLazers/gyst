package authority

import (
	"fmt"
	"strings"
	"testing"
)

func k(loc string) Key { return Key{"s", loc} }
func f(loc, digest string) File {
	return File{Key: k(loc), Digest: digest, ObsID: "obs_" + loc}
}

func TestSoleCopyIsNoneNotDeclared(t *testing.T) {
	a := Resolve(Input{Files: []File{f("a.pdf", "d1")}})[k("a.pdf")]
	if a.State != StateNone || a.Of != nil {
		t.Fatalf("%+v; a single copy nobody declared is not an authority", a)
	}
	if len(a.Evidence) == 0 {
		t.Error("even 'none' must cite what was looked at")
	}
}

// Every empty file shares one digest. Two .keep files are not candidates
// for each other's authority.
func TestEmptyFilesAreNotPeers(t *testing.T) {
	res := Resolve(Input{Files: []File{f("a/.keep", emptySHA256), f("b/.keep", emptySHA256)}})
	if res[k("a/.keep")].State != StateNone {
		t.Fatalf("%+v", res[k("a/.keep")])
	}
}

// A file inside a dependency directory is nobody's candidate.
func TestVendoredFilesAreNotPeers(t *testing.T) {
	res := Resolve(Input{Files: []File{f("src/lib.js", "d"), f("node_modules/x/lib.js", "d")}})
	if a := res[k("src/lib.js")]; a.State != StateNone {
		t.Errorf("real file with a vendored copy: %+v", a)
	}
	if a := res[k("node_modules/x/lib.js")]; a.State != StateNone || !strings.Contains(a.Explanation, "dependency") {
		t.Errorf("vendored file: %+v", a)
	}
}

func TestDuplicatesAreMultipleCandidates(t *testing.T) {
	a := Resolve(Input{Files: []File{f("bom.xlsx", "d"), f("bom (copy).xlsx", "d")}})[k("bom.xlsx")]
	if a.State != StateMultiple || a.Of != nil {
		t.Fatalf("%+v", a)
	}
	if !strings.Contains(a.Explanation, "bom (copy).xlsx") {
		t.Error("explanation must name the candidates")
	}
}

// A confident version series has a likely authority: the profile's current
// member. Inferred, so its confidence is the profile's and below 1.0.
func TestConfidentSeriesYieldsLikely(t *testing.T) {
	in := Input{
		Files: []File{f("w_rev2.pdf", "d2"), f("w_rev3.pdf", "d3")},
		Groups: []Group{{ArtifactID: "art", Members: []Member{
			{Key: k("w_rev2.pdf"), Confidence: 0.9, Rule: "suffix-version"},
			{Key: k("w_rev3.pdf"), IsCurrent: true, Confidence: 0.9, Rule: "suffix-version"},
		}}},
	}
	for _, loc := range []string{"w_rev2.pdf", "w_rev3.pdf"} {
		a := Resolve(in)[k(loc)]
		if a.State != StateLikely || a.Of == nil || a.Of.Locator != "w_rev3.pdf" || a.Confidence != 0.9 {
			t.Errorf("%s: %+v", loc, a)
		}
	}
}

// Below the threshold the series is a compare-set. Its "current" member is
// a guess, and a guess is not a likely authority.
func TestLowConfidenceSeriesIsMultiple(t *testing.T) {
	in := Input{
		Files: []File{f("notes v2.pdf", "a"), f("notes v2 FINAL.pdf", "b")},
		Groups: []Group{{ArtifactID: "art", Members: []Member{
			{Key: k("notes v2.pdf"), Confidence: 0.3, Rule: "compare-set"},
			{Key: k("notes v2 FINAL.pdf"), IsCurrent: true, Confidence: 0.3, Rule: "compare-set"},
		}}},
	}
	if a := Resolve(in)[k("notes v2.pdf")]; a.State != StateMultiple {
		t.Fatalf("%+v", a)
	}
}

// A person's word beats the profile, and beats content.
func TestAssertionDeclares(t *testing.T) {
	in := Input{
		Files: []File{f("bom.xlsx", "d"), f("bom (copy).xlsx", "d")},
		Assertions: []Assertion{{ID: "ast_1", Kind: KindAuthority, Subject: k("bom.xlsx"),
			ActorID: "daniel", Reason: "the one we build from", Evidence: []string{"obs_bom.xlsx"}}},
	}
	for _, loc := range []string{"bom.xlsx", "bom (copy).xlsx"} {
		a := Resolve(in)[k(loc)]
		if a.State != StateDeclared || a.Of.Locator != "bom.xlsx" || a.Confidence != 1.0 || a.AssertionID != "ast_1" {
			t.Errorf("%s: %+v", loc, a)
		}
	}
}

func TestConflictingAssertionsAreMultiple(t *testing.T) {
	in := Input{
		Files: []File{f("a", "d"), f("b", "d")},
		Assertions: []Assertion{
			{ID: "1", Kind: KindAuthority, Subject: k("a"), ActorID: "x", Evidence: []string{"obs_a"}},
			{ID: "2", Kind: KindAuthority, Subject: k("b"), ActorID: "y", Evidence: []string{"obs_b"}},
		},
	}
	a := Resolve(in)[k("a")]
	if a.State != StateMultiple || !strings.Contains(a.Explanation, "retracted") {
		t.Fatalf("%+v; two declarations must surface as a conflict to resolve, not a silent pick", a)
	}
}

// "Not the authority" on the profile's current member removes the likely
// answer without inventing another.
func TestDenialRemovesLikely(t *testing.T) {
	in := Input{
		Files: []File{f("w_rev2.pdf", "d2"), f("w_rev3.pdf", "d3")},
		Groups: []Group{{ArtifactID: "art", Members: []Member{
			{Key: k("w_rev2.pdf"), Confidence: 0.9, Rule: "suffix-version"},
			{Key: k("w_rev3.pdf"), IsCurrent: true, Confidence: 0.9, Rule: "suffix-version"},
		}}},
		Assertions: []Assertion{{ID: "n", Kind: KindNotAuthority, Subject: k("w_rev3.pdf"), ActorID: "d", Evidence: []string{"obs_w_rev3.pdf"}}},
	}
	a := Resolve(in)[k("w_rev2.pdf")]
	if a.State == StateLikely || a.State == StateDeclared {
		t.Fatalf("%+v; denial must not leave rev3 as likely, and must not promote rev2", a)
	}
}

// Ruling out every copy but one does not declare the survivor.
func TestDenyingAllButOneIsStillNone(t *testing.T) {
	in := Input{
		Files:      []File{f("a", "d"), f("b", "d")},
		Assertions: []Assertion{{ID: "n", Kind: KindNotAuthority, Subject: k("b"), ActorID: "d", Evidence: []string{"obs_b"}}},
	}
	a := Resolve(in)[k("a")]
	if a.State != StateNone || !strings.Contains(a.Explanation, "has not been declared") {
		t.Fatalf("%+v", a)
	}
}

func TestEveryStateExplainsItself(t *testing.T) {
	in := Input{
		Files:      []File{f("a", "d"), f("b", "d"), f("c", "e")},
		Assertions: []Assertion{{ID: "1", Kind: KindAuthority, Subject: k("a"), ActorID: "x", Reason: "r", Evidence: []string{"obs_a"}}},
	}
	for key, a := range Resolve(in) {
		if a.Explanation == "" {
			t.Errorf("%s: %s with no explanation", key, a.State)
		}
	}
}

// A thousand identical files must not each carry a thousand-entry record.
func TestMultipleCandidatesAreBounded(t *testing.T) {
	var files []File
	for i := 0; i < 1000; i++ {
		files = append(files, f(fmt.Sprintf("copy%04d.bin", i), "same"))
	}
	a := Resolve(Input{Files: files})[k("copy0000.bin")]
	if a.State != StateMultiple {
		t.Fatalf("%s", a.State)
	}
	if len(a.Evidence) > maxNamedPeers+1 {
		t.Errorf("%d evidence ids cited; the cost of citing every peer is quadratic", len(a.Evidence))
	}
	if !strings.Contains(a.Explanation, "1000 files") || !strings.Contains(a.Explanation, "and 995 more") {
		t.Errorf("explanation must give the total and say the list is cut: %s", a.Explanation)
	}
}
