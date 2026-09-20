package findings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// Stats reports what a detection pass did.
type Stats struct {
	Detected, New, Reopened, Resolved int
	Open                              int // open + acknowledged after the pass
}

// Project loads the inputs, runs every rule, and reconciles the result with
// what is already recorded. New findings open; re-detected ones keep their
// disposition unless a waiver has expired; findings no longer produced are
// resolved with a timestamp, never deleted.
func Project(ctx context.Context, s *store.Store, now time.Time) (Stats, error) {
	var st Stats
	in, err := load(ctx, s, now)
	if err != nil {
		return st, err
	}
	found := Detect(in)
	st.Detected = len(found)

	for _, f := range found {
		subjects, _ := json.Marshal(f.Subjects)
		var remediation []byte
		if f.Remediation != nil {
			remediation, _ = json.Marshal(f.Remediation)
		}
		prior, err := s.UpsertFinding(ctx, store.FindingRow{
			FindingID: f.FindingID, RuleID: f.Rule.ID, RuleVersion: f.Rule.Version, Severity: f.Severity,
			Subjects: subjects, Evidence: f.Evidence, Confidence: f.Confidence, Summary: f.Summary,
			Remediation: remediation,
		}, now)
		if err != nil {
			return st, err
		}
		switch {
		case prior == nil:
			st.New++
		case *prior == StatusResolved:
			st.Reopened++
		}
	}
	// Anything not produced by this pass is no longer a finding.
	if st.Resolved, err = s.ResolveUnseenFindings(ctx, now); err != nil {
		return st, err
	}
	st.Open, err = s.CountOpenFindings(ctx, "")
	return st, err
}

func load(ctx context.Context, s *store.Store, now time.Time) (Inputs, error) {
	in := Inputs{Now: now}

	present, err := s.PresentFiles(ctx)
	if err != nil {
		return in, err
	}
	for _, f := range present {
		in.Files = append(in.Files, File{SourceID: f.SourceID, Locator: f.Locator, Digest: f.Digest,
			NativeVersion: f.NativeVersion, ObsID: f.ObsID, Size: f.Size})
	}

	// Ambiguities: compare-set-with relations whose gone side is still
	// absent and whose candidates are still present. One group per gone
	// file; the rows arrive ordered by gone file.
	amb, err := s.Ambiguities(ctx)
	if err != nil {
		return in, err
	}
	var cur *Ambiguity
	for _, r := range amb {
		gone := File{SourceID: r.GoneSource, Locator: r.GoneLocator, NativeVersion: r.GoneNativeVersion, ObsID: r.GoneObsID}
		cand := File{SourceID: r.CandSource, Locator: r.CandLocator, NativeVersion: r.CandNativeVersion,
			Size: r.CandSize, Digest: r.CandDigest, ObsID: r.CandObsID}
		if cur == nil || cur.Gone.SourceID != gone.SourceID || cur.Gone.Locator != gone.Locator {
			in.Ambiguities = append(in.Ambiguities, Ambiguity{Gone: gone, Confidence: r.Confidence})
			cur = &in.Ambiguities[len(in.Ambiguities)-1]
		}
		cur.Candidates = append(cur.Candidates, cand)
		cur.Evidence = appendUnique(cur.Evidence, r.Evidence...)
	}

	bad, errs, err := s.InvalidManifests(ctx)
	if err != nil {
		return in, err
	}
	for i, f := range bad {
		in.Manifests = append(in.Manifests, BadManifest{
			File:  File{SourceID: f.SourceID, Locator: f.Locator, NativeVersion: f.NativeVersion, Size: f.Size, ObsID: f.ObsID},
			Error: errs[i],
		})
	}

	states, err := s.SourceStates(ctx)
	if err != nil {
		return in, err
	}
	for _, r := range states {
		in.Sources = append(in.Sources, SourceState{
			SourceID: r.SourceID, LocationKind: r.LocationKind, PassStatus: r.PassStatus,
			PassStarted: r.PassStarted, PassDetail: r.PassDetail,
			Cadence: time.Duration(r.CadenceSeconds) * time.Second, LatestObsID: r.LatestObsID,
		})
	}
	return in, nil
}

func appendUnique(dst []string, xs ...string) []string {
	seen := map[string]bool{}
	for _, d := range dst {
		seen[d] = true
	}
	for _, x := range xs {
		if !seen[x] {
			dst = append(dst, x)
			seen[x] = true
		}
	}
	return dst
}

// Acknowledge records that a person has seen a finding.
func Acknowledge(ctx context.Context, s *store.Store, id, by string) error {
	if by == "" {
		return fmt.Errorf("--by is required: an acknowledgement is a person's act")
	}
	ok, err := s.AcknowledgeFinding(ctx, id, by)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no open finding %s", id)
	}
	return nil
}

// Waive records a person's decision that a finding does not need action.
// Only a person may do this; the schema and this function both refuse a
// rule or any other actor kind.
func Waive(ctx context.Context, s *store.Store, id string, w Waiver) error {
	if w.Actor.Kind != "user" {
		return fmt.Errorf("only a person may waive a finding; actor kind %q refused", w.Actor.Kind)
	}
	if w.Actor.ID == "" || w.Reason == "" {
		return fmt.Errorf("a waiver needs who (--by) and why (--reason)")
	}
	body, _ := json.Marshal(w)
	ok, err := s.WaiveFinding(ctx, id, w.Actor.ID, body)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no open or acknowledged finding %s", id)
	}
	return nil
}

// Row is a finding as read back, with its bookkeeping.
type Row struct {
	Finding
	LastSeenAt time.Time
	ResolvedAt *time.Time
	DisposedBy string
}

// List returns findings. With all=false, only open and acknowledged.
func List(ctx context.Context, s *store.Store, all bool) ([]Row, error) {
	rows, err := s.ListFindings(ctx, all)
	if err != nil {
		return nil, err
	}
	return convert(rows)
}

// ForFile returns every non-resolved finding that names a file as a subject.
func ForFile(ctx context.Context, s *store.Store, sourceID, locator string) ([]Row, error) {
	rows, err := s.FindingsForFile(ctx, sourceID, locator)
	if err != nil {
		return nil, err
	}
	return convert(rows)
}

func convert(rows []store.FindingRow) ([]Row, error) {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		f := Row{
			Finding: Finding{
				SchemaVersion: observe.SchemaVersion, FindingID: r.FindingID,
				Rule: Rule{r.RuleID, r.RuleVersion}, Severity: r.Severity, Status: r.Status,
				Evidence: r.Evidence, DetectedAt: r.DetectedAt, Confidence: r.Confidence, Summary: r.Summary,
			},
			LastSeenAt: r.LastSeenAt, ResolvedAt: r.ResolvedAt, DisposedBy: r.DisposedBy,
		}
		if err := json.Unmarshal(r.Subjects, &f.Subjects); err != nil {
			return nil, err
		}
		if len(r.Remediation) > 0 {
			f.Remediation = &Remediation{}
			_ = json.Unmarshal(r.Remediation, f.Remediation)
		}
		if len(r.Waiver) > 0 {
			f.Waiver = &Waiver{}
			_ = json.Unmarshal(r.Waiver, f.Waiver)
		}
		out = append(out, f)
	}
	return out, nil
}
