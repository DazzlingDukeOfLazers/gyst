package project

import "testing"

func TestManifestDeclaresProject(t *testing.T) {
	plan := ResolveProjects([]ManifestEvidence{{
		SourceID: "src", Locator: "engineering/widget/.gyst/project.yaml", ObsID: "obs_m",
		Valid: true, ID: "widget", Name: "Widget", Members: []string{"**"},
	}}, nil, nil)
	if len(plan.Projects) != 1 {
		t.Fatalf("projects = %d", len(plan.Projects))
	}
	p := plan.Projects[0]
	if p.ID != "widget" || p.Basis != BasisManifest || p.Confidence != 1.0 {
		t.Errorf("%+v", p)
	}
	if len(p.Members) != 1 || p.Members[0].Pattern != "engineering/widget/**" {
		t.Errorf("members %+v", p.Members)
	}
}

// One id declared in two sources is one project spanning both. This is the
// whole point of manifests over folders.
func TestSameIdInTwoSourcesIsOneProject(t *testing.T) {
	plan := ResolveProjects([]ManifestEvidence{
		{SourceID: "repo-a", Locator: ".gyst/project.yaml", ObsID: "o1", Valid: true, ID: "widget", Name: "Widget", Members: []string{"**"}},
		{SourceID: "share", Locator: "boards/widget/.gyst/project.yaml", ObsID: "o2", Valid: true, ID: "widget", Name: "Widget", Members: []string{"**"}},
	}, nil, nil)
	if len(plan.Projects) != 1 {
		t.Fatalf("projects = %d, want one spanning both sources", len(plan.Projects))
	}
	p := plan.Projects[0]
	if len(p.Members) != 2 || p.Members[0].SourceID == p.Members[1].SourceID {
		t.Errorf("members %+v", p.Members)
	}
	if len(p.Evidence) != 2 {
		t.Errorf("evidence %v; both manifests must be cited", p.Evidence)
	}
}

func TestMarkerSuggestsProjectAtLowerConfidence(t *testing.T) {
	plan := ResolveProjects(nil, []MarkerEvidence{{SourceID: "src", Locator: "firmware", ObsID: "o", Markers: []string{"git"}}}, nil)
	if len(plan.Projects) != 1 {
		t.Fatal("no project from marker")
	}
	p := plan.Projects[0]
	if p.Basis != BasisNativeMarker || p.Confidence >= 0.8 {
		t.Errorf("marker project %+v; a marker suggests, it does not declare", p)
	}
	if p.Name != "firmware" || p.Members[0].Pattern != "firmware/**" {
		t.Errorf("%+v", p)
	}
}

func TestRootMarkerCoversWholeSource(t *testing.T) {
	plan := ResolveProjects(nil, []MarkerEvidence{{SourceID: "gyst", Locator: ".", ObsID: "o", Markers: []string{"git", "go"}}}, nil)
	p := plan.Projects[0]
	if p.Members[0].Pattern != "**" || p.Name != "gyst" {
		t.Errorf("%+v", p)
	}
}

// A manifest in the marker's own folder has named what the marker hints at.
// Emitting both would show two projects for one thing.
func TestMarkerSuppressedWhereManifestDeclares(t *testing.T) {
	plan := ResolveProjects(
		[]ManifestEvidence{{SourceID: "src", Locator: "engineering/widget/.gyst/project.yaml", ObsID: "m", Valid: true, ID: "widget", Name: "Widget", Members: []string{"**"}}},
		[]MarkerEvidence{
			{SourceID: "src", Locator: "engineering/widget", ObsID: "k1", Markers: []string{"gyst"}},
			{SourceID: "src", Locator: "firmware", ObsID: "k2", Markers: []string{"git"}},
		}, nil)
	if len(plan.Projects) != 2 || plan.SuppressedMarkers != 1 {
		t.Fatalf("projects %d suppressed %d", len(plan.Projects), plan.SuppressedMarkers)
	}
}

func TestInvalidManifestIsCountedNotUsed(t *testing.T) {
	plan := ResolveProjects([]ManifestEvidence{{SourceID: "src", Locator: "x/.gyst/project.yaml", Valid: false, Error: "yaml: bad"}}, nil, nil)
	if len(plan.Projects) != 0 || plan.InvalidManifests != 1 {
		t.Errorf("%+v", plan)
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	a := ResolveProjects(nil, []MarkerEvidence{
		{SourceID: "s", Locator: "b", ObsID: "1", Markers: []string{"git"}},
		{SourceID: "s", Locator: "a", ObsID: "2", Markers: []string{"git"}},
	}, nil)
	b := ResolveProjects(nil, []MarkerEvidence{
		{SourceID: "s", Locator: "a", ObsID: "2", Markers: []string{"git"}},
		{SourceID: "s", Locator: "b", ObsID: "1", Markers: []string{"git"}},
	}, nil)
	if a.Projects[0].ID != b.Projects[0].ID || a.Projects[1].ID != b.Projects[1].ID {
		t.Error("order of evidence changed the plan")
	}
}

// A marker inside node_modules names a dependency, not a project, even
// when the log already holds its observation.
func TestVendoredMarkersAreNotProjects(t *testing.T) {
	plan := ResolveProjects(nil, []MarkerEvidence{
		{SourceID: "s", Locator: "app/node_modules/left-pad", ObsID: "o1", Markers: []string{"node"}},
		{SourceID: "s", Locator: "app", ObsID: "o2", Markers: []string{"node"}},
	}, nil)
	if len(plan.Projects) != 1 || plan.Projects[0].Name != "app" || plan.VendoredMarkers != 1 {
		t.Fatalf("%+v", plan)
	}
}

// A person's word sits above markers: confirming keeps the boundary and
// raises the record to confirmed; ignoring keeps the record but withdraws
// every membership; an assertion about a vanished record is counted stale
// and applied to nothing.
func TestCurationConfirmIgnoreAndStale(t *testing.T) {
	markers := []MarkerEvidence{
		{SourceID: "s", Locator: "fw", ObsID: "o1", Markers: []string{"git"}},
		{SourceID: "s", Locator: "web", ObsID: "o2", Markers: []string{"node"}},
	}
	base := ResolveProjects(nil, markers, nil)
	fw, web := base.Projects[0].ID, base.Projects[1].ID
	if base.Projects[0].Name != "fw" {
		fw, web = web, fw
	}
	plan := ResolveProjects(nil, markers, []Curation{
		{ID: "ast_1", Kind: "project.confirm", ProjectID: fw, ActorID: "d", Reason: "it ships"},
		{ID: "ast_2", Kind: "project.ignore", ProjectID: web, ActorID: "d", Reason: "scratch"},
		{ID: "ast_3", Kind: "project.confirm", ProjectID: "mark_gone", ActorID: "d", Reason: "?"},
	})
	if plan.StaleCurations != 1 {
		t.Errorf("stale %d", plan.StaleCurations)
	}
	for _, p := range plan.Projects {
		switch p.ID {
		case fw:
			if p.State != StateConfirmed || p.Confidence != 1.0 || len(p.Members) != 1 || p.Basis != BasisNativeMarker {
				t.Errorf("confirmed: %+v", p)
			}
		case web:
			if p.State != StateIgnored || len(p.Members) != 0 {
				t.Errorf("ignored: %+v", p)
			}
		}
	}
	for _, p := range base.Projects {
		if p.State != StateCandidate {
			t.Errorf("baseline state %s", p.State)
		}
	}
}
