// Package discover finds project roots on disk.
//
// A project root is a directory carrying a marker a build tool or version
// control system left there: .git, go.mod, a KiCad project file. Discovery
// walks configured roots, reports every directory with a marker together
// with where it physically lives, and stops descending at the first marker
// unless told otherwise. It reads directory listings only and never opens a
// file, so it is safe to point at anything.
//
// The result is a list of candidates for a person to register as sources. It
// is not a project: the product's projects are many-to-many with folders and
// declared by manifests, and a marker walk has no basis for that.
package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
)

// Candidate is one directory that looks like a project root.
type Candidate struct {
	Path     string            `json:"path"`
	Markers  []string          `json:"markers"`
	Location location.Location `json:"location"`
	Depth    int               `json:"depth"`
}

// Stats says how much of the walk was possible.
type Stats struct {
	Roots            []string `json:"roots"`
	RootsUnavailable []string `json:"roots_unavailable,omitempty"`
	Dirs             int      `json:"dirs_visited"`
	Unreadable       int      `json:"dirs_unreadable"`
	Pruned           int      `json:"dirs_pruned"`
}

type Options struct {
	Roots    []string
	MaxDepth int  // directories below a root to descend; 0 means DefaultDepth
	Nested   bool // keep descending below a directory that has a marker
	// Classify decides where a candidate lives. Nil means location.Probe.
	Classify func(string) location.Location
}

const DefaultDepth = 6

// Exact file or directory names that mark a project root.
var nameMarkers = map[string]string{
	".git":           "git",
	".gyst":          "gyst",
	"go.mod":         "go",
	"Cargo.toml":     "rust",
	"package.json":   "node",
	"pyproject.toml": "python",
	"setup.py":       "python",
	"CMakeLists.txt": "cmake",
	"pom.xml":        "maven",
	"build.gradle":   "gradle",
	"Gemfile":        "ruby",
	"mix.exs":        "elixir",
	"Package.swift":  "swift",
	"composer.json":  "php",
	"Makefile":       "make",
}

// Suffixes that mark a project root when any entry carries them.
var suffixMarkers = map[string]string{
	".kicad_pro": "kicad",
	".sln":       "dotnet",
	".csproj":    "dotnet",
	".xcodeproj": "xcode",
	".uproject":  "unreal",
	".godot":     "godot",
}

// Directories never worth descending into: dependency caches, build output,
// version-control internals, and operating-system furniture. A project inside
// one of these is either a vendored copy or an accident.
var pruneNames = map[string]bool{
	"node_modules": true, ".git": true, ".svn": true, ".hg": true,
	".venv": true, "venv": true, "__pycache__": true, ".tox": true,
	"target": true, "dist": true, "build": true, ".build": true,
	".cache": true, ".npm": true, ".cargo": true, ".rustup": true, ".gradle": true, ".m2": true,
	".Trash": true, ".Trashes": true, "$RECYCLE.BIN": true, "System Volume Information": true,
	".Spotlight-V100": true, ".fseventsd": true, ".TemporaryItems": true, ".DocumentRevisions-V100": true,
	"AppData": true, ".godot": true, "site-packages": true,
	// Tool state that holds whole copies of a repository: Claude Code keeps
	// its worktrees under .claude, and each one is the project again.
	".claude": true,
}

// Vendored reports whether a locator lies inside a dependency cache, build
// output, or version-control internals: the same names Find never
// descends into. Files there are observed like any other, but they are
// expected copies of things owned elsewhere: not project boundaries, not
// duplicates worth a finding, not candidates for authority. Dogfooding on
// real trees found 83% of files there.
func Vendored(locator string) bool {
	for _, seg := range strings.Split(locator, "/") {
		if pruneNames[seg] {
			return true
		}
	}
	return false
}

// Directories pruned only when they sit directly under the home directory.
var pruneUnderHome = map[string]bool{
	"Library": true, "Applications": true, "Movies": true, "Music": true, "Pictures": true,
}

// Find walks the roots and returns candidates sorted by path.
func Find(opts Options) ([]Candidate, Stats, error) {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = DefaultDepth
	}
	if opts.Classify == nil {
		opts.Classify = location.Probe
	}
	home, _ := os.UserHomeDir()

	var out []Candidate
	stats := Stats{}
	seen := map[string]bool{}
	for _, r := range opts.Roots {
		root, err := filepath.Abs(r)
		if err != nil {
			return nil, stats, err
		}
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		stats.Roots = append(stats.Roots, root)
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			stats.RootsUnavailable = append(stats.RootsUnavailable, root)
			continue
		}
		walk(root, root, 0, home, &opts, &out, &stats)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, stats, nil
}

func walk(root, dir string, depth int, home string, opts *Options, out *[]Candidate, stats *Stats) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		stats.Unreadable++
		return
	}
	stats.Dirs++

	markers := Markers(entries)
	if len(markers) > 0 {
		*out = append(*out, Candidate{
			Path: dir, Markers: markers, Location: opts.Classify(dir), Depth: depth,
		})
		if !opts.Nested {
			return
		}
	}
	if depth >= opts.MaxDepth {
		return
	}
	for _, e := range entries {
		// A symlinked directory can leave the root entirely. Never follow.
		if !e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		name := e.Name()
		if pruneNames[name] || (dir == home && pruneUnderHome[name]) {
			stats.Pruned++
			continue
		}
		walk(root, filepath.Join(dir, name), depth+1, home, opts, out, stats)
	}
}

// Markers returns the sorted, de-duplicated project-marker labels among a
// directory's entries.
func Markers(entries []fs.DirEntry) []string {
	found := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if label, ok := nameMarkers[name]; ok {
			found[label] = true
			continue
		}
		for suffix, label := range suffixMarkers {
			if strings.HasSuffix(name, suffix) {
				found[label] = true
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	out := make([]string, 0, len(found))
	for l := range found {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
