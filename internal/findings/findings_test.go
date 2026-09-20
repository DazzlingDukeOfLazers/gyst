package findings

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

var now = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

func f(src, loc, digest, obs string) File {
	return File{SourceID: src, Locator: loc, Digest: digest, ObsID: obs, NativeVersion: "1:1", Size: 1}
}

func byRule(fs []Finding, rule string) []Finding {
	var out []Finding
	for _, x := range fs {
		if x.Rule.ID == rule {
			out = append(out, x)
		}
	}
	return out
}

func TestDuplicatesAcrossSources(t *testing.T) {
	fs := Detect(Inputs{Now: now, Files: []File{
		f("repo", "bom.xlsx", "aaa", "obs_1"),
		f("share", "widget/bom.xlsx", "aaa", "obs_2"),
		f("share", "widget/bom (copy).xlsx", "aaa", "obs_3"),
		f("repo", "unique.txt", "bbb", "obs_4"),
	}})
	d := byRule(fs, RuleDuplicateContent)
	if len(d) != 1 {
		t.Fatalf("%d duplicate findings, want 1", len(d))
	}
	if len(d[0].Subjects) != 3 || len(d[0].Evidence) != 3 {
		t.Errorf("subjects %d evidence %d", len(d[0].Subjects), len(d[0].Evidence))
	}
	if !strings.Contains(d[0].Summary, "2 sources") {
		t.Errorf("summary does not say it spans sources: %s", d[0].Summary)
	}
	if d[0].Remediation == nil || !d[0].Remediation.RequiresApproval {
		t.Error("remediation must be a proposal requiring approval")
	}
}

// Every empty file shares one digest. Reporting them as duplicates would be
// confident nonsense, and files never read have nothing to compare.
func TestDuplicatesIgnoreEmptyAndUnread(t *testing.T) {
	fs := Detect(Inputs{Now: now, Files: []File{
		f("s", "a.txt", EmptySHA256, "obs_1"),
		f("s", "b.txt", EmptySHA256, "obs_2"),
		f("s", "c.bin", "", "obs_3"),
		f("s", "d.bin", "", "obs_4"),
	}})
	if len(byRule(fs, RuleDuplicateContent)) != 0 {
		t.Fatal("empty or unread files reported as duplicates")
	}
}

func TestFindingIDIsStableAndOrderIndependent(t *testing.T) {
	a := Detect(Inputs{Now: now, Files: []File{f("s", "x", "d", "obs_1"), f("s", "y", "d", "obs_2")}})
	b := Detect(Inputs{Now: now.Add(time.Hour), Files: []File{f("s", "y", "d", "obs_9"), f("s", "x", "d", "obs_8")}})
	if a[0].FindingID != b[0].FindingID {
		t.Fatalf("same subjects, different ids: %s vs %s; a waiver would not survive a rescan", a[0].FindingID, b[0].FindingID)
	}
}

func TestAmbiguousOriginNeedsTwoCandidates(t *testing.T) {
	one := Detect(Inputs{Now: now, Ambiguities: []Ambiguity{{
		Gone: f("s", "gone.pdf", "d", "obs_g"), Candidates: []File{f("s", "a.pdf", "d", "obs_a")}, Evidence: []string{"obs_g", "obs_a"},
	}}})
	if len(byRule(one, RuleAmbiguousOrigin)) != 0 {
		t.Error("a single candidate is a rename, not an ambiguity")
	}
	two := Detect(Inputs{Now: now, Ambiguities: []Ambiguity{{
		Gone:       f("s", "gone.pdf", "d", "obs_g"),
		Candidates: []File{f("s", "a.pdf", "d", "obs_a"), f("s", "b.pdf", "d", "obs_b")},
		Evidence:   []string{"obs_g", "obs_a", "obs_b"},
	}}})
	am := byRule(two, RuleAmbiguousOrigin)
	if len(am) != 1 || len(am[0].Subjects) != 3 || am[0].Severity != SeverityMedium {
		t.Fatalf("%+v", am)
	}
	if am[0].Confidence >= 1.0 {
		t.Error("an inference about ambiguity is not a directly observed fact")
	}
}

