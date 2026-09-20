package project

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/DazzlingDukeOfLazers/gyst/internal/manifest"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
	"github.com/jackc/pgx/v5"
)

// Membership bases, in precedence order. See migrations/0007_projects.sql.
const (
	BasisExplicit     = "explicit"
	BasisManifest     = "manifest"
	BasisNativeMarker = "native-marker"
	BasisOrgRule      = "organization-rule"
	BasisSuggestion   = "suggestion"
)

// markerConfidence is how strongly a native marker suggests a project. A
// .git directory is a repository boundary, and repository boundaries are
// often project boundaries, but the design is explicit that they are not
// equated. Below the 0.8 line that separates an inference from a suggestion.
const markerConfidence = 0.6

// ManifestEvidence is the latest observation of one .gyst/project.yaml.
type ManifestEvidence struct {
	SourceID, Locator, ObsID string
	Valid                    bool
	Error                    string
	ID, Name, Description    string
	Members                  []string
}

// MarkerEvidence is the latest observation of one folder carrying markers.
type MarkerEvidence struct {
	SourceID, Locator, ObsID string
	Markers                  []string
}

type PlannedMember struct {
	SourceID, Pattern, Basis, Evidence string
	Confidence                         float64
}

type PlannedProject struct {
	ID, Name, Description, Basis string
	SourceID, Locator            string
	Evidence                     []string
	Confidence                   float64
	Explanation                  string
	Members                      []PlannedMember
}

// MembershipPlan is what the evidence resolves to, before anything is written.
type MembershipPlan struct {
	Projects          []PlannedProject
	InvalidManifests  int
	SuppressedMarkers int
}

// ResolveProjects turns manifest and marker evidence into projects.
//
// A valid manifest declares a project at confidence 1.0. Two manifests with
// the same id, in different sources or folders, describe one project: that is
// how a project spans repositories. A marker folder suggests a project at
// lower confidence, unless a manifest sits in that very folder, in which case
// the manifest has already named what the marker only hints at and the
// suggestion is dropped as redundant rather than recorded as a second
// project.
func ResolveProjects(manifests []ManifestEvidence, markers []MarkerEvidence) MembershipPlan {
	plan := MembershipPlan{}
	byID := map[string]*PlannedProject{}
	declaredAt := map[string]bool{} // source \x00 dir

	sort.Slice(manifests, func(i, j int) bool {
		if manifests[i].SourceID != manifests[j].SourceID {
			return manifests[i].SourceID < manifests[j].SourceID
		}
		return manifests[i].Locator < manifests[j].Locator
	})
	for _, m := range manifests {
		if !m.Valid {
			plan.InvalidManifests++
			continue
		}
		declaredAt[m.SourceID+"\x00"+manifest.Dir(m.Locator)] = true
		p, ok := byID[m.ID]
		if !ok {
			p = &PlannedProject{
				ID: m.ID, Name: m.Name, Description: m.Description, Basis: BasisManifest,
				SourceID: m.SourceID, Locator: m.Locator, Confidence: 1.0,
				Explanation: fmt.Sprintf("declared by %s", m.Locator),
			}
			byID[m.ID] = p
		} else {
			p.Explanation += fmt.Sprintf("; also declared by %s in %s", m.Locator, m.SourceID)
		}
		p.Evidence = append(p.Evidence, m.ObsID)
		for _, pat := range m.Members {
			p.Members = append(p.Members, PlannedMember{
				SourceID: m.SourceID, Pattern: pat, Basis: BasisManifest,
				Evidence: m.ObsID, Confidence: 1.0,
			})
		}
	}

	sort.Slice(markers, func(i, j int) bool {
		if markers[i].SourceID != markers[j].SourceID {
			return markers[i].SourceID < markers[j].SourceID
		}
		return markers[i].Locator < markers[j].Locator
	})
	for _, mk := range markers {
		dir := mk.Locator
		if dir == "." {
			dir = ""
		}
		if declaredAt[mk.SourceID+"\x00"+dir] {
			plan.SuppressedMarkers++
			continue
		}
		id := "mark_" + shortHash(mk.SourceID, dir)
		name := path.Base(dir)
		if dir == "" {
			name = mk.SourceID
		}
		pattern := "**"
		if dir != "" {
			pattern = dir + "/**"
		}
		byID[id] = &PlannedProject{
			ID: id, Name: name, Basis: BasisNativeMarker,
			SourceID: mk.SourceID, Locator: mk.Locator, Evidence: []string{mk.ObsID},
			Confidence: markerConfidence,
			Explanation: fmt.Sprintf("folder carries %s; a native marker suggests a project boundary but does not declare one",
				strings.Join(mk.Markers, ", ")),
			Members: []PlannedMember{{
				SourceID: mk.SourceID, Pattern: pattern, Basis: BasisNativeMarker,
				Evidence: mk.ObsID, Confidence: markerConfidence,
			}},
		}
	}

	for _, p := range byID {
		plan.Projects = append(plan.Projects, *p)
	}
	sort.Slice(plan.Projects, func(i, j int) bool { return plan.Projects[i].ID < plan.Projects[j].ID })
	return plan
}

