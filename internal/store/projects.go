package store

import (
	"context"
	"encoding/json"
)

// ManifestRow is the latest project.manifest observation of a present file.
type ManifestRow struct {
	SourceID, Locator, ObsID string
	Payload                  []byte
}

// latestClaim is the newest observation of a claim type per locator, joined
// to the present inventory. The max(seq) subquery is the portable spelling
// of DISTINCT ON.
const latestClaim = `
	SELECT o.source_id, o.locator, o.observation_id, o.claim_payload, cf.native_version_value, coalesce(cf.size_bytes,0)
	FROM observations o
	JOIN current_files cf ON cf.source_id=o.source_id AND cf.locator=o.locator AND cf.present
	WHERE o.claim_type = $1
	  AND o.seq = (SELECT max(x.seq) FROM observations x
	               WHERE x.source_id=o.source_id AND x.locator=o.locator AND x.claim_type=$1)
	ORDER BY o.source_id, o.locator`

// LatestManifests returns the newest manifest observation for every
// manifest file that is currently present.
func (s *Store) LatestManifests(ctx context.Context) ([]ManifestRow, error) {
	rows, err := s.db.query(ctx, latestClaim, "project.manifest")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManifestRow
	for rows.Next() {
		var m ManifestRow
		var nv string
		var size int64
		if err := rows.Scan(&m.SourceID, &m.Locator, &m.ObsID, &m.Payload, &nv, &size); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkerRow is the latest folder.metadata observation of a folder that
// still has a present file beneath it.
type MarkerRow struct {
	SourceID, Locator, ObsID string
	Markers                  []string
}

// LatestMarkers returns marker folders. Folders get no tombstones, so a
// folder with nothing present under it is taken to be gone.
func (s *Store) LatestMarkers(ctx context.Context) ([]MarkerRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT o.source_id, o.locator, o.observation_id, o.claim_payload
		FROM observations o
		WHERE o.claim_type = 'folder.metadata' AND o.subject_kind = 'folder'
		  AND o.seq = (SELECT max(x.seq) FROM observations x
		               WHERE x.source_id=o.source_id AND x.locator=o.locator AND x.claim_type='folder.metadata')
		  AND (o.locator = '.' OR EXISTS (
			SELECT 1 FROM current_files cf
			WHERE cf.source_id=o.source_id AND cf.present AND cf.locator LIKE o.locator || '/%'))
		ORDER BY o.source_id, o.locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MarkerRow
	for rows.Next() {
		var m MarkerRow
		var raw []byte
		if err := rows.Scan(&m.SourceID, &m.Locator, &m.ObsID, &raw); err != nil {
			return nil, err
		}
		var p struct {
			Markers []string `json:"markers"`
		}
		_ = json.Unmarshal(raw, &p)
		m.Markers = p.Markers
		out = append(out, m)
	}
	return out, rows.Err()
}

// InvalidManifests returns present manifests whose latest observation says
// they did not parse, with the error.
func (s *Store) InvalidManifests(ctx context.Context) ([]PresentFile, []string, error) {
	rows, err := s.db.query(ctx, latestClaim, "project.manifest")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var files []PresentFile
	var errs []string
	for rows.Next() {
		var f PresentFile
		var raw []byte
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.ObsID, &raw, &f.NativeVersion, &f.Size); err != nil {
			return nil, nil, err
		}
		var p struct {
			Valid bool   `json:"valid"`
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Valid {
			continue
		}
		files = append(files, f)
		errs = append(errs, p.Error)
	}
	return files, errs, rows.Err()
}

// ProjectRow mirrors the projects table.
type ProjectRow struct {
	ProjectID, Name, Description, Basis string
	SourceID, Locator                   string
	Evidence                            []string
	Confidence                          float64
	Explanation                         string
}

type ProjectMemberRow struct {
	ProjectID, SourceID, Pattern, Basis, Evidence string
	Confidence                                    float64
}

type FileProjectRow struct {
	SourceID, Locator, ProjectID, Basis, Pattern string
	Confidence                                   float64
}

// ReplaceProjects rebuilds the three project tables in one transaction.
func (s *Store) ReplaceProjects(ctx context.Context, projects []ProjectRow, members []ProjectMemberRow, files []FileProjectRow) error {
	tx, err := s.db.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.exec(ctx, `DELETE FROM projects`); err != nil {
		return err
	}
	for _, p := range projects {
		if _, err := tx.exec(ctx, `INSERT INTO projects (project_id, name, description, basis, source_id, locator,
			evidence, confidence, explanation) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			p.ProjectID, p.Name, p.Description, p.Basis, p.SourceID, p.Locator, p.Evidence, p.Confidence, p.Explanation); err != nil {
			return err
		}
	}
	for _, m := range members {
		if _, err := tx.exec(ctx, `INSERT INTO project_members (project_id, source_id, pattern, basis, confidence, evidence)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			m.ProjectID, m.SourceID, m.Pattern, m.Basis, m.Confidence, m.Evidence); err != nil {
			return err
		}
	}
	for _, f := range files {
		if _, err := tx.exec(ctx, `INSERT INTO file_projects (source_id, locator, project_id, basis, confidence, pattern)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			f.SourceID, f.Locator, f.ProjectID, f.Basis, f.Confidence, f.Pattern); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FileProjectsOf lists the projects a file belongs to, strongest first,
// with the project's name.
func (s *Store) FileProjectsOf(ctx context.Context, sourceID, locator string) ([]FileProjectRow, []string, error) {
	rows, err := s.db.query(ctx, `
		SELECT fp.project_id, p.name, fp.basis, fp.pattern, fp.confidence
		FROM file_projects fp JOIN projects p ON p.project_id=fp.project_id
		WHERE fp.source_id=$1 AND fp.locator=$2
		ORDER BY fp.confidence DESC, fp.project_id`, sourceID, locator)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []FileProjectRow
	var names []string
	for rows.Next() {
		var f FileProjectRow
		var name string
		if err := rows.Scan(&f.ProjectID, &name, &f.Basis, &f.Pattern, &f.Confidence); err != nil {
			return nil, nil, err
		}
		f.SourceID, f.Locator = sourceID, locator
		out = append(out, f)
		names = append(names, name)
	}
	return out, names, rows.Err()
}

// ProjectSummary is a project with its aggregate counts and members.
type ProjectSummary struct {
	ProjectRow
	FileCount int
	SourceIDs []string
	Members   []ProjectMemberRow
}

// ProjectSummaries lists every project with counts and members, assembled
// from three plain queries rather than JSON aggregates.
func (s *Store) ProjectSummaries(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.db.query(ctx, `
		SELECT project_id, name, description, basis, source_id, locator, evidence, confidence, explanation
		FROM projects ORDER BY basis, name, project_id`)
	if err != nil {
		return nil, err
	}
	var out []ProjectSummary
	index := map[string]int{}
	for rows.Next() {
		var p ProjectSummary
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.Description, &p.Basis, &p.SourceID, &p.Locator,
			jsl(&p.Evidence), &p.Confidence, &p.Explanation); err != nil {
			rows.Close()
			return nil, err
		}
		p.SourceIDs = []string{}
		index[p.ProjectID] = len(out)
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mrows, err := s.db.query(ctx, `
		SELECT project_id, source_id, pattern, basis, confidence, evidence
		FROM project_members ORDER BY project_id, source_id, pattern`)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var m ProjectMemberRow
		if err := mrows.Scan(&m.ProjectID, &m.SourceID, &m.Pattern, &m.Basis, &m.Confidence, &m.Evidence); err != nil {
			mrows.Close()
			return nil, err
		}
		if i, ok := index[m.ProjectID]; ok {
			out[i].Members = append(out[i].Members, m)
		}
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}

	frows, err := s.db.query(ctx, `SELECT project_id, source_id FROM file_projects ORDER BY project_id, source_id`)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	seen := map[string]bool{}
	for frows.Next() {
		var pid, src string
		if err := frows.Scan(&pid, &src); err != nil {
			return nil, err
		}
		i, ok := index[pid]
		if !ok {
			continue
		}
		out[i].FileCount++
		if !seen[pid+"\x00"+src] {
			seen[pid+"\x00"+src] = true
			out[i].SourceIDs = append(out[i].SourceIDs, src)
		}
	}
	return out, frows.Err()
}

// AllFileProjects lists every file membership, by file then confidence.
func (s *Store) AllFileProjects(ctx context.Context) ([]FileProjectRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT source_id, locator, project_id, basis, pattern, confidence
		FROM file_projects ORDER BY source_id, locator, confidence DESC, project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileProjectRow
	for rows.Next() {
		var f FileProjectRow
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.ProjectID, &f.Basis, &f.Pattern, &f.Confidence); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
