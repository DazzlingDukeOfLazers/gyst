package report

import (
	"testing"

	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

func TestBoundaryOf(t *testing.T) {
	if b := boundaryOf("manifest", "s", "engineering/widget/.gyst/project.yaml"); b.Locator != "engineering/widget" {
		t.Errorf("manifest boundary %q", b.Locator)
	}
	if b := boundaryOf("manifest", "s", ".gyst/project.yaml"); b.Locator != "" {
		t.Errorf("root manifest boundary %q", b.Locator)
	}
	if b := boundaryOf("native-marker", "s", "raves-of-qud/godot"); b.Locator != "raves-of-qud/godot" {
		t.Errorf("marker boundary %q", b.Locator)
	}
	if b := boundaryOf("native-marker", "s", "."); b.Locator != "" {
		t.Errorf("root marker boundary %q", b.Locator)
	}
}

// Physical nesting is a fact about folders. The nearest enclosing
// boundary wins, a root encloses everything in its source, a sibling
// with a shared name prefix does not count, and another source does not
// count.
func TestPhysicallyWithin(t *testing.T) {
	ps := []Project{
		{ProjectID: "root", Boundary: Boundary{"s", ""}},
		{ProjectID: "raves", Boundary: Boundary{"s", "raves-of-qud"}},
		{ProjectID: "raves-godot", Boundary: Boundary{"s", "raves-of-qud/godot"}},
		{ProjectID: "raves-godot-mod", Boundary: Boundary{"s", "raves-of-qud/godot/mod"}},
		{ProjectID: "raves2", Boundary: Boundary{"s", "raves-of-qud-2"}},
		{ProjectID: "elsewhere", Boundary: Boundary{"t", "raves-of-qud/godot/inner"}},
	}
	placeBoundaries(ps)
	want := map[string]string{"root": "", "raves": "root", "raves-godot": "raves", "raves-godot-mod": "raves-godot", "raves2": "root", "elsewhere": ""}
	for _, p := range ps {
		got := ""
		if p.PhysicallyWithin != nil {
			got = *p.PhysicallyWithin
		}
		if got != want[p.ProjectID] {
			t.Errorf("%s within %q, want %q", p.ProjectID, got, want[p.ProjectID])
		}
	}
}

func TestFindingProjectsFromFileSubjects(t *testing.T) {
	files := []File{
		{SourceID: "s", Locator: "a/x.pdf", Projects: []FileProject{{ProjectID: "A"}, {ProjectID: "root"}}},
		{SourceID: "s", Locator: "b/x.pdf", Projects: []FileProject{{ProjectID: "B"}}},
		{SourceID: "s", Locator: "c/x.pdf"},
	}
	m := fileMemberships(files)
	sub := func(loc, kind string) observe.ArtifactRef {
		return observe.ArtifactRef{Kind: kind, Location: observe.Location{SourceID: "s", Locator: loc}}
	}
	ids, cross := findingProjects(findings.Finding{Subjects: []observe.ArtifactRef{sub("a/x.pdf", "file"), sub("b/x.pdf", "file")}}, m)
	if len(ids) != 3 || !cross || ids[0] != "A" || ids[2] != "root" {
		t.Errorf("cross-project: %v %v", ids, cross)
	}
	ids, cross = findingProjects(findings.Finding{Subjects: []observe.ArtifactRef{sub("a/x.pdf", "file")}}, m)
	if len(ids) != 2 || !cross {
		t.Errorf("one file in two projects is cross-project too: %v %v", ids, cross)
	}
	ids, cross = findingProjects(findings.Finding{Subjects: []observe.ArtifactRef{sub("c/x.pdf", "file"), sub(".", "folder")}}, m)
	if len(ids) != 0 || cross {
		t.Errorf("no memberships and a folder subject: %v %v", ids, cross)
	}
}