// MembershipStats reports what a membership projection pass did.
type MembershipStats struct {
	Projects          int
	Manifests         int
	InvalidManifests  int
	MarkedFolders     int
	SuppressedMarkers int
	FileMemberships   int
}

// ProjectMembership rebuilds projects, project_members, and file_projects
// from the log. Full rebuild every time: the inputs are few and the output
// must never carry a stale row from a manifest that has since been deleted.
func ProjectMembership(ctx context.Context, s *store.Store) (MembershipStats, error) {
	var st MembershipStats

	manifests, err := loadManifests(ctx, s)
	if err != nil {
		return st, err
	}
	markers, err := loadMarkers(ctx, s)
	if err != nil {
		return st, err
	}
	st.Manifests, st.MarkedFolders = len(manifests), len(markers)
	plan := ResolveProjects(manifests, markers)
	st.Projects, st.InvalidManifests, st.SuppressedMarkers =
		len(plan.Projects), plan.InvalidManifests, plan.SuppressedMarkers

	files, err := currentFiles(ctx, s)
	if err != nil {
		return st, err
	}

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return st, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM projects`); err != nil {
		return st, err
	}
	batch := &pgx.Batch{}
	n := 0
	for _, p := range plan.Projects {
		batch.Queue(`INSERT INTO projects (project_id, name, description, basis, source_id, locator,
			evidence, confidence, explanation) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			p.ID, p.Name, p.Description, p.Basis, p.SourceID, p.Locator, p.Evidence, p.Confidence, p.Explanation)
		n++
		for _, m := range p.Members {
			batch.Queue(`INSERT INTO project_members (project_id, source_id, pattern, basis, confidence, evidence)
				VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
				p.ID, m.SourceID, m.Pattern, m.Basis, m.Confidence, m.Evidence)
			n++
			for _, f := range files {
				if f.source != m.SourceID || !manifest.Match(m.Pattern, f.locator) {
					continue
				}
				batch.Queue(`INSERT INTO file_projects (source_id, locator, project_id, basis, confidence, pattern)
					VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
					f.source, f.locator, p.ID, m.Basis, m.Confidence, m.Pattern)
				n++
				st.FileMemberships++
			}
		}
	}
	res := tx.SendBatch(ctx, batch)
	for i := 0; i < n; i++ {
		if _, err := res.Exec(); err != nil {
			res.Close()
			return st, err
		}
	}
	if err := res.Close(); err != nil {
		return st, err
	}
	return st, tx.Commit(ctx)
}

// loadManifests returns the latest manifest observation for every manifest
// file that is currently present. A deleted manifest's project must vanish
// with it.
func loadManifests(ctx context.Context, s *store.Store) ([]ManifestEvidence, error) {
	rows, err := s.Pool().Query(ctx, `
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
	var out []ManifestEvidence
	for rows.Next() {
		var m ManifestEvidence
		var payload []byte
		if err := rows.Scan(&m.SourceID, &m.Locator, &m.ObsID, &payload); err != nil {
			return nil, err
		}
		var p struct {
			Valid       bool     `json:"valid"`
			Error       string   `json:"error"`
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Members     []string `json:"members"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, err
		}
		m.Valid, m.Error, m.ID, m.Name, m.Description, m.Members =
			p.Valid, p.Error, p.ID, p.Name, p.Description, p.Members
		out = append(out, m)
	}
	return out, rows.Err()
}

// loadMarkers returns the latest marker observation for every folder that
// still has a present file beneath it. Folders get no tombstones, so a
// folder with nothing present under it is taken to be gone.
func loadMarkers(ctx context.Context, s *store.Store) ([]MarkerEvidence, error) {
	rows, err := s.Pool().Query(ctx, `
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
	var out []MarkerEvidence
	for rows.Next() {
		var m MarkerEvidence
		var markers []byte
		if err := rows.Scan(&m.SourceID, &m.Locator, &m.ObsID, &markers); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(markers, &m.Markers); err != nil {
			return nil, err
		}
		if len(m.Markers) > 0 {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

type fileRow struct{ source, locator string }

func currentFiles(ctx context.Context, s *store.Store) ([]fileRow, error) {
	rows, err := s.Pool().Query(ctx,
		`SELECT source_id, locator FROM current_files WHERE present ORDER BY source_id, locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.source, &f.locator); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FileProject is one project a file belongs to.
type FileProject struct {
	ProjectID, Name, Basis, Pattern string
	Confidence                      float64
}

// ProjectsOf lists the projects a file belongs to, strongest basis first.
func ProjectsOf(ctx context.Context, s *store.Store, sourceID, locator string) ([]FileProject, error) {
	rows, err := s.Pool().Query(ctx, `
		SELECT fp.project_id, p.name, fp.basis, fp.pattern, fp.confidence
		FROM file_projects fp JOIN projects p ON p.project_id=fp.project_id
		WHERE fp.source_id=$1 AND fp.locator=$2
		ORDER BY fp.confidence DESC, fp.project_id`, sourceID, locator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileProject
	for rows.Next() {
		var f FileProject
		if err := rows.Scan(&f.ProjectID, &f.Name, &f.Basis, &f.Pattern, &f.Confidence); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
