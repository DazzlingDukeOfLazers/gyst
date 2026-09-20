package store

import (
	"context"
	"encoding/json"
)

// CommitObservation is a git.commit log entry with its payload decoded.
type CommitObservation struct {
	Seq      int64
	ObsID    string
	SourceID string
	OID      string
	Payload  map[string]any
}

// CommitObservations lists every git.commit observation in log order.
func (s *Store) CommitObservations(ctx context.Context) ([]CommitObservation, error) {
	rows, err := s.db.query(ctx, `
		SELECT seq, observation_id, source_id, native_version_value, claim_payload
		FROM observations WHERE claim_type='git.commit' ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommitObservation
	for rows.Next() {
		var c CommitObservation
		var raw []byte
		if err := rows.Scan(&c.Seq, &c.ObsID, &c.SourceID, &c.OID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &c.Payload); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CommitRow is a commit as the projection holds it.
type CommitRow struct {
	SourceID, OID, ObsID, Author, Message string
	Seq                                   int64
	AuthoredAt                            string
	Parents                               []string
	ChangedPaths                          []string
}

// WriteCommits upserts commits, their touched paths, and the contains
// relations that reconcile them with scanned files, in one transaction.
func (s *Store) WriteCommits(ctx context.Context, commits []CommitRow, relations []RelationRow) error {
	tx, err := s.db.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range commits {
		if _, err := tx.exec(ctx, `
			INSERT INTO commits (source_id, oid, seq, observation_id, author, message, authored_at, parents)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (source_id, oid) DO UPDATE SET seq=EXCLUDED.seq, observation_id=EXCLUDED.observation_id`,
			c.SourceID, c.OID, c.Seq, c.ObsID, c.Author, c.Message, c.AuthoredAt, c.Parents); err != nil {
			return err
		}
		for _, p := range c.ChangedPaths {
			if _, err := tx.exec(ctx, `
				INSERT INTO commit_files (source_id, oid, locator) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
				c.SourceID, c.OID, p); err != nil {
				return err
			}
		}
	}
	for _, r := range relations {
		if err := execRelation(ctx, tx, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecentCommit is a commit observation as the git command summarises it.
type RecentCommit struct {
	OID, Author, Message string
	Files                int
}

// RecentCommits lists the newest commit observations of a source.
func (s *Store) RecentCommits(ctx context.Context, sourceID string, limit int) ([]RecentCommit, error) {
	rows, err := s.db.query(ctx, `
		SELECT native_version_value, claim_payload
		FROM observations WHERE source_id=$1 AND claim_type='git.commit'
		ORDER BY seq DESC LIMIT $2`, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecentCommit
	for rows.Next() {
		var c RecentCommit
		var raw []byte
		if err := rows.Scan(&c.OID, &raw); err != nil {
			return nil, err
		}
		var p struct {
			Author  string   `json:"author"`
			Message string   `json:"message"`
			Changed []string `json:"changed_paths"`
		}
		_ = json.Unmarshal(raw, &p)
		c.Author, c.Message, c.Files = p.Author, p.Message, len(p.Changed)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SourceRoots maps every source to its root path.
func (s *Store) SourceRoots(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.query(ctx, `SELECT source_id, root FROM sources`)
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
