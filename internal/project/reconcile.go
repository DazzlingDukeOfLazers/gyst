package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	roots, err := sourceRoots(ctx, s)
	if err != nil {
		return st, err
	}

	rows, err := s.Pool().Query(ctx, `
		SELECT seq, observation_id, source_id, native_version_value, claim_payload
		FROM observations WHERE claim_type='git.commit' ORDER BY seq`)
	if err != nil {
		return st, err
	}
	defer rows.Close()

	type commitRow struct {
		seq     int64
		obsID   string
		source  string
		oid     string
		payload map[string]any
	}
	var commits []commitRow
	for rows.Next() {
		var c commitRow
		var raw []byte
		if err := rows.Scan(&c.seq, &c.obsID, &c.source, &c.oid, &raw); err != nil {
			return st, err
		}
		if err := json.Unmarshal(raw, &c.payload); err != nil {
			return st, err
		}
		commits = append(commits, c)
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return st, err
	}
	defer tx.Rollback(ctx)

	for _, c := range commits {
		authored := str(c.payload["authored_at"])
		if _, err := tx.Exec(ctx, `
			INSERT INTO commits (source_id, oid, seq, observation_id, author, message, authored_at, parents)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (source_id, oid) DO UPDATE SET
				seq=EXCLUDED.seq, observation_id=EXCLUDED.observation_id`,
			c.source, c.oid, c.seq, c.obsID, str(c.payload["author"]),
			str(c.payload["message"]), authored, strSlice(c.payload["parents"])); err != nil {
			return st, err
		}
		st.Commits++

		for _, p := range strSlice(c.payload["changed_paths"]) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO commit_files (source_id, oid, locator) VALUES ($1,$2,$3)
				ON CONFLICT DO NOTHING`, c.source, c.oid, p); err != nil {
				return st, err
			}
			st.Files++

			// Resolve both sides to absolute paths and look for a file the
			// filesystem scan already knows about.
			abs := filepath.Join(roots[c.source], p)
			target, found, err := fileAtPath(ctx, s, roots, abs)
			if err != nil {
				return st, err
			}
			if !found {
				// The commit touched a path no scan has observed: the file was
				// deleted later, or lives outside every configured root. The
				// commit_files row stands on its own; no relation is invented.
				st.Orphaned++
				continue
			}

			relID := "rel_" + shortHash("contains", c.source+"@"+c.oid, target.sourceID+"\x00"+target.locator)
			if _, err := tx.Exec(ctx, `
				INSERT INTO relations (identity_policy_version, relation_id, type,
					from_source, from_locator, to_source, to_locator,
					precedence, actor_kind, actor_id, evidence, confidence, explanation)
				VALUES (NULL,$1,'contains',$2,$3,$4,$5,'source_native_marker','connector','git/0.1.0',$6,1.0,$7)
				ON CONFLICT (relation_id) DO NOTHING`,
				relID, c.source, c.source+"@"+c.oid, target.sourceID, target.locator,
				[]string{c.obsID, target.obsID},
				"commit "+short12(c.oid)+" changed this path; matched to the scanned file by resolved filesystem path",
			); err != nil {
				return st, err
			}
			st.Relations++
			st.Bridged++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return st, err
	}
	return st, nil
}

type fileTarget struct {
	sourceID string
	locator  string
	obsID    string
}

// fileAtPath finds a currently-present file whose resolved path equals abs.
func fileAtPath(ctx context.Context, s *store.Store, roots map[string]string, abs string) (fileTarget, bool, error) {
	for sourceID, root := range roots {
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == "" || rel[0] == '.' {
			continue // outside this source's root
		}
		var t fileTarget
		err = s.Pool().QueryRow(ctx, `
			SELECT c.source_id, c.locator, coalesce(o.observation_id,'')
			FROM current_files c
			LEFT JOIN observations o ON o.seq = c.latest_seq
			WHERE c.source_id=$1 AND c.locator=$2 AND c.present`, sourceID, rel).
			Scan(&t.sourceID, &t.locator, &t.obsID)
		if err == nil {
			return t, true, nil
		}
	}
	return fileTarget{}, false, nil
}

func sourceRoots(ctx context.Context, s *store.Store) (map[string]string, error) {
	rows, err := s.Pool().Query(ctx, `SELECT source_id, root FROM sources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, root string
		if err := rows.Scan(&id, &root); err != nil {
			return nil, err
		}
		out[id] = root
	}
	return out, rows.Err()
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
