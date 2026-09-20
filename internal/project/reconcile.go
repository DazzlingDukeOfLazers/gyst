package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// CommitStats reports what a commit projection pass did.
type CommitStats struct {
	Commits   int
	Files     int
	Relations int
	Bridged   int
	Orphaned  int
}

// ProjectCommits folds git.commit observations into the commits and
// commit_files tables, then reconciles those paths against files observed by
// filesystem scans.
//
// Reconciliation is the point. The same bytes are observed twice -- once by the
// local-folder connector as a path with an mtime, once by the Git connector as
// a path inside a commit -- and until the two are related, "what changed?" can
// be answered from either side but never from both.
//
// Matching is by resolved filesystem path, not by locator string. A locator is
// only meaningful relative to its source's root: the scan calls the file
// "firmware/README.md" and Git calls it "README.md", and they are the same file
// only because the Git repository sits at "firmware/" inside the scanned tree.
func ProjectCommits(ctx context.Context, s *store.Store) (CommitStats, error) {
	var st CommitStats

	roots, err := s.SourceRoots(ctx)
	if err != nil {
		return st, err
	}
	commits, err := s.CommitObservations(ctx)
	if err != nil {
		return st, err
	}
	present, err := s.PresentFiles(ctx)
	if err != nil {
		return st, err
	}
	byPath := map[string]store.PresentFile{}
	for _, f := range present {
		byPath[f.SourceID+"\x00"+f.Locator] = f
	}

	var rows []store.CommitRow
	var relations []store.RelationRow
	for _, c := range commits {
		row := store.CommitRow{
			SourceID: c.SourceID, OID: c.OID, Seq: c.Seq, ObsID: c.ObsID,
			Author: str(c.Payload["author"]), Message: str(c.Payload["message"]),
			AuthoredAt: str(c.Payload["authored_at"]), Parents: strSlice(c.Payload["parents"]),
			ChangedPaths: strSlice(c.Payload["changed_paths"]),
		}
		rows = append(rows, row)
		st.Commits++
		for _, p := range row.ChangedPaths {
			st.Files++
			// Resolve both sides to absolute paths and look for a file the
			// filesystem scan already knows about.
			abs := filepath.Join(roots[c.SourceID], p)
			target, found := fileAtPath(roots, byPath, abs)
			if !found {
				// The commit touched a path no scan has observed: the file was
				// deleted later, or lives outside every configured root. The
				// commit_files row stands on its own; no relation is invented.
				st.Orphaned++
				continue
			}
			relations = append(relations, store.RelationRow{
				RelationID: "rel_" + shortHash("contains", c.SourceID+"@"+c.OID, target.SourceID+"\x00"+target.Locator),
				Type:       "contains",
				FromSource: c.SourceID, FromLocator: c.SourceID + "@" + c.OID,
				ToSource: target.SourceID, ToLocator: target.Locator,
				Precedence: "source_native_marker", ActorKind: "connector", ActorID: "git/0.1.0",
				Evidence: []string{c.ObsID, target.ObsID}, Confidence: 1.0,
				Explanation: "commit " + short12(c.OID) + " changed this path; matched to the scanned file by resolved filesystem path",
			})
			st.Relations++
			st.Bridged++
		}
	}
	if err := s.WriteCommits(ctx, rows, relations); err != nil {
		return st, err
	}
	return st, nil
}

// fileAtPath finds a currently-present file whose resolved path equals abs.
func fileAtPath(roots map[string]string, byPath map[string]store.PresentFile, abs string) (store.PresentFile, bool) {
	for sourceID, root := range roots {
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == "" || rel[0] == '.' {
			continue // outside this source's root
		}
		if f, ok := byPath[sourceID+"\x00"+filepath.ToSlash(rel)]; ok {
			return f, true
		}
	}
	return store.PresentFile{}, false
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// strSlice always returns a non-nil slice. A nil one marshals to SQL NULL,
// which a root commit -- legitimately parentless -- would hit on every run.
func strSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func shortHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func short12(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
