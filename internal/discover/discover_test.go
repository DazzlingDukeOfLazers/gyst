package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
)

func mk(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if filepath.Ext(p) == "" && !filepath.IsAbs(p) && p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func fixed(string) location.Location {
	return location.Location{Kind: location.KindLocal, Provider: "test", Evidence: "test"}
}

func paths(cs []Candidate, root string) []string {
	var out []string
	for _, c := range cs {
		rel, _ := filepath.Rel(root, c.Path)
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func TestFindsMarkersAndStopsAtFirst(t *testing.T) {
	root := t.TempDir()
	mk(t, root,
		"repos/gyst/.git/",
		"repos/gyst/go.mod",
		"repos/gyst/site/package.json", // nested: must not appear by default
		"boards/widget/widget.kicad_pro",
		"docs/notes.txt",
	)
	cs, stats, err := Find(Options{Roots: []string{root}, Classify: fixed})
	if err != nil {
		t.Fatal(err)
	}
	got := paths(cs, root)
	want := []string{"boards/widget", "repos/gyst"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("candidates %v, want %v", got, want)
	}
	if m := cs[1].Markers; len(m) != 2 || m[0] != "git" || m[1] != "go" {
		t.Errorf("gyst markers %v, want [git go]", m)
	}
	if cs[0].Markers[0] != "kicad" {
		t.Errorf("widget markers %v", cs[0].Markers)
	}
	if stats.Dirs == 0 {
		t.Error("no directories counted")
	}
}

func TestNestedFindsProjectsInsideProjects(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "repos/gyst/.git/", "repos/gyst/site/package.json")
	cs, _, err := Find(Options{Roots: []string{root}, Classify: fixed, Nested: true})
	if err != nil {
		t.Fatal(err)
	}
	got := paths(cs, root)
	if len(got) != 2 || got[1] != "repos/gyst/site" {
		t.Fatalf("nested candidates %v", got)
	}
}

func TestPrunesDependencyAndBuildDirs(t *testing.T) {
	root := t.TempDir()
	mk(t, root,
		"app/package.json",
		"app/node_modules/left-pad/package.json",
		"other/target/debug/build/x/Cargo.toml",
		"other/dist/package.json",
	)
	cs, stats, err := Find(Options{Roots: []string{root}, Classify: fixed, Nested: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(cs, root); len(got) != 1 || got[0] != "app" {
		t.Fatalf("candidates %v; vendored and built copies are not projects", got)
	}
	if stats.Pruned == 0 {
		t.Error("nothing recorded as pruned")
	}
}

func TestDepthLimit(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "a/b/c/d/go.mod")
	cs, _, _ := Find(Options{Roots: []string{root}, Classify: fixed, MaxDepth: 2})
	if len(cs) != 0 {
		t.Fatalf("found %v beyond the depth limit", paths(cs, root))
	}
	cs, _, _ = Find(Options{Roots: []string{root}, Classify: fixed, MaxDepth: 4})
	if len(cs) != 1 {
		t.Fatalf("not found within the depth limit")
	}
}

func TestSymlinkedDirectoryIsNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mk(t, outside, "escaped/go.mod")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	cs, _, _ := Find(Options{Roots: []string{root}, Classify: fixed})
	if len(cs) != 0 {
		t.Fatalf("followed a symlink out of the root: %v", paths(cs, root))
	}
}

func TestUnavailableRootIsReportedNotFatal(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "p/go.mod")
	missing := filepath.Join(root, "not-mounted")
	cs, stats, err := Find(Options{Roots: []string{missing, root}, Classify: fixed})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.RootsUnavailable) != 1 || stats.RootsUnavailable[0] != missing {
		t.Errorf("unavailable roots %v", stats.RootsUnavailable)
	}
	if len(cs) != 1 {
		t.Errorf("the available root was not walked")
	}
}
