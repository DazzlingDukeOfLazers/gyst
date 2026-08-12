// Package identity groups observed files into logical artifacts.
//
// Everything here is a projection. Grouping is computed from the observation
// log under a named policy version and can be thrown away and rebuilt; no
// function in this package writes to an observation, and switching profiles
// must leave the log byte-identical.
//
// Profiles are ordered, scoped rules with named capture groups, per the
// round-two design. The first rule whose scope and pattern match wins, so
// specific rules are declared before general ones.
package identity

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type Profile string

const (
	ContentPathExact Profile = "content-path-exact"
	SuffixAsVersion  Profile = "suffix-as-version"
	SuffixAsIdentity Profile = "suffix-as-identity"
	CanonicalName    Profile = "canonical-name"
	CompareSet       Profile = "compare-set"
)

func Profiles() []Profile {
	return []Profile{ContentPathExact, SuffixAsVersion, SuffixAsIdentity, CanonicalName, CompareSet}
}

func Valid(p Profile) bool {
	for _, k := range Profiles() {
		if k == p {
			return true
		}
	}
	return false
}

// SupersedesThreshold is the confidence a machine-suggested supersedes relation
// must reach. Below it the correct output is compare-set-with: the files are
// offered side by side and no ordering is asserted.
//
// This number is not derived from the design documents. It was chosen so that
// an explicit revision marker ("rev3", "v2") clears the bar and a bare numeric
// suffix ("connector_124") does not, because the second is far more often a
// part number than a revision. It is the single value that decides whether
// connector_124 appears to supersede connector_123, and it belongs in
// decisions.md rather than buried here.
const SupersedesThreshold = 0.8

// Match is what a profile concluded about one locator.
type Match struct {
	// GroupingKey identifies the logical artifact. Locators sharing a key under
	// the same policy version belong to the same artifact.
	GroupingKey string

	// VersionLabel is the captured revision token, empty when the rule captured
	// none. Ordering within an artifact uses it when it is numeric.
	VersionLabel string

	// Confidence in the grouping, and the reason, shown in preview before a
	// profile is activated and by "gyst explain" afterwards.
	Confidence  float64
	Explanation string

	// Rule that produced the match, for explainability.
	Rule string
}

// OrderableVersion reports the numeric ordering of a version label.
func (m Match) OrderableVersion() (int, bool) {
	if m.VersionLabel == "" {
		return 0, false
	}
	n, err := strconv.Atoi(m.VersionLabel)
	return n, err == nil
}

type rule struct {
	name    string
	scope   string // locator prefix the rule applies to; "" matches everything
	pattern *regexp.Regexp
	conf    float64
	why     string
}

// An explicit revision marker: widget_rev3, drawing-v2, plate.ver10.
// The marker word is what raises confidence -- someone wrote "rev" on purpose.
//
// The separator before the marker is required, not optional. Allowing it to be
// empty let the trailing "r" of "connector_123" serve as the marker, reading
// the file as version 123 of "connecto". A marker has to be its own token.
var reExplicitRevision = regexp.MustCompile(`(?i)^(?P<base>.+?)[._\- ]+(?:rev|ver|version|r|v)[._\- ]*(?P<version>\d+)$`)

// A bare trailing number: connector_124, part 7, sheet-3.
// Structurally identical to a revision and usually not one.
var reBareNumeric = regexp.MustCompile(`^(?P<base>.+?)[._\- ]+(?P<version>\d+)$`)

// Trailing words people append instead of versioning: FINAL, draft, copy.
// Not a revision scheme, and never ordered. Brackets are optional because the
// common machine-generated form is "widget_bom (copy).xlsx".
var reQualifier = regexp.MustCompile(`(?i)^(?P<base>.+?)[._\- ]+[\(\[]?(?:final|draft|copy|old|new|latest|backup|orig|original)[\)\]]?$`)

var versionRules = []rule{
	{
		name:    "explicit-revision-marker",
		pattern: reExplicitRevision,
		conf:    0.88,
		why:     "an explicit revision marker precedes the number",
	},
	{
		name:    "bare-numeric-suffix",
		pattern: reBareNumeric,
		conf:    0.35,
		why: "a bare trailing number with no revision marker; this is at least as " +
			"often a part number as a revision, so no ordering is asserted",
	},
}

