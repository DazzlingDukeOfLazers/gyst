// Package findings turns evidence into things that need a person's
// attention: duplicates, ambiguity, sources that have gone quiet, manifests
// that do not parse.
//
// A finding is advisory. It cites the observations behind it, proposes at
// most a remediation, and never performs one. The detectors here are pure
// functions over loaded inputs so every rule is testable without a
// database; Project loads, detects, and upserts.
package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

const (
	SeverityInfo   = "info"
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"

	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusWaived       = "waived"
	StatusResolved     = "resolved"
)

// Rule ids. The version bumps when a rule's meaning changes, so a finding
// under the old version is not mistaken for one under the new.
const (
	RuleDuplicateContent = "hygiene.duplicate-content"
	RuleAmbiguousOrigin  = "hygiene.ambiguous-origin"
	RuleManifestInvalid  = "project.manifest-invalid"
	RuleSourceStale      = "source.stale"
	RuleSourceUnavail    = "source.unavailable"
	RuleSourceInterrupt  = "source.interrupted"
	RuleVersion          = "1"
)

// Finding mirrors schemas/v0/finding.schema.json.
type Finding struct {
	SchemaVersion string                `json:"schema_version"`
	FindingID     string                `json:"finding_id"`
	Rule          Rule                  `json:"rule"`
	Severity      string                `json:"severity"`
	Status        string                `json:"status"`
	Subjects      []observe.ArtifactRef `json:"subjects"`
	Evidence      []string              `json:"evidence"`
	DetectedAt    time.Time             `json:"detected_at"`
	Confidence    float64               `json:"confidence"`
	Summary       string                `json:"summary"`
	Remediation   *Remediation          `json:"remediation,omitempty"`
	Waiver        *Waiver               `json:"waiver,omitempty"`
}

type Rule struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Remediation struct {
	Proposal         string `json:"proposal"`
	Reversible       bool   `json:"reversible"`
	RequiresApproval bool   `json:"requires_approval"`
}

