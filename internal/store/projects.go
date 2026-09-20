package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// ManifestRow is the latest project.manifest observation of a present file.
type ManifestRow struct {
	SourceID, Locator, ObsID string
	Payload                  []byte
}

// LatestManifests returns the newest manifest observation for every
// manifest file that is currently present.
func (s *Store) LatestManifests(ctx context.Context) ([]ManifestRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.source_id, o.locator, o.observation_id, o.claim_payload
		FROM (
			SELECT DISTINCT ON (source_id, locator) source_id, locator, observation_id, claim_payload
			FROM observations WHERE claim_type='project.manifest'
			ORDER BY source_id, locator, seq DESC
		) o
		JOIN current_files cf ON cf.source_id=o.source_id AND cf.locator=o.locator AND cf.present
		ORDER BY o.source_id, o.locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManifestRow
	for rows.Next() {
		var m ManifestRow
		if err := rows.Scan(&m.SourceID, &m.Locator, &m.ObsID, &m.Payload); err != nil {
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
	rows, err := s.pool.Query(ctx, `
		SELECT o.source_id, o.locator, o.observation_id, o.claim_payload->'markers'
		FROM (
			SELECT DISTINCT ON (source_id, locator) source_id, locator, observation_id, claim_payload
			FROM observations WHERE claim_type='folder.metadata' AND subject_kind='folder'
			ORDER BY source_id, locator, seq DESC
		) o
		WHERE o.locator = '.' OR EXISTS (
			SELECT 1 FROM current_files cf
			WHERE cf.source_id=o.source_id AND cf.present AND cf.locator LIKE o.locator || '/%')
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
		if err := json.Unmarshal(raw, &m.Markers); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// InvalidManifests returns present manifests whose latest observation says
// they did not parse, with the error.
func (s *Store) InvalidManifests(ctx context.Context) ([]PresentFile, []string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.source_id, o.locator, cf.native_version_value, coalesce(cf.size_bytes,0),
		       o.observation_id, coalesce(o.claim_payload->>'error','')
		FROM (
			SELECT DISTINCT ON (source_id, locator) source_id, locator, observation_id, claim_payload
			FROM observations WHERE claim_type='project.manifest'
			ORDER BY source_id, locator, seq DESC
		) o
		JOIN current_files cf ON cf.source_id=o.source_id AND cf.locator=o.locator AND cf.present
		WHERE (o.claim_payload->>'valid')::boolean = false
		ORDER BY o.source_id, o.locator`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var files []PresentFile
	var errs []string
	for rows.Next() {
		var f PresentFile
		var e string
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.NativeVersion, &f.Size, &f.ObsID, &e); err != nil {
			return nil, nil, err
		}
		files = append(files, f)
		errs = append(errs, e)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM projects`); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range projects {
		batch.Queue(`INSERT INTO projects (project_id, name, description, basis, source_id, locator,
			evidence, confidence, explanation) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			p.ProjectID, p.Name, p.Description, p.Basis, p.SourceID, p.Locator, p.Evidence, p.Confidence, p.Explanation)
	}
	for _, m := range members {
		batch.Queue(`INSERT INTO project_members (project_id, source_id, pattern, basis, confidence, evidence)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			m.ProjectID, m.SourceID, m.Pattern, m.Basis, m.Confidence, m.Evidence)
	}
	for _, f := range files {
		batch.Queue(`INSERT INTO file_projects (source_id, locator, project_id, basis, confidence, pattern)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			f.SourceID, f.Locator, f.ProjectID, f.Basis, f.Confidence, f.Pattern)
	}
	if err := sendBatch(ctx, tx, batch); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func sendBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch) error {
	if batch.Len() == 0 {
		return nil
	}
	res := tx.SendBatch(ctx, batch)
	for i := 0; i < batch.Len(); i++ {
		if _, err := res.Exec(); err != nil {
			res.Close()
			return err
		}
	}
	return res.Close()
}

// FileProjectsOf lists the projects a file belongs to, strongest first,
// with the project's name.
func (s *Store) FileProjectsOf(ctx context.Context, sourceID, locator string) ([]FileProjectRow, []string, error) {
	rows, err := s.pool.Query(ctx, `
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

// ProjectSummaries lists every project with counts and members.
func (s *Store) ProjectSummaries(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.project_id, p.name, p.description, p.basis, p.source_id, p.locator, p.evidence, p.confidence, p.explanation,
		       (SELECT count(*) FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT coalesce(array_agg(DISTINCT fp.source_id ORDER BY fp.source_id), '{}')
		          FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT coalesce(json_agg(json_build_object(
		            'project_id', m.project_id, 'source_id', m.source_id, 'pattern', m.pattern, 'basis', m.basis,
		            'confidence', m.confidence, 'evidence', m.evidence) ORDER BY m.source_id, m.pattern), '[]')
		          FROM project_members m WHERE m.project_id=p.project_id)
		FROM projects p ORDER BY p.basis, p.name, p.project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectSummary
	for rows.Next() {
		var p ProjectSummary
		var members []byte
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.Description, &p.Basis, &p.SourceID, &p.Locator,
			&p.Evidence, &p.Confidence, &p.Explanation, &p.FileCount, &p.SourceIDs, &members); err != nil {
			return nil, err
		}
		var ms []struct {
			ProjectID  string  `json:"project_id"`
			SourceID   string  `json:"source_id"`
			Pattern    string  `json:"pattern"`
			Basis      string  `json:"basis"`
			Confidence float64 `json:"confidence"`
			Evidence   string  `json:"evidence"`
		}
		if err := json.Unmarshal(members, &ms); err != nil {
			return nil, err
		}
		for _, m := range ms {
			p.Members = append(p.Members, ProjectMemberRow{m.ProjectID, m.SourceID, m.Pattern, m.Basis, m.Evidence, m.Confidence})
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AllFileProjects lists every file membership, by file then confidence.
func (s *Store) AllFileProjects(ctx context.Context) ([]FileProjectRow, error) {
	rows, err := s.pool.Query(ctx, `
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
