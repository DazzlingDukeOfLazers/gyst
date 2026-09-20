// Package report assembles everything a reader needs into one document.
//
// This is the contract between the engine and the static report the design
// describes: sources with their freshness, projects with their members,
// files with their memberships and groupings, relations, and findings, each
// citing the observations behind it. It is a snapshot for a reader, not a
// new source of truth; every number in it can be rebuilt from the log.
//
// The document carries its own visibility scope. A static file has no
// viewer to filter for, so whoever holds it holds everything in it. The
// scope says what that is.
package report

import (
	"context"
	"sort"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/discover"
	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const Schema = "gyst.report/0.1.0"

type Document struct {
	Report     Meta               `json:"report"`
	Sources    []Source           `json:"sources"`
	Projects   []Project          `json:"projects"`
	Files      []File             `json:"files"`
	Artifacts  []Artifact         `json:"artifacts"`
	Relations  []Relation         `json:"relations"`
	Findings   []findings.Finding `json:"findings"`
	Assertions []AssertionRecord  `json:"assertions"`
}

// AssertionRecord is a person's statement, active or retracted. Retracted
// ones stay in the document: the history of what was asserted is part of
// the evidence.
type AssertionRecord struct {
	AssertionID   string        `json:"assertion_id"`
	Kind          string        `json:"kind"`
	Subject       authority.Key `json:"subject"`
	Actor         observe.Actor `json:"actor"`
	Reason        string        `json:"reason"`
	Evidence      []string      `json:"evidence"`
	AssertedAt    time.Time     `json:"asserted_at"`
	RetractedAt   *time.Time    `json:"retracted_at"`
	RetractedBy   *string       `json:"retracted_by"`
	RetractReason *string       `json:"retract_reason"`
}

type Meta struct {
	Schema          string          `json:"schema"`
	Generator       Generator       `json:"generator"`
	GeneratedAt     time.Time       `json:"generated_at"`
	VisibilityScope string          `json:"visibility_scope"`
	IdentityPolicy  *IdentityPolicy `json:"identity_policy"`
	Counts          Counts          `json:"counts"`
}

type Generator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type IdentityPolicy struct {
	Version string `json:"version"`
	Profile string `json:"profile"`
}

type Counts struct {
	Sources      int `json:"sources"`
	Projects     int `json:"projects"`
	FilesPresent int `json:"files_present"`
	FilesAbsent  int `json:"files_absent"`
	Placeholders int `json:"placeholders"`
	Artifacts    int `json:"artifacts"`
	Relations    int `json:"relations"`
	FindingsOpen int `json:"findings_open"`
	Observations int `json:"observations"`
	// Authority states across present files. "none" is a legitimate
	// outcome and is counted, not hidden.
	AuthorityDeclared int `json:"authority_declared"`
	AuthorityLikely   int `json:"authority_likely"`
	AuthorityMultiple int `json:"authority_multiple"`
	AuthorityNone     int `json:"authority_none"`
}

// Freshness states, as the design names them. Age and coverage are two
// values; this is the state a reader sees first, derived from both and
// from the source's own cadence.
const (
	FreshCurrent     = "current"
	FreshDueSoon     = "due-soon"
	FreshStale       = "stale"
	FreshInterrupted = "interrupted"
	FreshUnavailable = "unavailable"
	FreshNever       = "never-scanned"
	FreshRunning     = "running"
)

type Freshness struct {
	State             string `json:"state"`
	AgeSeconds        *int64 `json:"age_seconds"`
	ExpectedEverySecs int64  `json:"expected_every_seconds"`
	Coverage          string `json:"coverage"` // pass status, or "" when never scanned
	CoverageDetail    string `json:"coverage_detail"`
}

type Pass struct {
	PassID     string     `json:"pass_id"`
	Status     string     `json:"status"`
	Detail     string     `json:"detail"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Resumed    bool       `json:"resumed"`
	Scanned    int        `json:"scanned"`
	Unchanged  int        `json:"unchanged"`
	Skipped    int        `json:"skipped"`
	Appended   int        `json:"appended"`
}

type Source struct {
	SourceID     string            `json:"source_id"`
	Kind         string            `json:"kind"`
	Root         string            `json:"root"`
	Location     location.Location `json:"location"`
	Cadence      int64             `json:"cadence_seconds"`
	LatestPass   *Pass             `json:"latest_pass"`
	Freshness    Freshness         `json:"freshness"`
	FilesPresent int               `json:"files_present"`
	FindingsOpen int               `json:"findings_open"`
}

type Member struct {
	SourceID   string  `json:"source_id"`
	Pattern    string  `json:"pattern"`
	Basis      string  `json:"basis"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

type Project struct {
	ProjectID   string   `json:"project_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Basis       string   `json:"basis"`
	Confidence  float64  `json:"confidence"`
	Explanation string   `json:"explanation"`
	Evidence    []string `json:"evidence"`
	Members     []Member `json:"members"`
	FileCount   int      `json:"file_count"`
	SourceIDs   []string `json:"source_ids"`
}

type FileProject struct {
	ProjectID  string  `json:"project_id"`
	Basis      string  `json:"basis"`
	Confidence float64 `json:"confidence"`
	Pattern    string  `json:"pattern"`
}

type Grouping struct {
	ArtifactID   string  `json:"artifact_id"`
	GroupingKey  string  `json:"grouping_key"`
	VersionLabel *string `json:"version_label"`
	IsCurrent    bool    `json:"is_current"`
	Rule         string  `json:"rule"`
	Confidence   float64 `json:"confidence"`
	Explanation  string  `json:"explanation"`
}

type File struct {
	SourceID      string          `json:"source_id"`
	Locator       string          `json:"locator"`
	Present       bool            `json:"present"`
	SizeBytes     *int64          `json:"size_bytes"`
	ContentDigest *observe.Digest `json:"content_digest"`
	NativeVersion string          `json:"native_version"`
	ObservedAt    time.Time       `json:"observed_at"`
	ObservationID string          `json:"observation_id"`
	ContentLevel  string          `json:"content_level"`
	Placeholder   bool            `json:"placeholder"`
	// Vendored marks a file inside a dependency cache or build output:
	// observed, but an expected copy of something owned elsewhere. Such
	// files are not project markers, duplicate findings, or authority
	// candidates.
	Vendored bool          `json:"vendored"`
	Projects []FileProject `json:"projects"`
	Grouping *Grouping     `json:"grouping"`
	// Authority is separate from grouping and from membership. A manifest
	// declares membership; a profile marks a current version; neither is
	// authority. Only an assertion declares it, and absent one this says
	// likely, multiple, or none.
	Authority *authority.Authority `json:"authority"`
}

type ArtifactMember struct {
	Locator      string  `json:"locator"`
	VersionLabel *string `json:"version_label"`
	IsCurrent    bool    `json:"is_current"`
	Rule         string  `json:"rule"`
	Confidence   float64 `json:"confidence"`
	Explanation  string  `json:"explanation"`
}

type Artifact struct {
	ArtifactID  string           `json:"artifact_id"`
	SourceID    string           `json:"source_id"`
	GroupingKey string           `json:"grouping_key"`
	MemberCount int              `json:"member_count"`
	Confidence  float64          `json:"confidence"`
	Members     []ArtifactMember `json:"members"`
}

type Endpoint struct {
	SourceID string `json:"source_id"`
	Locator  string `json:"locator"`
}

type Relation struct {
	RelationID     string        `json:"relation_id"`
	Type           string        `json:"type"`
	From           Endpoint      `json:"from"`
	To             Endpoint      `json:"to"`
	Precedence     string        `json:"precedence"`
	Actor          observe.Actor `json:"actor"`
	Evidence       []string      `json:"evidence"`
	Confidence     float64       `json:"confidence"`
	Explanation    string        `json:"explanation"`
	AssertedAt     time.Time     `json:"asserted_at"`
	IdentityPolicy *string       `json:"identity_policy_version"`
}

// Classify derives the freshness state from the latest pass, its age, and
// the cadence. Due soon is the final fifth of the interval.
func Classify(passStatus string, age time.Duration, cadence time.Duration) string {
	switch passStatus {
	case "":
		return FreshNever
	case store.PassUnavailable:
		return FreshUnavailable
	case store.PassInterrupted:
		return FreshInterrupted
	case store.PassRunning:
		return FreshRunning
	}
	switch {
	case age > cadence:
		return FreshStale
	case age > cadence*4/5:
		return FreshDueSoon
	default:
		return FreshCurrent
	}
}

// Build assembles the document. Findings are re-detected first so that
// freshness reflects the clock at generation, not at the last scan.
func Build(ctx context.Context, s *store.Store, now time.Time, version string) (*Document, error) {
	if _, err := findings.Project(ctx, s, now); err != nil {
		return nil, err
	}
	doc := &Document{Report: Meta{
		Schema:      Schema,
		Generator:   Generator{Name: "gyst", Version: version},
		GeneratedAt: now,
		VisibilityScope: "Everything the exporting operator could see. No viewer filtering was applied; " +
			"whoever holds this file holds all of it.",
	}}

	polVersion, profile, err := s.ActivePolicy(ctx)
	if err != nil {
		return nil, err
	}
	if polVersion != "" {
		doc.Report.IdentityPolicy = &IdentityPolicy{Version: polVersion, Profile: profile}
	}

	if doc.Sources, err = sources(ctx, s, now); err != nil {
		return nil, err
	}
	if doc.Projects, err = projects(ctx, s); err != nil {
		return nil, err
	}
	if doc.Files, err = files(ctx, s, polVersion); err != nil {
		return nil, err
	}
	if doc.Artifacts, err = artifacts(ctx, s, polVersion); err != nil {
		return nil, err
	}
	if doc.Relations, err = relations(ctx, s); err != nil {
		return nil, err
	}
	if err := attachAuthority(ctx, s, doc); err != nil {
		return nil, err
	}
	asts, err := authority.List(ctx, s, true)
	if err != nil {
		return nil, err
	}
	doc.Assertions = make([]AssertionRecord, 0, len(asts))
	for _, a := range asts {
		doc.Assertions = append(doc.Assertions, AssertionRecord{
			AssertionID: a.ID, Kind: a.Kind, Subject: a.Subject,
			Actor: observe.Actor{Kind: "user", ID: a.ActorID}, Reason: a.Reason, Evidence: a.Evidence,
			AssertedAt: a.AssertedAt, RetractedAt: a.RetractedAt, RetractedBy: a.RetractedBy, RetractReason: a.RetractReason,
		})
	}
	rows, err := findings.List(ctx, s, true)
	if err != nil {
		return nil, err
	}
	doc.Findings = make([]findings.Finding, 0, len(rows))
	for _, r := range rows {
		doc.Findings = append(doc.Findings, r.Finding)
		if r.Status == findings.StatusOpen || r.Status == findings.StatusAcknowledged {
			doc.Report.Counts.FindingsOpen++
		}
	}

	sortDocument(doc)

	c := &doc.Report.Counts
	c.Sources, c.Projects, c.Artifacts, c.Relations = len(doc.Sources), len(doc.Projects), len(doc.Artifacts), len(doc.Relations)
	for _, f := range doc.Files {
		if f.Present {
			c.FilesPresent++
		} else {
			c.FilesAbsent++
		}
		if f.Placeholder {
			c.Placeholders++
		}
	}
	n, err := s.Count(ctx)
	if err != nil {
		return nil, err
	}
	c.Observations = int(n)
	return doc, nil
}

// sortDocument puts every list in byte order in Go. Databases sort text by
// collation, and collations differ between engines and between machines;
// the report is a contract and must come out the same everywhere.
func sortDocument(doc *Document) {
	sort.Slice(doc.Sources, func(i, j int) bool { return doc.Sources[i].SourceID < doc.Sources[j].SourceID })
	sort.Slice(doc.Projects, func(i, j int) bool {
		a, b := doc.Projects[i], doc.Projects[j]
		if a.Basis != b.Basis {
			return a.Basis < b.Basis
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ProjectID < b.ProjectID
	})
	for i := range doc.Projects {
		m := doc.Projects[i].Members
		sort.Slice(m, func(a, b int) bool {
			if m[a].SourceID != m[b].SourceID {
				return m[a].SourceID < m[b].SourceID
			}
			return m[a].Pattern < m[b].Pattern
		})
		sort.Strings(doc.Projects[i].SourceIDs)
	}
	sort.Slice(doc.Files, func(i, j int) bool {
		a, b := doc.Files[i], doc.Files[j]
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		return a.Locator < b.Locator
	})
	sort.Slice(doc.Artifacts, func(i, j int) bool {
		a, b := doc.Artifacts[i], doc.Artifacts[j]
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		return a.GroupingKey < b.GroupingKey
	})
	for i := range doc.Artifacts {
		m := doc.Artifacts[i].Members
		sort.Slice(m, func(a, b int) bool { return m[a].Locator < m[b].Locator })
	}
	sort.Slice(doc.Relations, func(i, j int) bool {
		a, b := doc.Relations[i], doc.Relations[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.From.SourceID != b.From.SourceID {
			return a.From.SourceID < b.From.SourceID
		}
		if a.From.Locator != b.From.Locator {
			return a.From.Locator < b.From.Locator
		}
		return a.To.Locator < b.To.Locator
	})
	rank := map[string]int{"high": 0, "medium": 1, "low": 2, "info": 3}
	sort.Slice(doc.Findings, func(i, j int) bool {
		a, b := doc.Findings[i], doc.Findings[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		if a.Status != b.Status {
			return a.Status < b.Status
		}
		return a.FindingID < b.FindingID
	})
	sort.Slice(doc.Assertions, func(i, j int) bool {
		return doc.Assertions[i].AssertedAt.Before(doc.Assertions[j].AssertedAt)
	})
}

// attachAuthority joins the resolved authority state onto every present
// file and counts the states.
func attachAuthority(ctx context.Context, s *store.Store, doc *Document) error {
	rows, err := s.AllFileAuthority(ctx)
	if err != nil {
		return err
	}
	byKey := map[authority.Key]authority.Authority{}
	for _, r := range rows {
		byKey[authority.Key{SourceID: r.SourceID, Locator: r.Locator}] = authority.FromRow(r)
	}
	c := &doc.Report.Counts
	for i := range doc.Files {
		f := &doc.Files[i]
		if a, ok := byKey[authority.Key{SourceID: f.SourceID, Locator: f.Locator}]; ok {
			a := a
			f.Authority = &a
			switch a.State {
			case authority.StateDeclared:
				c.AuthorityDeclared++
			case authority.StateLikely:
				c.AuthorityLikely++
			case authority.StateMultiple:
				c.AuthorityMultiple++
			default:
				c.AuthorityNone++
			}
		}
	}
	return nil
}

func sources(ctx context.Context, s *store.Store, now time.Time) ([]Source, error) {
	passes, err := s.LatestPasses(ctx)
	if err != nil {
		return nil, err
	}
	srcs, err := s.Sources(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.SourceRow{}
	for _, r := range srcs {
		byID[r.SourceID] = r
	}
	out := make([]Source, 0, len(passes))
	for _, p := range passes {
		row := byID[p.SourceID]
		src := Source{SourceID: p.SourceID, Kind: p.Kind, Root: row.Root, Location: row.Location}
		cadence, err := s.SourceCadence(ctx, p.SourceID)
		if err != nil {
			return nil, err
		}
		cd := time.Duration(cadence) * time.Second
		if cd <= 0 {
			cd = findings.DefaultCadence(string(row.Location.Kind))
		}
		src.Cadence = int64(cd.Seconds())
		src.Freshness = Freshness{ExpectedEverySecs: src.Cadence, Coverage: p.Status, CoverageDetail: p.Detail}
		var age time.Duration
		if p.Status != "" {
			src.LatestPass = &Pass{
				PassID: p.PassID, Status: p.Status, Detail: p.Detail, StartedAt: p.StartedAt,
				FinishedAt: p.FinishedAt, Resumed: p.Resumed,
				Scanned: p.Scanned, Unchanged: p.Unchanged, Skipped: p.Skipped, Appended: p.Appended,
			}
			age = now.Sub(p.StartedAt)
			secs := int64(age.Seconds())
			src.Freshness.AgeSeconds = &secs
		}
		src.Freshness.State = Classify(p.Status, age, cd)
		if src.FilesPresent, err = s.CountPresentFiles(ctx, p.SourceID); err != nil {
			return nil, err
		}
		if src.FindingsOpen, err = s.CountOpenFindings(ctx, p.SourceID); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

func projects(ctx context.Context, s *store.Store) ([]Project, error) {
	rows, err := s.ProjectSummaries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		p := Project{ProjectID: r.ProjectID, Name: r.Name, Description: r.Description, Basis: r.Basis,
			Confidence: r.Confidence, Explanation: r.Explanation, Evidence: r.Evidence,
			FileCount: r.FileCount, SourceIDs: r.SourceIDs, Members: []Member{}}
		for _, m := range r.Members {
			p.Members = append(p.Members, Member{SourceID: m.SourceID, Pattern: m.Pattern, Basis: m.Basis,
				Confidence: m.Confidence, Evidence: m.Evidence})
		}
		out = append(out, p)
	}
	return out, nil
}

func files(ctx context.Context, s *store.Store, policy string) ([]File, error) {
	inv, err := s.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	fps, err := s.AllFileProjects(ctx)
	if err != nil {
		return nil, err
	}
	projectsOf := map[string][]FileProject{}
	for _, fp := range fps {
		k := fp.SourceID + "\x00" + fp.Locator
		projectsOf[k] = append(projectsOf[k], FileProject{ProjectID: fp.ProjectID, Basis: fp.Basis,
			Confidence: fp.Confidence, Pattern: fp.Pattern})
	}
	groupingOf := map[string]*Grouping{}
	if policy != "" {
		arts, err := s.Artifacts(ctx, policy)
		if err != nil {
			return nil, err
		}
		keyOf := map[string]string{}
		for _, a := range arts {
			keyOf[a.ArtifactID] = a.GroupingKey
		}
		members, err := s.ArtifactMembers(ctx, policy)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			groupingOf[m.SourceID+"\x00"+m.Locator] = &Grouping{ArtifactID: m.ArtifactID, GroupingKey: keyOf[m.ArtifactID],
				VersionLabel: m.VersionLabel, IsCurrent: m.IsCurrent, Rule: m.Rule, Confidence: m.Confidence, Explanation: m.Explanation}
		}
	}
	out := make([]File, 0, len(inv))
	for _, r := range inv {
		f := File{SourceID: r.SourceID, Locator: r.Locator, Present: r.Present, SizeBytes: r.Size,
			NativeVersion: r.NativeVersion, ObservedAt: r.ObservedAt, ObservationID: r.ObsID,
			ContentLevel: r.ContentLevel, Placeholder: r.Placeholder, Vendored: discover.Vendored(r.Locator), Projects: []FileProject{}}
		if r.Digest != nil {
			f.ContentDigest = &observe.Digest{Algo: "sha256", Hex: *r.Digest}
		}
		k := r.SourceID + "\x00" + r.Locator
		if ps := projectsOf[k]; ps != nil {
			f.Projects = ps
		}
		f.Grouping = groupingOf[k]
		out = append(out, f)
	}
	return out, nil
}

func artifacts(ctx context.Context, s *store.Store, policy string) ([]Artifact, error) {
	if policy == "" {
		return []Artifact{}, nil
	}
	arts, err := s.Artifacts(ctx, policy)
	if err != nil {
		return nil, err
	}
	members, err := s.ArtifactMembers(ctx, policy)
	if err != nil {
		return nil, err
	}
	byArtifact := map[string][]ArtifactMember{}
	for _, m := range members {
		byArtifact[m.ArtifactID] = append(byArtifact[m.ArtifactID], ArtifactMember{Locator: m.Locator,
			VersionLabel: m.VersionLabel, IsCurrent: m.IsCurrent, Rule: m.Rule, Confidence: m.Confidence, Explanation: m.Explanation})
	}
	// Only groupings with more than one member. A file that is its own
	// artifact says nothing files[].grouping does not already say, and
	// eighty thousand of them made a report three times the size it needs.
	out := make([]Artifact, 0, len(arts))
	for _, a := range arts {
		if a.MemberCount < 2 {
			continue
		}
		ms := byArtifact[a.ArtifactID]
		if ms == nil {
			ms = []ArtifactMember{}
		}
		out = append(out, Artifact{ArtifactID: a.ArtifactID, SourceID: a.SourceID, GroupingKey: a.GroupingKey,
			MemberCount: a.MemberCount, Confidence: a.Confidence, Members: ms})
	}
	return out, nil
}

func relations(ctx context.Context, s *store.Store) ([]Relation, error) {
	rows, err := s.AllRelations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Relation, 0, len(rows))
	for _, r := range rows {
		out = append(out, Relation{RelationID: r.RelationID, Type: r.Type,
			From: Endpoint{r.FromSource, r.FromLocator}, To: Endpoint{r.ToSource, r.ToLocator},
			Precedence: r.Precedence, Actor: observe.Actor{Kind: r.ActorKind, ID: r.ActorID},
			Evidence: r.Evidence, Confidence: r.Confidence, Explanation: r.Explanation,
			AssertedAt: r.AssertedAt, IdentityPolicy: r.PolicyVersion})
	}
	return out, nil
}
