package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// Member is one locator's place in a grouping, before it is written anywhere.
type Member struct {
	SourceID  string
	Locator   string
	LatestSeq int64
	Digest    string
	Match     Match
	IsCurrent bool
}

// Group is one logical artifact under a profile.
type Group struct {
	ArtifactID  string
	SourceID    string
	GroupingKey string
	Confidence  float64
	Members     []Member
}

// Plan is a complete grouping, computed but not stored.
//
// Preview and apply share this type deliberately. What a user is shown before
// activating a profile has to be the same object that gets written, or the
// preview is a separate implementation that can disagree with reality.
type Plan struct {
	Profile   Profile
	Version   string
	Groups    []Group
	Relations []Relation
}

type Relation struct {
	RelationID  string
	Type        string
	FromSource  string
	FromLocator string
	ToSource    string
	ToLocator   string
	Precedence  string
	ActorKind   string
	ActorID     string
	Evidence    []string
	Confidence  float64
	Explanation string
}

// Build computes a grouping for every current file, without writing anything.
func Build(ctx context.Context, s *store.Store, profile Profile, version string) (*Plan, error) {
	files, err := s.PresentFiles(ctx)
	if err != nil {
		return nil, err
	}

	byKey := map[string]*Group{}
	obsID := map[string]string{}
	var order []string

	for _, f := range files {
		m := Member{SourceID: f.SourceID, Locator: f.Locator, LatestSeq: f.LatestSeq, Digest: f.Digest}
		m.Match = Classify(profile, m.Locator)
		obsID[m.SourceID+"\x00"+m.Locator] = f.ObsID

		key := m.SourceID + "\x00" + m.Match.GroupingKey
		g, seen := byKey[key]
		if !seen {
			g = &Group{
				ArtifactID:  artifactID(version, m.SourceID, m.Match.GroupingKey),
				SourceID:    m.SourceID,
				GroupingKey: m.Match.GroupingKey,
				Confidence:  m.Match.Confidence,
			}
			byKey[key] = g
			order = append(order, key)
		}
		// A group is only as trustworthy as its least confident member.
		if m.Match.Confidence < g.Confidence {
			g.Confidence = m.Match.Confidence
		}
		g.Members = append(g.Members, m)
	}

	plan := &Plan{Profile: profile, Version: version}
	for _, key := range order {
		g := byKey[key]
		rankMembers(g)
		plan.Groups = append(plan.Groups, *g)
		plan.Relations = append(plan.Relations, relationsFor(profile, version, g, obsID)...)
	}
	return plan, nil
}

// rankMembers orders a group's members and marks the current one.
//
// Ordering uses the numeric version label when every member has one. Where it
// does not, no member is marked current: claiming one is "the current file"
// without knowing the ordering is exactly the confident-but-wrong behaviour the
// product measures itself against.
func rankMembers(g *Group) {
	allNumbered := len(g.Members) > 0
	for _, m := range g.Members {
		if _, ok := m.Match.OrderableVersion(); !ok {
			allNumbered = false
			break
		}
	}

	if allNumbered && g.Confidence >= SupersedesThreshold {
		sort.Slice(g.Members, func(i, j int) bool {
			a, _ := g.Members[i].Match.OrderableVersion()
			b, _ := g.Members[j].Match.OrderableVersion()
			return a < b
		})
		g.Members[len(g.Members)-1].IsCurrent = true
		return
	}

	sort.Slice(g.Members, func(i, j int) bool { return g.Members[i].Locator < g.Members[j].Locator })
	if len(g.Members) == 1 {
		// A group of one is unambiguous whatever the profile.
		g.Members[0].IsCurrent = true
	}
}

