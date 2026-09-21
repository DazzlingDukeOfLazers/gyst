package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DazzlingDukeOfLazers/gyst/internal/discover"
	"github.com/DazzlingDukeOfLazers/gyst/internal/manifest"
)

// The fixture's project expectations are hand-authored in
// testdata/generate.py, independently of this code. Resolving membership from
// the generated tree and comparing is what makes them a check rather than a
// restatement.
type fixtureInventory struct {
	Files []struct {
		Path     string   `json:"path"`
		Ignored  bool     `json:"ignored"`
		Projects []string `json:"projects"`
		Note     string   `json:"note"`
	} `json:"files"`
	Projects []struct {
		ID         string   `json:"id"`
		Basis      string   `json:"basis"`
		DeclaredBy string   `json:"declared_by"`
		Folder     string   `json:"folder"`
		Members    []string `json:"members"`
		Markers    []string `json:"markers"`
	} `json:"projects"`
	SuppressedMarkers []string `json:"suppressed_markers"`
}

const fixtureSource = "src_fixture"

func loadFixture(t *testing.T) (fixtureInventory, string) {
	t.Helper()
	root := filepath.Join("..", "..", "testdata")
	blob, err := os.ReadFile(filepath.Join(root, "expected-inventory.json"))
	if err != nil {
		t.Skipf("fixture not generated (run testdata/generate.py): %v", err)
	}
	var inv fixtureInventory
	if err := json.Unmarshal(blob, &inv); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(root, "tree")
	if _, err := os.Stat(tree); err != nil {
		t.Skipf("fixture tree not generated: %v", err)
	}
	return inv, tree
}

// evidenceFromTree does what the scanner and the projector's loaders do,
// without a database: parse every manifest in the tree and list every folder
// with markers.
func evidenceFromTree(t *testing.T, tree string) ([]ManifestEvidence, []MarkerEvidence) {
	t.Helper()
	var manifests []ManifestEvidence
	var markers []MarkerEvidence
	err := filepath.WalkDir(tree, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(tree, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || strings.HasSuffix(rel, "/.git") {
				return filepath.SkipDir
			}
			entries, err := os.ReadDir(p)
			if err != nil {
				return err
			}
			if m := discover.Markers(entries); len(m) > 0 {
				markers = append(markers, MarkerEvidence{SourceID: fixtureSource, Locator: rel, ObsID: "obs_" + rel, Markers: m})
			}
			return nil
		}
		if manifest.IsManifest(rel) {
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			ev := ManifestEvidence{SourceID: fixtureSource, Locator: rel, ObsID: "obs_" + rel}
			m, _, perr := manifest.Parse(body, manifest.Dir(rel))
			if perr != nil {
				ev.Error = perr.Error()
			} else {
				ev.Valid, ev.ID, ev.Name, ev.Description, ev.Members = true, m.ID, m.Name, m.Description, m.Members
			}
			manifests = append(manifests, ev)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifests, markers
}

func TestProjectsMatchFixtureExpectations(t *testing.T) {
	inv, tree := loadFixture(t)
	manifests, markers := evidenceFromTree(t, tree)
	plan := ResolveProjects(manifests, markers, nil)

	// Marker projects are named in the fixture by folder, not by their
	// derived id. Map ids to fixture names.
	name := map[string]string{}
	for _, p := range plan.Projects {
		switch p.Basis {
		case BasisManifest:
			name[p.ID] = p.ID
		case BasisNativeMarker:
			name[p.ID] = "marker:" + p.Locator
		}
	}

	// 1. The set of projects.
	var got []string
	for _, p := range plan.Projects {
		got = append(got, name[p.ID]+"/"+p.Basis)
	}
	var want []string
	for _, p := range inv.Projects {
		want = append(want, p.ID+"/"+p.Basis)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("projects\n  got  %v\n  want %v", got, want)
	}
	if plan.SuppressedMarkers != len(inv.SuppressedMarkers) {
		t.Errorf("suppressed markers = %d, fixture expects %d (%v)",
			plan.SuppressedMarkers, len(inv.SuppressedMarkers), inv.SuppressedMarkers)
	}

	// 2. Every file's membership.
	checked, twice := 0, 0
	for _, f := range inv.Files {
		if f.Ignored {
			continue
		}
		var got []string
		for _, p := range plan.Projects {
			for _, m := range p.Members {
				if m.SourceID == fixtureSource && manifest.Match(m.Pattern, f.Path) {
					got = append(got, name[p.ID])
					break
				}
			}
		}
		sort.Strings(got)
		want := append([]string{}, f.Projects...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s\n  projects = %v\n  fixture wants %v\n  note: %s", f.Path, got, want, f.Note)
		}
		if len(want) > 1 {
			twice++
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no files checked")
	}
	if twice == 0 {
		t.Fatal("the fixture has no file in two projects; the many-to-many case is untested")
	}
	t.Logf("checked %d files, %d of them in two projects", checked, twice)
}
