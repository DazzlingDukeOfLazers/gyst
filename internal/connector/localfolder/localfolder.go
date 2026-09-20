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

	"github.com/DazzlingDukeOfLazers/gyst/internal/discover"
	"github.com/DazzlingDukeOfLazers/gyst/internal/manifest"
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
	Root          string
	SourceID      string
	ContentLevel  string
	Egress        string
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

	// Now is the pass clock. Every observation the pass produces carries it,
	// and the caller records it as the pass's started_at, so the two agree by
	// construction. Zero means time.Now().
	Now time.Time
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
	// Symlinks were seen and deliberately not followed. They are not a
	// hole in coverage: nothing beneath a symlink is in this source, by
	// policy. Counting them as skipped cost every tree with a
	// node_modules/.bin its complete status and its tombstones.
	Symlinks int
	// Placeholders are files present in the tree whose content is not on
	// disk: a cloud sync engine holds the bytes elsewhere. They are observed
	// by metadata only and never opened, because opening one downloads it.
	Placeholders int
	// Manifests and MarkedFolders count the project evidence found: parsed
	// .gyst/project.yaml files and folders carrying a native project marker.
	Manifests     int
	MarkedFolders int
	Bytes         int64
	// HashedBytes is what was actually read. On an incremental pass it is far
	// below Bytes, and the gap is the point of Known.
	HashedBytes int64

	// Seen is every locator this pass actually looked at, including unchanged
	// ones. Deletion detection is the set difference between known state and
	// this -- which is only sound when Complete is true and Skipped is zero.
	Seen map[string]bool

	// Pass is the single clock reading shared by every observation this scan
	// produces, tombstones included. Rename detection pairs a disappearance
	// with an arrival only within one pass, and it identifies a pass by
	// observed_at -- so two clock readings in one scan silently split it in
	// two and no rename is ever found.
	Pass time.Time

	// IgnoredPaths are locators excluded by policy rather than absent. A newly
	// ignored file disappears from Seen exactly like a deleted one, and
	// tombstoning it would assert it was removed from the source when it was
	// only removed from view.
	IgnoredPaths map[string]bool
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
	// An unopenable root is not an empty tree. Walking it would produce a
	// complete pass that saw nothing, and a complete pass that saw nothing
	// tombstones every known file. An unmounted share must never do that.
	if info, serr := os.Stat(root); serr != nil {
		return nil, &UnavailableError{Root: root, Err: serr}
	} else if !info.IsDir() {
		return nil, &UnavailableError{Root: root, Err: fmt.Errorf("not a directory")}
	}
	ig, err := loadIgnores(root)
	if err != nil {
		return nil, err
	}

	now := opts.Now.UTC()
	if opts.Now.IsZero() {
		now = time.Now().UTC()
	}
	res := &Result{Complete: true, Pass: now,
		Seen: map[string]bool{}, IgnoredPaths: map[string]bool{}}

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory is a gap in coverage, not a reason to
			// abandon the scan. Report and continue.
			res.Skipped++
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = observe.NormalizeLocator(rel)

		if d.IsDir() {
			if rel == "." {
				// The root itself: only its markers matter.
				res.observeMarkers(p, ".", opts, now)
				return nil
			}
			// Never follow a symlinked directory: it can leave the configured
			// root entirely, which policy forbids.
			if ig.match(rel, true) {
				res.Ignored++
				res.IgnoredPaths[rel] = true
				return filepath.SkipDir
			}
			if rel == ".git" || strings.HasSuffix(rel, "/.git") {
				return filepath.SkipDir
			}
			res.observeMarkers(p, rel, opts, now)
			return nil
		}
		if !d.Type().IsRegular() {
			if d.Type()&fs.ModeSymlink != 0 {
				res.Symlinks++
			} else {
				res.Skipped++
			}
			return nil
		}
		if ig.match(rel, false) {
			res.Ignored++
			res.IgnoredPaths[rel] = true
			return nil
		}
		if opts.Cursor != "" && rel <= opts.Cursor {
			return nil
		}
		// Files, not observations: a marked folder's observation must not
		// eat into the budget, or a tree of exactly the cap comes out
		// partial by the number of its repositories.
		if res.Scanned >= opts.MaxFiles {
			res.Complete = false
			return filepath.SkipAll
		}

		info, serr := d.Info()
		if serr != nil {
			res.Skipped++
			return nil
		}
		res.Bytes += info.Size()
		res.Seen[rel] = true

		// Compare against known state before touching the file's contents.
		nv := nativeVersion(info)
		known, seen := opts.Known[rel]
		if seen && known.NativeVersion == nv.Value {
			res.Unchanged++
			res.NextCursor = rel
			// A manifest is re-read even when unchanged. Its observation is
			// derived deterministically from the same version, so a repeat
			// collides harmlessly in the log; what this buys is that a
			// manifest first seen by an older scanner, or one whose
			// observation was never projected, cannot go missing. One small
			// file per project per pass.
			if manifest.IsManifest(rel) && !isPlaceholder(info) {
				if m, ok := observeManifest(p, rel, info, nv, known.Seq, opts, now); ok {
					res.Observations = append(res.Observations, m)
					res.Manifests++
				}
			}
			return nil
		}

		if !stable(p, info) {
			// Emitting a half-written file as settled state would put a wrong
			// digest in an immutable log. Leave it for the next scan.
			res.Unstable++
			return nil
		}

		obs, placeholder, oerr := observeFile(p, rel, info, nv, known.Seq, opts, now)
		if oerr != nil {
			res.Skipped++
			return nil
		}
		res.Observations = append(res.Observations, obs)
		res.NextCursor = rel
		res.Scanned++
		if placeholder {
			res.Placeholders++
		} else if obs.Subject.Version.ContentDigest != nil {
			res.HashedBytes += info.Size()
		}
		if manifest.IsManifest(rel) && !placeholder {
			if m, ok := observeManifest(p, rel, info, nv, known.Seq, opts, now); ok {
				res.Observations = append(res.Observations, m)
				res.Manifests++
			}
		}
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

