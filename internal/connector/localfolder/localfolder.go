// Package localfolder implements the local-folder connector: the smallest real
// implementation of discover(cursor) -> observations, next_cursor.
//
// It is read-only. Nothing in this package opens a file for writing, renames,
// or removes anything, and that is a property worth keeping deliberately.
package localfolder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

const (
	ConnectorName    = "local-folder"
	ConnectorVersion = "0.1.0"

	// A file whose mtime is younger than this may still be being written, so it
	// is re-stat'ed before hashing. Older files are assumed settled.
	//
	// Checking every file twice would double the syscall cost of a scan for no
	// benefit: a file untouched for a minute is not mid-write. This is the
	// cheap approximation of the "detect file stability" requirement, and it is
	// an approximation -- a slow writer that pauses longer than the window can
	// still be caught mid-write. Real durability needs the OS-level file
	// notification path, which is a Windows-specific spike.
	stabilityWindow = 2 * time.Second
	settleDelay     = 150 * time.Millisecond
)

type Options struct {
	Root         string
	SourceID     string
	ContentLevel string
	Egress       string
	PolicyVersion string

	// Cursor resumes a scan. Entries at or before it in walk order are skipped.
	Cursor string

	// Known is what the projection already believes, keyed by locator. A file
	// whose native version matches is unchanged: it is not hashed and no
	// observation is emitted.
	//
	// This is the incremental scan. Without it every pass re-reads and re-hashes
	// the entire tree, which is the dominant cost by orders of magnitude, and
	// re-observing a state already in the log cannot be distinguished from a
	// revert to that state.
	Known map[string]observe.KnownState

	// MaxFiles bounds a single discover call so a huge tree yields in batches
	// rather than buffering everything.
	MaxFiles int
}

type Result struct {
	Observations []observe.Observation
	NextCursor   string
	Complete     bool

	Scanned   int
	Unchanged int
	Skipped   int
	Ignored   int
	Unstable  int
	Bytes     int64
	// HashedBytes is what was actually read. On an incremental pass it is far
	// below Bytes, and the gap is the point of Known.
	HashedBytes int64
}

