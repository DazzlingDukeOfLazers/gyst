// Package observe holds the observation envelope: the only record in Gyst that
// is evidence rather than derivation.
//
// These types mirror schemas/v0/*.json. The Go structs and the JSON Schemas are
// two statements of one contract, kept in step by hand today: envelope_test.go
// checks the invariants that matter most on the Go side, but nothing yet
// validates a connector's output against the schemas at build time. Wiring that
// up is worth doing before a second connector exists.
package observe

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = "0.1.0"

// Content levels, most restrictive first. The effective level decides whether a
// connector may read bytes at all, so it gates whether Version.ContentDigest
// can legally be populated.
const (
	ContentExclude      = "exclude"
	ContentMetadata     = "metadata"
	ContentFingerprint  = "fingerprint"
	ContentExtractLocal = "extract-local"
	ContentFull         = "content"
)

// PermitsDigest reports whether a content level allows recording a digest. The
// database enforces this too; duplicating it here means a connector fails fast
// with a clear message instead of hitting a constraint violation.
func PermitsDigest(level string) bool {
	switch level {
	case ContentFingerprint, ContentExtractLocal, ContentFull:
		return true
	default:
		return false
	}
}

type Digest struct {
	Algo string `json:"algo"`
	Hex  string `json:"hex"`
}

type NativeVersion struct {
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
}

type Location struct {
	SourceID      string        `json:"source_id"`
	Locator       string        `json:"locator"`
	NativeVersion NativeVersion `json:"native_version"`
}

type Version struct {
	ContentDigest *Digest `json:"content_digest,omitempty"`
	SizeBytes     int64   `json:"size_bytes"`
}

// ArtifactRef deliberately has no Artifact field. Logical grouping is a
// projection; an observation that carried one would make identity part of the
// evidence record and defeat reversible identity profiles.
type ArtifactRef struct {
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
	Version  *Version `json:"version,omitempty"`
}

type Source struct {
	SourceID         string `json:"source_id"`
	Connector        string `json:"connector"`
	ConnectorVersion string `json:"connector_version"`
	Cursor           string `json:"cursor,omitempty"`
}

type Claim struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

type Extractor struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	InputDigest  *Digest  `json:"input_digest,omitempty"`
	OutputSchema string   `json:"output_schema"`
	Warnings     []string `json:"warnings"`
	Confidence   float64  `json:"confidence"`
}

type Policy struct {
	ContentLevel           string `json:"content_level"`
	Egress                 string `json:"egress"`
	EffectivePolicyVersion string `json:"effective_policy_version"`
}

type Visibility struct {
	Labels            []string `json:"labels"`
	SourceACLComplete bool     `json:"source_acl_complete"`
}

type Observation struct {
	SchemaVersion string      `json:"schema_version"`
	ObservationID string      `json:"observation_id"`
	ObservedAt    time.Time   `json:"observed_at"`
	Source        Source      `json:"source"`
	Subject       ArtifactRef `json:"subject"`
	Claim         Claim       `json:"claim"`
	Extractor     Extractor   `json:"extractor"`
	Policy        Policy      `json:"policy"`
	Visibility    Visibility  `json:"visibility"`
	Corrects      string      `json:"corrects,omitempty"`
}

// DeriveID computes a deterministic observation id for a state transition.
//
// priorSeq is the seq of the observation this one supersedes, or 0 if the
// locator is new. Including it is not decoration: without it, an id identifies
// a *state*, and re-observing a state the log has already seen collides and
// appends nothing.
//
// That failure is not hypothetical. A file edited and then restored to a
// byte-identical earlier state -- same mtime, same size, which a checkout or a
// backup restore reproduces exactly -- re-derived its original id, collided,
// and left the projection asserting the intermediate content permanently. With
// priorSeq the same bytes observed after a different history derive a different
// id, so the transition is recorded.
//
// Callers must not use this to decide whether to append. Unchanged files are
// filtered against known state before they ever reach here, so an id collision
// is now a genuine duplicate rather than the mechanism.
//
// Note what is excluded: observed_at. Including the clock would make every scan
// produce fresh ids and the log would grow without bound on an idle tree.
//
// Note what is included: the extractor name and version. A new extractor
// version re-observing the same bytes is a new claim about them, not a
// duplicate, and must be recorded rather than silently dropped.
func DeriveID(o *Observation, priorSeq int64) string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	write(
		o.Source.SourceID,
		o.Subject.Location.Locator,
		o.Subject.Location.NativeVersion.Scheme,
		o.Subject.Location.NativeVersion.Value,
		o.Claim.Type,
		o.Extractor.Name,
		o.Extractor.Version,
		strconv.FormatInt(priorSeq, 10),
	)
	return "obs_" + hex.EncodeToString(h.Sum(nil))[:32]
}

// KnownState is what the projection already believes about a locator. A scan
// compares against it to decide whether anything happened at all.
type KnownState struct {
	NativeVersion string
	Seq           int64
}

// TombstoneID derives the id for an absence claim. A vanished file has no
// native version, so the id is keyed on the locator and the seq of the
// observation whose subject went missing.
//
// priorSeq is required for the same reason it is on DeriveID, and this is the
// case where forgetting it bites hardest: delete a file, restore it, delete it
// again, and a state-derived id would collide with the first tombstone. The
// second deletion would append nothing and the projection would show the file
// as present forever.
func TombstoneID(sourceID, locator string, priorSeq int64) string {
	h := sha256.New()
	h.Write([]byte(sourceID + "\x00" + locator + "\x00artifact.absent\x00" +
		strconv.FormatInt(priorSeq, 10)))
	return "obs_" + hex.EncodeToString(h.Sum(nil))[:32]
}

// Tombstone builds an absence observation for a locator that was known but is
// no longer present.
//
// Absence is only evidence when the observer actually looked everywhere it
// should have. Callers must not build one from a partial or resumed pass -- see
// localfolder.Result.Complete.
func Tombstone(sourceID, locator string, prior KnownState, policy Policy, observedAt time.Time) Observation {
	obs := Observation{
		SchemaVersion: SchemaVersion,
		ObservationID: TombstoneID(sourceID, locator, prior.Seq),
		ObservedAt:    observedAt,
		Source: Source{
			SourceID:         sourceID,
			Connector:        "local-folder",
			ConnectorVersion: "0.1.0",
		},
		Subject: ArtifactRef{
			Kind: "file",
			Location: Location{
				SourceID: sourceID,
				Locator:  locator,
				// The last version known to exist. Recording it keeps the
				// tombstone joinable to what it is a statement about.
				NativeVersion: NativeVersion{Scheme: "mtime_size", Value: prior.NativeVersion},
			},
		},
		Claim: Claim{
			Type: "artifact.absent",
			Payload: map[string]any{
				"last_known_native_version": prior.NativeVersion,
				"last_known_seq":            prior.Seq,
			},
		},
		Extractor: Extractor{
			Name:         "absence-detector",
			Version:      "0.1.0",
			OutputSchema: "gyst.claim.artifact.absent/0.1.0",
			Warnings:     []string{},
			// Not 1.0. A complete pass failing to find a file is strong
			// evidence of deletion but not proof: it may have been moved,
			// permissions may have changed, or a mount may have been absent.
			Confidence: 0.9,
		},
		Policy: policy,
		Visibility: Visibility{
			Labels:            []string{"src:" + sourceID + ":read"},
			SourceACLComplete: true,
		},
	}
	return obs
}

// NormalizeLocator makes locators comparable across platforms. Paths always use
// forward slashes so a tree scanned on Windows and on macOS produces the same
// locator for the same file.
func NormalizeLocator(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