func relationsFor(profile Profile, version string, g *Group, obsID map[string]string) []Relation {
	if len(g.Members) < 2 {
		return nil
	}
	var out []Relation
	for i := 1; i < len(g.Members); i++ {
		newer, older := g.Members[i], g.Members[i-1]
		relType, conf, why := RelationFor(profile, newer.Match, older.Match)

		evidence := []string{}
		for _, m := range []Member{newer, older} {
			if id := obsID[m.SourceID+"\x00"+m.Locator]; id != "" {
				evidence = append(evidence, id)
			}
		}
		if len(evidence) == 0 {
			// Refuse to emit a relation with nothing behind it. The database
			// constraint would reject it anyway; failing here keeps the reason
			// legible.
			continue
		}

		out = append(out, Relation{
			RelationID: relationID(version, relType,
				newer.SourceID, newer.Locator, older.SourceID, older.Locator),
			Type:        relType,
			FromSource:  newer.SourceID,
			FromLocator: newer.Locator,
			ToSource:    older.SourceID,
			ToLocator:   older.Locator,
			Precedence:  "gyst_suggestion",
			ActorKind:   "suggestion",
			ActorID:     "identity." + string(profile) + "/1",
			Evidence:    evidence,
			Confidence:  conf,
			Explanation: why,
		})
	}
	return out
}

func artifactID(version, sourceID, key string) string {
	h := sha256.Sum256([]byte(version + "\x00" + sourceID + "\x00" + key))
	return "art_" + hex.EncodeToString(h[:])[:24]
}

// relationID must include the source of each endpoint, not just the locator.
// Two sources scanning trees that both contain "widget_rev3.pdf" produce
// distinct relations about distinct files; keying on locators alone collided
// them and the second insert failed on the primary key.
func relationID(version, relType, fromSource, from, toSource, to string) string {
	h := sha256.Sum256([]byte(version + "\x00" + relType + "\x00" +
		fromSource + "\x00" + from + "\x00" + toSource + "\x00" + to))
	return "rel_" + hex.EncodeToString(h[:])[:24]
}

// Apply writes a plan under its policy version and makes it active.
//
// Previously active policies are deactivated but retained, so a release that
// pinned an earlier interpretation can still resolve it.
func Apply(ctx context.Context, s *store.Store, plan *Plan) error {
	var artifacts []store.ArtifactRow
	var members []store.ArtifactMemberRow
	for _, g := range plan.Groups {
		artifacts = append(artifacts, store.ArtifactRow{
			ArtifactID: g.ArtifactID, SourceID: g.SourceID, GroupingKey: g.GroupingKey,
			MemberCount: len(g.Members), Confidence: g.Confidence,
		})
		for _, m := range g.Members {
			members = append(members, store.ArtifactMemberRow{
				ArtifactID: g.ArtifactID, SourceID: m.SourceID, Locator: m.Locator, LatestSeq: m.LatestSeq,
				VersionLabel: nullIfEmpty(m.Match.VersionLabel), IsCurrent: m.IsCurrent,
				Rule: m.Match.Rule, Confidence: m.Match.Confidence, Explanation: m.Match.Explanation,
			})
		}
	}
	var relations []store.RelationRow
	for _, r := range plan.Relations {
		relations = append(relations, store.RelationRow{
			RelationID: r.RelationID, Type: r.Type,
			FromSource: r.FromSource, FromLocator: r.FromLocator, ToSource: r.ToSource, ToLocator: r.ToLocator,
			Precedence: r.Precedence, ActorKind: r.ActorKind, ActorID: r.ActorID,
			Evidence: r.Evidence, Confidence: r.Confidence, Explanation: r.Explanation,
		})
	}
	return s.ApplyIdentityPlan(ctx, plan.Version, string(plan.Profile), artifacts, members, relations)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// LogFingerprint hashes the observation log. Comparing it before and after a
// profile switch is the day 3 exit criterion: grouping may change, evidence
// may not.
func LogFingerprint(ctx context.Context, s *store.Store) (string, int64, error) {
	return s.LogFingerprint(ctx)
}

// ActivePolicy returns the active policy version and profile.
func ActivePolicy(ctx context.Context, s *store.Store) (version string, profile Profile, err error) {
	v, p, err := s.ActivePolicy(ctx)
	return v, Profile(p), err
}