type Waiver struct {
	Actor     observe.Actor `json:"actor"`
	Reason    string        `json:"reason"`
	WaivedAt  time.Time     `json:"waived_at"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}

// ID derives a finding's identity from its rule and subjects. Stable across
// rebuilds, so a disposition survives the next scan; independent of
// evidence ids, so a re-observation of the same files does not open a
// second finding about them.
func ID(ruleID string, subjects []observe.ArtifactRef) string {
	keys := make([]string, 0, len(subjects))
	for _, s := range subjects {
		keys = append(keys, s.Location.SourceID+"\x00"+s.Location.Locator)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(ruleID))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
	}
	return "fnd_" + hex.EncodeToString(h.Sum(nil))[:24]
}

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// File is a present file as the detectors see it.
type File struct {
	SourceID, Locator, Digest, NativeVersion, ObsID string
	Size                                            int64
}

// Ambiguity is one disappearance with several content-matching arrivals,
// from compare-set-with relations.
type Ambiguity struct {
	Gone       File
	Candidates []File
	Evidence   []string
	Confidence float64
}

// BadManifest is a present manifest whose latest observation says invalid.
type BadManifest struct {
	File  File
	Error string
}

// SourceState is what the latest pass says about a source.
type SourceState struct {
	SourceID     string
	LocationKind string
	PassStatus   string // "" for never scanned
	PassStarted  time.Time
	PassDetail   string
	Cadence      time.Duration
	// LatestObsID is the newest observation from this source, or "" if none.
	// A finding about the source cites it: the rule fires because of what is
	// known about the source, and this is the most recent thing known.
	LatestObsID string
}

// Inputs is everything the rules consult.
type Inputs struct {
	Now         time.Time
	Files       []File
	Ambiguities []Ambiguity
	Manifests   []BadManifest
	Sources     []SourceState
}

// Detect runs every rule. Output is sorted by id so a run is diffable.
func Detect(in Inputs) []Finding {
	var out []Finding
	out = append(out, duplicates(in)...)
	out = append(out, ambiguous(in)...)
	out = append(out, badManifests(in)...)
	out = append(out, sources(in)...)
	sort.Slice(out, func(i, j int) bool { return out[i].FindingID < out[j].FindingID })
	return out
}

func fileRef(f File) observe.ArtifactRef {
	ref := observe.ArtifactRef{
		Kind: "file",
		Location: observe.Location{
			SourceID: f.SourceID, Locator: f.Locator,
			NativeVersion: observe.NativeVersion{Scheme: "mtime_size", Value: f.NativeVersion},
		},
	}
	if f.NativeVersion != "" || f.Size > 0 {
		ref.Version = &observe.Version{SizeBytes: f.Size}
	}
	return ref
}

// duplicates: present files sharing a digest. Empty files are excluded --
// every empty file shares one digest and the match means nothing -- and so
// are files without a digest, which policy kept Gyst from reading.
func duplicates(in Inputs) []Finding {
	byDigest := map[string][]File{}
	for _, f := range in.Files {
		if f.Digest == "" || f.Digest == EmptySHA256 {
			continue
		}
		byDigest[f.Digest] = append(byDigest[f.Digest], f)
	}
	var out []Finding
	for digest, group := range byDigest {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if group[i].SourceID != group[j].SourceID {
				return group[i].SourceID < group[j].SourceID
			}
			return group[i].Locator < group[j].Locator
		})
		subjects := make([]observe.ArtifactRef, 0, len(group))
		evidence := make([]string, 0, len(group))
		srcs := map[string]bool{}
		names := make([]string, 0, len(group))
		for _, f := range group {
			subjects = append(subjects, fileRef(f))
			evidence = append(evidence, f.ObsID)
			srcs[f.SourceID] = true
			names = append(names, f.Locator)
		}
		where := "one source"
		if len(srcs) > 1 {
			where = fmt.Sprintf("%d sources", len(srcs))
		}
		out = append(out, Finding{
			SchemaVersion: observe.SchemaVersion,
			FindingID:     ID(RuleDuplicateContent, subjects),
			Rule:          Rule{RuleDuplicateContent, RuleVersion},
			Severity:      SeverityLow,
			Status:        StatusOpen,
			Subjects:      subjects,
			Evidence:      evidence,
			DetectedAt:    in.Now,
			Confidence:    1.0,
			Summary: fmt.Sprintf("%d byte-identical files (sha256 %s) in %s: %s. None is marked authoritative, so a reader cannot tell which to use.",
				len(group), digest[:min(12, len(digest))], where, strings.Join(names, ", ")),
			Remediation: &Remediation{
				Proposal:         "Nominate one as the authoritative copy and relate the others to it; or, if they are deliberate mirrors, record that.",
				Reversible:       true,
				RequiresApproval: true,
			},
		})
	}
	return out
}

// ambiguous: a file disappeared and more than one present file matches its
// content. The rename detector refused to choose; this is where a person
// is asked to.
func ambiguous(in Inputs) []Finding {
	var out []Finding
	for _, a := range in.Ambiguities {
		// The same candidate can be offered by relations from several
		// passes. One file is one candidate.
		seen := map[string]bool{}
		var cands []File
		for _, c := range a.Candidates {
			k := c.SourceID + "\x00" + c.Locator
			if !seen[k] {
				seen[k] = true
				cands = append(cands, c)
			}
		}
		if len(cands) < 2 {
			continue
		}
		subjects := []observe.ArtifactRef{fileRef(a.Gone)}
		names := make([]string, 0, len(cands))
		for _, c := range cands {
			subjects = append(subjects, fileRef(c))
			names = append(names, c.Locator)
		}
		out = append(out, Finding{
			SchemaVersion: observe.SchemaVersion,
			FindingID:     ID(RuleAmbiguousOrigin, subjects),
			Rule:          Rule{RuleAmbiguousOrigin, RuleVersion},
			Severity:      SeverityMedium,
			Status:        StatusOpen,
			Subjects:      subjects,
			Evidence:      a.Evidence,
			DetectedAt:    in.Now,
			Confidence:    0.9,
			Summary: fmt.Sprintf("%s is gone and %d present files match its content: %s. The evidence cannot say which, if any, is where it went.",
				a.Gone.Locator, len(cands), strings.Join(names, ", ")),
			Remediation: &Remediation{
				Proposal:         "Compare the candidates and record which one, if any, continues the missing file.",
				Reversible:       true,
				RequiresApproval: true,
			},
		})
	}
	return out
}

func badManifests(in Inputs) []Finding {
	var out []Finding
	for _, m := range in.Manifests {
		subjects := []observe.ArtifactRef{fileRef(m.File)}
		out = append(out, Finding{
			SchemaVersion: observe.SchemaVersion,
			FindingID:     ID(RuleManifestInvalid, subjects),
			Rule:          Rule{RuleManifestInvalid, RuleVersion},
			Severity:      SeverityMedium,
			Status:        StatusOpen,
			Subjects:      subjects,
			Evidence:      []string{m.File.ObsID},
			DetectedAt:    in.Now,
			Confidence:    1.0,
			Summary: fmt.Sprintf("%s could not be parsed, so the project it declares does not exist: %s",
				m.File.Locator, m.Error),
			Remediation: &Remediation{
				Proposal:         "Fix the manifest. Until then membership falls back to native markers.",
				Reversible:       true,
				RequiresApproval: true,
			},
		})
	}
	return out
}

// DefaultCadence is how often a source is expected to be scanned when no
// cadence was configured. A share is scanned on a schedule and a week
// between passes is normal; a local folder that has not been looked at
// for a day is behind.
func DefaultCadence(locationKind string) time.Duration {
	switch locationKind {
	case "network-share":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// sources: the latest pass on a source is stale, unavailable, or
// interrupted. Never scanned is a state the status table shows, not a
// finding. A source with no observation at all cannot be the subject of a
// finding, because there is nothing to cite.
func sources(in Inputs) []Finding {
	var out []Finding
	for _, s := range in.Sources {
		if s.PassStatus == "" || s.LatestObsID == "" {
			continue
		}
		cadence := s.Cadence
		if cadence <= 0 {
			cadence = DefaultCadence(s.LocationKind)
		}
		subject := observe.ArtifactRef{
			Kind: "folder",
			Location: observe.Location{
				SourceID: s.SourceID, Locator: ".",
				// A source root has no native version of its own; the pass
				// that last looked at it is the closest thing.
				NativeVersion: observe.NativeVersion{Scheme: "none", Value: "pass:" + s.PassStarted.UTC().Format(time.RFC3339)},
			},
		}
		age := in.Now.Sub(s.PassStarted)
		var rule, severity, summary string
		switch {
		case s.PassStatus == "unavailable":
			rule, severity = RuleSourceUnavail, SeverityMedium
			summary = fmt.Sprintf("%s could not be reached by its latest pass (%s ago): %s. What is shown for it is what was last known.",
				s.SourceID, ago(age), s.PassDetail)
		case s.PassStatus == "interrupted":
			rule, severity = RuleSourceInterrupt, SeverityLow
			summary = fmt.Sprintf("%s: the latest pass (%s ago) began and never reported a result. Coverage is unknown.",
				s.SourceID, ago(age))
		case age > cadence:
			rule, severity = RuleSourceStale, SeverityMedium
			summary = fmt.Sprintf("%s was last scanned %s ago; it is expected every %s. What is shown for it may be out of date.",
				s.SourceID, ago(age), ago(cadence))
		default:
			continue
		}
		out = append(out, Finding{
			SchemaVersion: observe.SchemaVersion,
			FindingID:     ID(rule, []observe.ArtifactRef{subject}),
			Rule:          Rule{rule, RuleVersion},
			Severity:      severity,
			Status:        StatusOpen,
			Subjects:      []observe.ArtifactRef{subject},
			Evidence:      []string{s.LatestObsID},
			DetectedAt:    in.Now,
			Confidence:    1.0,
			Summary:       summary,
		})
	}
	return out
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// EmptySHA256 is the digest of zero bytes.
const EmptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// ParseCadence reads "7d", "12h", "90m", or a Go duration.
func ParseCadence(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err != nil || n <= 0 {
			return 0, fmt.Errorf("cadence %q: want a positive number of days", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("cadence %q: want e.g. 7d, 12h, 30m", s)
	}
	return d, nil
}
