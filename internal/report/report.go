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
	"encoding/json"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/identity"
	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const Schema = "gyst.report/0.1.0"

type Document struct {
	Report    Meta               `json:"report"`
	Sources   []Source           `json:"sources"`
	Projects  []Project          `json:"projects"`
	Files     []File             `json:"files"`
	Artifacts []Artifact         `json:"artifacts"`
	Relations []Relation         `json:"relations"`
	Findings  []findings.Finding `json:"findings"`
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
	Projects      []FileProject   `json:"projects"`
	Grouping      *Grouping       `json:"grouping"`
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

	polVersion, profile, err := identity.ActivePolicy(ctx, s)
	if err != nil {
		return nil, err
	}
	if polVersion != "" {
		doc.Report.IdentityPolicy = &IdentityPolicy{Version: polVersion, Profile: string(profile)}
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
	var n int64
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM observations`).Scan(&n); err != nil {
		return nil, err
	}
	c.Observations = int(n)
	return doc, nil
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
		var cadence int
		if err := s.Pool().QueryRow(ctx,
			`SELECT cadence_seconds FROM sources WHERE source_id=$1`, p.SourceID).Scan(&cadence); err != nil {
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
		if err := s.Pool().QueryRow(ctx,
			`SELECT count(*) FROM current_files WHERE source_id=$1 AND present`, p.SourceID).Scan(&src.FilesPresent); err != nil {
			return nil, err
		}
		if err := s.Pool().QueryRow(ctx, `
			SELECT count(*) FROM findings f
			WHERE f.status IN ('open','acknowledged')
			  AND EXISTS (SELECT 1 FROM jsonb_array_elements(f.subjects) sj
			              WHERE sj->'location'->>'source_id' = $1)`, p.SourceID).Scan(&src.FindingsOpen); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, nil
}

func projects(ctx context.Context, s *store.Store) ([]Project, error) {
	rows, err := s.Pool().Query(ctx, `
		SELECT p.project_id, p.name, p.description, p.basis, p.confidence, p.explanation, p.evidence,
		       (SELECT count(*) FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT coalesce(array_agg(DISTINCT fp.source_id ORDER BY fp.source_id), '{}')
		          FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT coalesce(json_agg(json_build_object(
		            'source_id', m.source_id, 'pattern', m.pattern, 'basis', m.basis,
		            'confidence', m.confidence, 'evidence', m.evidence) ORDER BY m.source_id, m.pattern), '[]')
		          FROM project_members m WHERE m.project_id=p.project_id)
		FROM projects p ORDER BY p.basis, p.name, p.project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		var members []byte
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.Description, &p.Basis, &p.Confidence, &p.Explanation,
			&p.Evidence, &p.FileCount, &p.SourceIDs, &members); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &p.Members); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func files(ctx context.Context, s *store.Store, policy string) ([]File, error) {
	rows, err := s.Pool().Query(ctx, `
		SELECT cf.source_id, cf.locator, cf.present, cf.size_bytes, cf.content_digest_hex,
		       cf.native_version_value, cf.observed_at, o.observation_id,
		       coalesce(o.policy->>'content_level',''),
		       coalesce((o.claim_payload->>'placeholder')::boolean, false),
		       (SELECT coalesce(json_agg(json_build_object(
		            'project_id', fp.project_id, 'basis', fp.basis,
		            'confidence', fp.confidence, 'pattern', fp.pattern) ORDER BY fp.confidence DESC, fp.project_id), '[]')
		          FROM file_projects fp WHERE fp.source_id=cf.source_id AND fp.locator=cf.locator),
		       m.artifact_id, a.grouping_key, m.version_label, m.is_current, m.rule, m.confidence, m.explanation
		FROM current_files cf
		JOIN observations o ON o.seq = cf.latest_seq
		LEFT JOIN artifact_members m ON m.identity_policy_version = $1
		     AND m.source_id = cf.source_id AND m.locator = cf.locator
		LEFT JOIN artifacts a ON a.identity_policy_version = m.identity_policy_version
		     AND a.artifact_id = m.artifact_id
		ORDER BY cf.source_id, cf.locator`, policy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		var digest *string
		var projects []byte
		var g Grouping
		var artifactID, groupingKey, rule, explanation *string
		var isCurrent *bool
		var conf *float64
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.Present, &f.SizeBytes, &digest,
			&f.NativeVersion, &f.ObservedAt, &f.ObservationID, &f.ContentLevel, &f.Placeholder, &projects,
			&artifactID, &groupingKey, &g.VersionLabel, &isCurrent, &rule, &conf, &explanation); err != nil {
			return nil, err
		}
		if digest != nil {
			f.ContentDigest = &observe.Digest{Algo: "sha256", Hex: *digest}
		}
		if err := json.Unmarshal(projects, &f.Projects); err != nil {
			return nil, err
		}
		if artifactID != nil {
			g.ArtifactID, g.GroupingKey, g.Rule, g.Explanation = *artifactID, *groupingKey, *rule, *explanation
			g.IsCurrent, g.Confidence = *isCurrent, *conf
			f.Grouping = &g
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func artifacts(ctx context.Context, s *store.Store, policy string) ([]Artifact, error) {
	if policy == "" {
		return []Artifact{}, nil
	}
	rows, err := s.Pool().Query(ctx, `
		SELECT a.artifact_id, a.source_id, a.grouping_key, a.member_count, a.confidence,
		       (SELECT coalesce(json_agg(json_build_object(
		            'locator', m.locator, 'version_label', m.version_label, 'is_current', m.is_current,
		            'rule', m.rule, 'confidence', m.confidence, 'explanation', m.explanation) ORDER BY m.locator), '[]')
		          FROM artifact_members m WHERE m.identity_policy_version=a.identity_policy_version
		           AND m.artifact_id=a.artifact_id)
		FROM artifacts a WHERE a.identity_policy_version=$1
		ORDER BY a.source_id, a.grouping_key`, policy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var a Artifact
		var members []byte
		if err := rows.Scan(&a.ArtifactID, &a.SourceID, &a.GroupingKey, &a.MemberCount, &a.Confidence, &members); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &a.Members); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func relations(ctx context.Context, s *store.Store) ([]Relation, error) {
	rows, err := s.Pool().Query(ctx, `
		SELECT relation_id, type, from_source, from_locator, to_source, to_locator,
		       precedence, actor_kind, actor_id, evidence, confidence, explanation, asserted_at,
		       identity_policy_version
		FROM relations ORDER BY type, from_source, from_locator, to_locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Relation{}
	for rows.Next() {
		var r Relation
		if err := rows.Scan(&r.RelationID, &r.Type, &r.From.SourceID, &r.From.Locator,
			&r.To.SourceID, &r.To.Locator, &r.Precedence, &r.Actor.Kind, &r.Actor.ID,
			&r.Evidence, &r.Confidence, &r.Explanation, &r.AssertedAt, &r.IdentityPolicy); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