// observeFile builds the observation for one file. The returned bool reports
// a placeholder: a file whose content a sync engine holds elsewhere. Such a
// file is observed by metadata only and is never opened, whatever the policy
// says, because opening it is a download the user did not ask for.
func observeFile(abs, rel string, info fs.FileInfo, nv observe.NativeVersion,
	priorSeq int64, opts Options, now time.Time) (observe.Observation, bool, error) {

	version := &observe.Version{SizeBytes: info.Size()}
	payload := map[string]any{
		"extension": path.Ext(rel),
		"mode":      info.Mode().String(),
	}

	claimType := "file.metadata"
	warnings := []string{}
	placeholder := isPlaceholder(info)

	switch {
	case placeholder:
		payload["placeholder"] = true
		warnings = append(warnings, "content not present locally: cloud placeholder, not read")
	case observe.PermitsDigest(opts.ContentLevel):
		sum, err := hashFile(abs)
		if err != nil {
			return observe.Observation{}, false, err
		}
		version.ContentDigest = &observe.Digest{Algo: "sha256", Hex: sum}
		claimType = "file.content_fingerprint"
	default:
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
			Type:    claimType,
			Payload: payload,
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
	return obs, placeholder, nil
}

// observeMarkers records the native project markers a directory carries: a
// .git, a go.mod, a KiCad project file. The markers are facts about the
// folder, recorded at confidence 1.0; that they suggest a project is an
// interpretation the projection makes at lower confidence.
//
// One extra directory read per directory. Cheap, and the alternative --
// deriving markers from the file observations later -- cannot see .git,
// which the walk never enters.
func (r *Result) observeMarkers(abs, rel string, opts Options, now time.Time) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return
	}
	markers := discover.Markers(entries)
	if len(markers) == 0 || discover.Vendored(rel) {
		// A package.json inside node_modules marks a dependency, not a
		// project. Dogfooding on real trees produced 1,745 such projects.
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	obs := observe.Observation{
		SchemaVersion: observe.SchemaVersion,
		ObservedAt:    now,
		Source: observe.Source{
			SourceID: opts.SourceID, Connector: ConnectorName, ConnectorVersion: ConnectorVersion,
		},
		Subject: observe.ArtifactRef{
			Kind: "folder",
			Location: observe.Location{
				SourceID: opts.SourceID,
				Locator:  rel,
				NativeVersion: observe.NativeVersion{Scheme: "mtime",
					Value: fmt.Sprintf("%d", info.ModTime().UTC().Unix())},
			},
		},
		Claim: observe.Claim{
			Type:    "folder.metadata",
			Payload: map[string]any{"markers": markers},
		},
		Extractor: observe.Extractor{
			Name: "project-markers", Version: ConnectorVersion,
			OutputSchema: "gyst.claim.folder.metadata/0.1.0",
			Warnings:     []string{}, Confidence: 1.0,
		},
		Policy: policyFor(opts),
		Visibility: observe.Visibility{
			Labels: []string{"src:" + opts.SourceID + ":read"}, SourceACLComplete: true,
		},
	}
	obs.ObservationID = observe.DeriveID(&obs, 0)
	r.Observations = append(r.Observations, obs)
	r.MarkedFolders++
}

