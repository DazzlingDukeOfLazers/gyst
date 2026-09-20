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

	files, err := s.PresentFiles(ctx)
	if err != nil {
		return st, err
	}

	var projects []store.ProjectRow
	var members []store.ProjectMemberRow
	var fileRows []store.FileProjectRow
	for _, p := range plan.Projects {
		projects = append(projects, store.ProjectRow{
			ProjectID: p.ID, Name: p.Name, Description: p.Description, Basis: p.Basis,
			SourceID: p.SourceID, Locator: p.Locator, Evidence: p.Evidence,
			Confidence: p.Confidence, Explanation: p.Explanation,
		})
		for _, m := range p.Members {
			members = append(members, store.ProjectMemberRow{
				ProjectID: p.ID, SourceID: m.SourceID, Pattern: m.Pattern, Basis: m.Basis,
				Evidence: m.Evidence, Confidence: m.Confidence,
			})
			for _, f := range files {
				if f.SourceID != m.SourceID || !manifest.Match(m.Pattern, f.Locator) {
					continue
				}
				fileRows = append(fileRows, store.FileProjectRow{
					SourceID: f.SourceID, Locator: f.Locator, ProjectID: p.ID,
					Basis: m.Basis, Pattern: m.Pattern, Confidence: m.Confidence,
				})
				st.FileMemberships++
			}
		}
	}
	return st, s.ReplaceProjects(ctx, projects, members, fileRows)
}

// loadManifests returns the latest manifest observation for every manifest
// file that is currently present. A deleted manifest's project must vanish
// with it.
func loadManifests(ctx context.Context, s *store.Store) ([]ManifestEvidence, error) {
	rows, err := s.LatestManifests(ctx)
	if err != nil {
		return nil, err
	}
	var out []ManifestEvidence
	for _, r := range rows {
		m := ManifestEvidence{SourceID: r.SourceID, Locator: r.Locator, ObsID: r.ObsID}
		var p struct {
			Valid       bool     `json:"valid"`
			Error       string   `json:"error"`
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Members     []string `json:"members"`
		}
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			return nil, err
		}
		m.Valid, m.Error, m.ID, m.Name, m.Description, m.Members =
			p.Valid, p.Error, p.ID, p.Name, p.Description, p.Members
		out = append(out, m)
	}
	return out, nil
}

// loadMarkers returns the latest marker observation for every folder that
// still has a present file beneath it.
func loadMarkers(ctx context.Context, s *store.Store) ([]MarkerEvidence, error) {
	rows, err := s.LatestMarkers(ctx)
	if err != nil {
		return nil, err
	}
	var out []MarkerEvidence
	for _, r := range rows {
		if len(r.Markers) > 0 {
			out = append(out, MarkerEvidence{SourceID: r.SourceID, Locator: r.Locator, ObsID: r.ObsID, Markers: r.Markers})
		}
	}
	return out, nil
}

// FileProject is one project a file belongs to.
type FileProject struct {
	ProjectID, Name, Basis, Pattern string
	Confidence                      float64
}

// ProjectsOf lists the projects a file belongs to, strongest basis first.
func ProjectsOf(ctx context.Context, s *store.Store, sourceID, locator string) ([]FileProject, error) {
	rows, names, err := s.FileProjectsOf(ctx, sourceID, locator)
	if err != nil {
		return nil, err
	}
	out := make([]FileProject, 0, len(rows))
	for i, r := range rows {
		out = append(out, FileProject{ProjectID: r.ProjectID, Name: names[i], Basis: r.Basis,
			Pattern: r.Pattern, Confidence: r.Confidence})
	}
	return out, nil
}
