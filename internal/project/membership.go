package project

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/DazzlingDukeOfLazers/gyst/internal/discover"
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

// Review states. A manifest declares; a marker suggests a candidate; a
// person confirms or ignores by assertion.
const (
	StateDeclared  = "declared"
	StateCandidate = "candidate"
	StateConfirmed = "confirmed"
	StateIgnored   = "ignored"
)

// Curation is a person's active assertion about a project record.
type Curation struct {
	ID, Kind, ProjectID, ActorID, Reason string
}

type PlannedProject struct {
	ID, Name, Description, Basis string
	State                        string
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
	VendoredMarkers   int
	// StaleCurations are assertions naming a project id that no longer
	// exists, a folder moved or a source gone. They are kept, not applied.
	StaleCurations int
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
func ResolveProjects(manifests []ManifestEvidence, markers []MarkerEvidence, curation []Curation) MembershipPlan {
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
				ID: m.ID, Name: m.Name, Description: m.Description, Basis: BasisManifest, State: StateDeclared,
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
				SourceID: m.SourceID, Pattern: manifest.Resolve(manifest.Dir(m.Locator), pat), Basis: BasisManifest,
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
		if discover.Vendored(dir) {
			// A marker inside a dependency or build directory names a
			// dependency, not a project. Filtered here as well as at the
			// scanner, so a log written before the scanner knew does not
			// keep producing them.
			plan.VendoredMarkers++
			continue
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
			ID: id, Name: name, Basis: BasisNativeMarker, State: StateCandidate,
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

	// A person's word, last. Explicit assertions sit above manifests and
	// markers in the precedence order, so they are applied after both.
	// The last active assertion about a record wins if there are several;
	// the CLI refuses to stack them, so that is a guard, not a feature.
	for _, c := range curation {
		p, ok := byID[c.ProjectID]
		if !ok {
			plan.StaleCurations++
			continue
		}
		switch c.Kind {
		case "project.confirm":
			p.State = StateConfirmed
			p.Confidence = 1.0
			p.Explanation += fmt.Sprintf("; confirmed as a project by %s: %s", c.ActorID, c.Reason)
			p.Evidence = append(p.Evidence, c.ID)
		case "project.ignore":
			// The record stays, so the decision is visible and retractable,
			// but it claims nothing: no members, so no file belongs to it.
			p.State = StateIgnored
			p.Members = nil
			p.Explanation += fmt.Sprintf("; set aside by %s: %s", c.ActorID, c.Reason)
			p.Evidence = append(p.Evidence, c.ID)
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
	// Review progress: candidates awaiting judgment, and judgments made.
	Remaining      int
	Confirmed      int
	Ignored        int
	StaleCurations int
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
	curation, err := loadCuration(ctx, s)
	if err != nil {
		return st, err
	}
	plan := ResolveProjects(manifests, markers, curation)
	st.Projects, st.InvalidManifests, st.SuppressedMarkers, st.StaleCurations =
		len(plan.Projects), plan.InvalidManifests, plan.SuppressedMarkers, plan.StaleCurations
	for _, p := range plan.Projects {
		switch p.State {
		case StateCandidate:
			st.Remaining++
		case StateConfirmed:
			st.Confirmed++
		case StateIgnored:
			st.Ignored++
		}
	}

	files, err := s.PresentFiles(ctx)
	if err != nil {
		return st, err
	}

	var projects []store.ProjectRow
	var members []store.ProjectMemberRow
	var fileRows []store.FileProjectRow
	for _, p := range plan.Projects {
		projects = append(projects, store.ProjectRow{
			ProjectID: p.ID, Name: p.Name, Description: p.Description, Basis: p.Basis, State: p.State,
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

// loadCuration returns active project assertions in the order made.
func loadCuration(ctx context.Context, s *store.Store) ([]Curation, error) {
	rows, err := s.ListAssertions(ctx, false)
	if err != nil {
		return nil, err
	}
	var out []Curation
	for _, r := range rows {
		if r.SubjectKind != "project" {
			continue
		}
		out = append(out, Curation{ID: r.AssertionID, Kind: r.Kind, ProjectID: r.Locator, ActorID: r.ActorID, Reason: r.Reason})
	}
	return out, nil
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
