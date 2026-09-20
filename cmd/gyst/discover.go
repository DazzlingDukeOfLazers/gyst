package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/DazzlingDukeOfLazers/gyst/internal/discover"
	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func cmdDiscover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	var roots multiFlag
	fs.Var(&roots, "root", "directory to search (repeatable; default: home and mounted volumes)")
	depth := fs.Int("depth", discover.DefaultDepth, "how many directories deep to look")
	nested := fs.Bool("nested", false, "keep looking inside a directory that already has a marker")
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	fs.Parse(args)

	if len(roots) == 0 {
		roots = defaultRoots()
	}

	cands, stats, err := discover.Find(discover.Options{
		Roots: roots, MaxDepth: *depth, Nested: *nested,
	})
	if err != nil {
		return err
	}

	// Which candidates are already sources. Discovery works without a
	// database; the annotation is a courtesy when one is reachable.
	registered := map[string]string{}
	if s, err := open(ctx); err == nil {
		if srcs, err := s.Sources(ctx); err == nil {
			for _, src := range srcs {
				registered[filepath.Clean(src.Root)] = src.SourceID
			}
		}
		s.Close()
	}

	if *asJSON {
		type out struct {
			Candidates []discover.Candidate `json:"candidates"`
			Stats      discover.Stats       `json:"stats"`
			Registered map[string]string    `json:"registered_sources"`
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out{cands, stats, registered})
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "LOCATION\tMARKERS\tSOURCE\tPATH")
	for _, c := range cands {
		src := registered[c.Path]
		if src == "" {
			src = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Location, strings.Join(c.Markers, ","), src, displayPath(c.Path))
	}
	w.Flush()

	fmt.Printf("\nsearched   %s\n", strings.Join(stats.Roots, ", "))
	fmt.Printf("found      %d candidate root(s) in %d directories; %d pruned, %d unreadable\n",
		len(cands), stats.Dirs, stats.Pruned, stats.Unreadable)
	for _, r := range stats.RootsUnavailable {
		fmt.Printf("unavailable  %s\n", r)
	}
	byKind := map[location.Kind]int{}
	for _, c := range cands {
		byKind[c.Location.Kind]++
	}
	for _, k := range location.Kinds() {
		if byKind[k] > 0 {
			fmt.Printf("           %d on %s\n", byKind[k], k)
		}
	}
	return nil
}

// defaultRoots is the home directory plus whatever else is mounted: on macOS
// the entries of /Volumes that are not the boot volume, on Linux the usual
// media mount points, on Windows every drive letter that answers.
func defaultRoots() []string {
	roots := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, home)
	}
	switch runtime.GOOS {
	case "darwin":
		entries, _ := os.ReadDir("/Volumes")
		for _, e := range entries {
			p := filepath.Join("/Volumes", e.Name())
			if real, err := filepath.EvalSymlinks(p); err == nil && real != "/" {
				roots = append(roots, p)
			}
		}
	case "linux":
		for _, base := range []string{"/mnt", "/media"} {
			entries, _ := os.ReadDir(base)
			for _, e := range entries {
				if e.IsDir() {
					roots = append(roots, filepath.Join(base, e.Name()))
				}
			}
		}
	case "windows":
		for c := 'D'; c <= 'Z'; c++ {
			p := string(c) + `:\`
			if _, err := os.Stat(p); err == nil {
				roots = append(roots, p)
			}
		}
	}
	return roots
}

// displayPath abbreviates the home directory for reading; the JSON output
// keeps full paths.
func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