// Discover walks the root in deterministic order and emits one observation per
// file. Walk order is lexical so that the cursor means something: everything at
// or before it has been emitted.
func Discover(opts Options) (*Result, error) {
	if opts.ContentLevel == "" {
		opts.ContentLevel = observe.ContentFingerprint
	}
	if opts.Egress == "" {
		opts.Egress = "device"
	}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = 100_000
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	ig, err := loadIgnores(root)
	if err != nil {
		return nil, err
	}

	res := &Result{Complete: true}
	now := time.Now().UTC()

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory is a gap in coverage, not a reason to
			// abandon the scan. Report and continue.
			res.Skipped++
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = observe.NormalizeLocator(rel)

		if d.IsDir() {
			// Never follow a symlinked directory: it can leave the configured
			// root entirely, which policy forbids.
			if ig.match(rel, true) {
				res.Ignored++
				return filepath.SkipDir
			}
			if rel == ".git" || strings.HasSuffix(rel, "/.git") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			res.Skipped++
			return nil
		}
		if ig.match(rel, false) {
			res.Ignored++
			return nil
		}
		if opts.Cursor != "" && rel <= opts.Cursor {
			return nil
		}
		if len(res.Observations) >= opts.MaxFiles {
			res.Complete = false
			return filepath.SkipAll
		}

		info, serr := d.Info()
		if serr != nil {
			res.Skipped++
			return nil
		}
		res.Bytes += info.Size()

		// Compare against known state before touching the file's contents.
		nv := nativeVersion(info)
		known, seen := opts.Known[rel]
		if seen && known.NativeVersion == nv.Value {
			res.Unchanged++
			res.NextCursor = rel
			return nil
		}

		if !stable(p, info) {
			// Emitting a half-written file as settled state would put a wrong
			// digest in an immutable log. Leave it for the next scan.
			res.Unstable++
			return nil
		}

		obs, oerr := observeFile(p, rel, info, nv, known.Seq, opts, now)
		if oerr != nil {
			res.Skipped++
			return nil
		}
		res.Observations = append(res.Observations, obs)
		res.NextCursor = rel
		res.Scanned++
		res.HashedBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func nativeVersion(info fs.FileInfo) observe.NativeVersion {
	return observe.NativeVersion{
		Scheme: "mtime_size",
		Value:  fmt.Sprintf("%d:%d", info.ModTime().UTC().Unix(), info.Size()),
	}
}

func observeFile(abs, rel string, info fs.FileInfo, nv observe.NativeVersion,
	priorSeq int64, opts Options, now time.Time) (observe.Observation, error) {

	version := &observe.Version{SizeBytes: info.Size()}

	claimType := "file.metadata"
	warnings := []string{}

	if observe.PermitsDigest(opts.ContentLevel) {
		sum, err := hashFile(abs)
		if err != nil {
			return observe.Observation{}, err
		}
		version.ContentDigest = &observe.Digest{Algo: "sha256", Hex: sum}
		claimType = "file.content_fingerprint"
	} else {
		warnings = append(warnings, "content withheld by effective policy")
	}

	obs := observe.Observation{
		SchemaVersion: observe.SchemaVersion,
		ObservedAt:    now,
		Source: observe.Source{
			SourceID:         opts.SourceID,
			Connector:        ConnectorName,
			ConnectorVersion: ConnectorVersion,
		},
		Subject: observe.ArtifactRef{
			Kind: "file",
			Location: observe.Location{
				SourceID:      opts.SourceID,
				Locator:       rel,
				NativeVersion: nv,
			},
			Version: version,
		},
		Claim: observe.Claim{
			Type: claimType,
			Payload: map[string]any{
				"extension": path.Ext(rel),
				"mode":      info.Mode().String(),
			},
		},
		Extractor: observe.Extractor{
			Name:         "file-fingerprint",
			Version:      ConnectorVersion,
			OutputSchema: "gyst.claim." + claimType + "/0.1.0",
			Warnings:     warnings,
			Confidence:   1.0,
		},
		Policy: observe.Policy{
			ContentLevel:           opts.ContentLevel,
			Egress:                 opts.Egress,
			EffectivePolicyVersion: opts.PolicyVersion,
		},
		Visibility: observe.Visibility{
			Labels:            []string{"src:" + opts.SourceID + ":read"},
			SourceACLComplete: true,
		},
	}
	obs.ObservationID = observe.DeriveID(&obs, priorSeq)
	return obs, nil
}

// stable re-stats a recently modified file to catch one that is still being
// written. Returns false if size or mtime moved.
func stable(p string, first fs.FileInfo) bool {
	if time.Since(first.ModTime()) > stabilityWindow {
		return true
	}
	time.Sleep(settleDelay)
	second, err := os.Stat(p)
	if err != nil {
		return false
	}
	return second.Size() == first.Size() && second.ModTime().Equal(first.ModTime())
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------------------------------------------------------------------------
// .gystignore
// ---------------------------------------------------------------------------

type ignoreRule struct {
	dir     string // directory the rule was declared in, relative to root
	pattern string
	dirOnly bool
}

type ignoreSet struct{ rules []ignoreRule }

// loadIgnores reads every .gystignore under the root. A rule applies to the
// subtree of the directory that declares it.
func loadIgnores(root string) (*ignoreSet, error) {
	set := &ignoreSet{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != ".gystignore" {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		if rel == "." {
			rel = ""
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			set.rules = append(set.rules, ignoreRule{
				dir:     observe.NormalizeLocator(rel),
				pattern: strings.TrimSuffix(line, "/"),
				dirOnly: strings.HasSuffix(line, "/"),
			})
		}
		return nil
	})
	sort.Slice(set.rules, func(i, j int) bool { return set.rules[i].dir < set.rules[j].dir })
	return set, err
}

func (s *ignoreSet) match(rel string, isDir bool) bool {
	for _, r := range s.rules {
		if r.dir != "" && !strings.HasPrefix(rel, r.dir+"/") {
			continue
		}
		sub := strings.TrimPrefix(strings.TrimPrefix(rel, r.dir), "/")
		if r.dirOnly && !isDir {
			// A directory rule also hides everything beneath it.
			if strings.HasPrefix(sub, r.pattern+"/") {
				return true
			}
			continue
		}
		if sub == r.pattern {
			return true
		}
		if ok, _ := path.Match(r.pattern, path.Base(sub)); ok {
			return true
		}
	}
	return false
}