// Relations from two passes can offer the same candidate twice. One file
// is one candidate, and one candidate offered twice is not an ambiguity.
func TestAmbiguityDeduplicatesCandidates(t *testing.T) {
	fs := Detect(Inputs{Now: now, Ambiguities: []Ambiguity{{
		Gone: f("s", "gone.pdf", "d", "obs_g"),
		Candidates: []File{f("s", "a.pdf", "d", "obs_a"), f("s", "a.pdf", "d", "obs_a2")},
		Evidence:   []string{"obs_g", "obs_a"},
	}}})
	if len(byRule(fs, RuleAmbiguousOrigin)) != 0 {
		t.Fatal("a single candidate listed twice was reported as an ambiguity")
	}
	fs = Detect(Inputs{Now: now, Ambiguities: []Ambiguity{{
		Gone: f("s", "gone.pdf", "d", "obs_g"),
		Candidates: []File{f("s", "a.pdf", "d", "obs_a"), f("s", "b.pdf", "d", "obs_b"), f("s", "a.pdf", "d", "obs_a2")},
		Evidence:   []string{"obs_g", "obs_a", "obs_b"},
	}}})
	am := byRule(fs, RuleAmbiguousOrigin)
	if len(am) != 1 || len(am[0].Subjects) != 3 || !strings.Contains(am[0].Summary, "2 present files") {
		t.Fatalf("%+v", am)
	}
}

func TestSourceFreshness(t *testing.T) {
	src := func(status string, age time.Duration, kind string, cadence time.Duration) SourceState {
		return SourceState{SourceID: "s", LocationKind: kind, PassStatus: status,
			PassStarted: now.Add(-age), Cadence: cadence, LatestObsID: "obs_last"}
	}
	cases := []struct {
		name string
		s    SourceState
		want string
	}{
		{"fresh local", src("complete", time.Hour, "local", 0), ""},
		{"stale local by default", src("complete", 30*time.Hour, "local", 0), RuleSourceStale},
		{"share within a week", src("complete", 3*24*time.Hour, "network-share", 0), ""},
		{"share past a week", src("complete", 8*24*time.Hour, "network-share", 0), RuleSourceStale},
		{"configured cadence wins", src("complete", 2*time.Hour, "local", time.Hour), RuleSourceStale},
		{"partial pass still ages", src("partial", 30*time.Hour, "local", 0), RuleSourceStale},
		{"unavailable", src("unavailable", time.Minute, "network-share", 0), RuleSourceUnavail},
		{"interrupted", src("interrupted", time.Minute, "local", 0), RuleSourceInterrupt},
	}
	for _, c := range cases {
		fs := Detect(Inputs{Now: now, Sources: []SourceState{c.s}})
		got := ""
		if len(fs) > 0 {
			got = fs[0].Rule.ID
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Never scanned is a state, not a finding. And a source with nothing
// observed has nothing to cite, so no finding can be made about it.
func TestSourceFindingsNeedEvidence(t *testing.T) {
	fs := Detect(Inputs{Now: now, Sources: []SourceState{
		{SourceID: "never", LocationKind: "local"},
		{SourceID: "empty-unavailable", LocationKind: "local", PassStatus: "unavailable", PassStarted: now},
	}})
	if len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}

func TestParseCadence(t *testing.T) {
	for in, want := range map[string]time.Duration{"7d": 7 * 24 * time.Hour, "12h": 12 * time.Hour, "90m": 90 * time.Minute} {
		if got, err := ParseCadence(in); err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "0d", "-1h", "soon"} {
		if _, err := ParseCadence(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The refusal happens before any store access: a nil store proves it.
func TestOnlyAPersonMayWaive(t *testing.T) {
	err := Waive(context.Background(), nil, "fnd_x", Waiver{
		Actor: observe.Actor{Kind: "rule", ID: "auto-waive/1"}, Reason: "low severity", WaivedAt: now})
	if err == nil || !strings.Contains(err.Error(), "only a person") {
		t.Fatalf("rule waiver not refused: %v", err)
	}
}