// observeManifest reads a .gyst/project.yaml and records what it declares.
//
// The file is read under any content level except exclude. It is not
// engineering data: it is the tree's owners describing the tree to Gyst,
// and a policy that forbids Gyst from reading its own configuration would
// forbid the project from ever being declared. Under exclude the path is
// never enumerated in the first place.
//
// A manifest that cannot be parsed is still observed, with valid=false and
// the error, so a broken manifest is visible rather than silently absent.
func observeManifest(abs, rel string, info fs.FileInfo, nv observe.NativeVersion,
	priorSeq int64, opts Options, now time.Time) (observe.Observation, bool) {

	body, err := os.ReadFile(abs)
	if err != nil {
		return observe.Observation{}, false
	}
	payload := map[string]any{"valid": true}
	warnings := []string{}
	confidence := 1.0
	m, warn, perr := manifest.Parse(body, manifest.Dir(rel))
	warnings = append(warnings, warn...)
	if perr != nil {
		payload["valid"] = false
		payload["error"] = perr.Error()
		confidence = 0
	} else {
		payload["id"] = m.ID
		payload["name"] = m.Name
		payload["description"] = m.Description
		payload["members"] = m.Members
		payload["owners"] = m.Owners
	}

	version := &observe.Version{SizeBytes: info.Size()}
	if observe.PermitsDigest(opts.ContentLevel) {
		sum := sha256.Sum256(body)
		version.ContentDigest = &observe.Digest{Algo: "sha256", Hex: hex.EncodeToString(sum[:])}
	}
	obs := observe.Observation{
		SchemaVersion: observe.SchemaVersion,
		ObservedAt:    now,
		Source: observe.Source{
			SourceID: opts.SourceID, Connector: ConnectorName, ConnectorVersion: ConnectorVersion,
		},
		Subject: observe.ArtifactRef{
			Kind:     "file",
			Location: observe.Location{SourceID: opts.SourceID, Locator: rel, NativeVersion: nv},
			Version:  version,
		},
		Claim: observe.Claim{Type: "project.manifest", Payload: payload},
		Extractor: observe.Extractor{
			Name: "project-manifest", Version: "0.1.0",
			OutputSchema: "gyst.claim.project.manifest/0.1.0",
			Warnings:     warnings, Confidence: confidence,
		},
		Policy: policyFor(opts),
		Visibility: observe.Visibility{
			Labels: []string{"src:" + opts.SourceID + ":read"}, SourceACLComplete: true,
		},
	}
	obs.ObservationID = observe.DeriveID(&obs, priorSeq)
	return obs, true
}

func policyFor(opts Options) observe.Policy {
	return observe.Policy{
		ContentLevel:           opts.ContentLevel,
		Egress:                 opts.Egress,
		EffectivePolicyVersion: opts.PolicyVersion,
	}
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