// Classify applies a profile to one locator.
func Classify(p Profile, locator string) Match {
	dir := path.Dir(locator)
	ext := strings.ToLower(path.Ext(locator))
	stem := strings.TrimSuffix(path.Base(locator), path.Ext(locator))

	switch p {
	case ContentPathExact:
		return Match{
			GroupingKey: locator,
			Confidence:  1.0,
			Rule:        "path-identity",
			Explanation: "every path is its own artifact; edits create versions of it",
		}

	case SuffixAsIdentity:
		return Match{
			GroupingKey: path.Join(dir, stem) + ext,
			Confidence:  1.0,
			Rule:        "stem-identity",
			Explanation: "the whole name identifies the artifact; a numeric suffix is part of its identity, not a version of something else",
		}

	case SuffixAsVersion:
		for _, r := range versionRules {
			base, version, ok := apply(r.pattern, stem)
			if !ok {
				continue
			}
			return Match{
				GroupingKey:  path.Join(dir, base) + ext,
				VersionLabel: version,
				Confidence:   r.conf,
				Rule:         r.name,
				Explanation: fmt.Sprintf("%q read as version %s of %q: %s",
					stem, version, base, r.why),
			}
		}
		return Match{
			GroupingKey: path.Join(dir, stem) + ext,
			Confidence:  1.0,
			Rule:        "no-version-suffix",
			Explanation: "no version suffix found; the name stands alone",
		}

	case CanonicalName:
		base := canonicalBase(stem)
		return Match{
			GroupingKey: path.Join(dir, base) + ext,
			Confidence:  0.7,
			Rule:        "canonical-name",
			Explanation: fmt.Sprintf("incoming names collapse onto the controlled deliverable %q", base+ext),
		}

	case CompareSet:
		// Deliberately looser than suffix-as-version. Compare-set exists for
		// files that are probably related but whose relationship cannot be
		// determined, so it strips qualifier words a strict rule must not
		// touch. It never orders anything, which is what makes the loose
		// grouping safe.
		base := canonicalBase(stem)
		return Match{
			GroupingKey: path.Join(dir, base) + ext + ":set",
			Confidence:  0.4,
			Rule:        "compare-set",
			Explanation: fmt.Sprintf("grouped with other candidates for %q for side-by-side review; no supersession is implied", base),
		}
	}

	return Match{GroupingKey: locator, Confidence: 0, Rule: "unknown-profile",
		Explanation: "unknown profile"}
}

// canonicalBase strips revision markers, bare numbers, and qualifier words,
// repeatedly, so "Assembly Notes v2 FINAL" and "Assembly Notes v2" reach the
// same base.
func canonicalBase(stem string) string {
	prev := ""
	cur := stem
	for cur != prev {
		prev = cur
		for _, re := range []*regexp.Regexp{reQualifier, reExplicitRevision, reBareNumeric} {
			if base, _, ok := apply(re, cur); ok {
				cur = base
				break
			}
		}
	}
	return strings.TrimSpace(cur)
}

func apply(re *regexp.Regexp, s string) (base, version string, ok bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	for i, name := range re.SubexpNames() {
		switch name {
		case "base":
			base = strings.TrimSpace(m[i])
		case "version":
			version = m[i]
		}
	}
	return base, version, base != ""
}

// RelationFor decides what relation, if any, holds between two members of the
// same artifact.
//
// This is where the threshold does its work. Two files can share a grouping key
// and still not be ordered: sharing a key means "these are probably related",
// and ordering requires knowing how.
func RelationFor(p Profile, newer, older Match) (relType string, confidence float64, explanation string) {
	conf := min(newer.Confidence, older.Confidence)

	nv, nok := newer.OrderableVersion()
	ov, ook := older.OrderableVersion()
	ordered := nok && ook && nv > ov

	if p == SuffixAsVersion && ordered && conf >= SupersedesThreshold {
		return "supersedes", conf, fmt.Sprintf(
			"version %d supersedes version %d under %s; %s", nv, ov, p, newer.Explanation)
	}

	switch {
	case !ordered && (nok || ook):
		explanation = fmt.Sprintf(
			"both carry version-like tokens but no ordering could be established under %s; offered side by side", p)
	case conf < SupersedesThreshold && ordered:
		explanation = fmt.Sprintf(
			"ordering is suggested (%d after %d) but confidence %.2f is below the %.2f required to assert supersession; %s",
			nv, ov, conf, SupersedesThreshold, newer.Explanation)
	default:
		explanation = fmt.Sprintf("grouped under %s for side-by-side comparison; no ordering asserted", p)
	}
	return "compare-set-with", conf, explanation
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
