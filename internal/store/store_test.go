package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// One suite, every engine. SQLite always runs, on a temporary file.
// PostgreSQL runs when GYST_TEST_DATABASE_URL names a disposable database:
// the suite drops and recreates its public schema from migrations/, so do
// not point it at anything you want to keep.
func engines(t *testing.T) map[string]*Store {
	t.Helper()
	ctx := context.Background()
	out := map[string]*Store{}

	sq, err := OpenDSN(ctx, "sqlite:"+filepath.Join(t.TempDir(), "suite.db"))
	if err != nil {
		t.Fatal(err)
	}
	out[EngineSQLite] = sq

	if dsn := os.Getenv("GYST_TEST_DATABASE_URL"); dsn != "" {
		pg, err := OpenDSN(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pg.db.exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
			t.Fatal(err)
		}
		files, _ := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
		sort.Strings(files)
		for _, f := range files {
			body, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pg.db.exec(ctx, string(body)); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
		}
		out[EnginePostgres] = pg
	}
	return out
}

var clock = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

func obs(kind, source, locator, claim, digest string, size int64, level string, payload map[string]any, at time.Time) observe.Observation {
	o := observe.Observation{
		SchemaVersion: observe.SchemaVersion,
		ObservedAt:    at,
		Source:        observe.Source{SourceID: source, Connector: "local-folder", ConnectorVersion: "0.1.0"},
		Subject: observe.ArtifactRef{Kind: kind, Location: observe.Location{SourceID: source, Locator: locator,
			NativeVersion: observe.NativeVersion{Scheme: "mtime_size", Value: "1:" + itoa(int(size))}}},
		Claim:      observe.Claim{Type: claim, Payload: payload},
		Extractor:  observe.Extractor{Name: "t", Version: "1", OutputSchema: "t/1", Warnings: []string{}, Confidence: 1},
		Policy:     observe.Policy{ContentLevel: level, Egress: "device", EffectivePolicyVersion: "p1"},
		Visibility: observe.Visibility{Labels: []string{"src:" + source + ":read"}, SourceACLComplete: true},
	}
	if kind == "commit" {
		o.Subject.Location.NativeVersion = observe.NativeVersion{Scheme: "git_oid", Value: strings.TrimPrefix(locator, source+"@")}
	}
	if kind == "file" && claim != "artifact.absent" {
		o.Subject.Version = &observe.Version{SizeBytes: size}
		if digest != "" {
			o.Subject.Version.ContentDigest = &observe.Digest{Algo: "sha256", Hex: digest}
		}
	}
	if payload == nil {
		o.Claim.Payload = map[string]any{}
	}
	o.ObservationID = observe.DeriveID(&o, 0)
	return o
}

func TestStoreOnEveryEngine(t *testing.T) {
	for name, s := range engines(t) {
		t.Run(name, func(t *testing.T) { suite(t, s) })
	}
}

func suite(t *testing.T, s *Store) {
	ctx := context.Background()
	const src = "src"
	d1 := strings.Repeat("a", 64)

	// --- log --------------------------------------------------------------
	batch := []observe.Observation{
		obs("file", src, "bom.xlsx", "file.content_fingerprint", d1, 10, "fingerprint", nil, clock),
		obs("file", src, "bom (copy).xlsx", "file.content_fingerprint", d1, 10, "fingerprint", nil, clock),
		obs("file", src, "fw/main.c", "file.metadata", "", 7, "metadata", nil, clock),
		obs("file", src, ".gyst/project.yaml", "file.content_fingerprint", strings.Repeat("b", 64), 20, "fingerprint", nil, clock),
		obs("file", src, ".gyst/project.yaml", "project.manifest", strings.Repeat("b", 64), 20, "fingerprint",
			map[string]any{"valid": true, "id": "p", "name": "P", "members": []string{"**"}}, clock),
		obs("folder", src, "fw", "folder.metadata", "", 0, "fingerprint", map[string]any{"markers": []string{"git"}}, clock),
		obs("commit", src, src+"@abc", "git.commit", "", 0, "fingerprint",
			map[string]any{"author": "a", "message": "m", "authored_at": "2026-09-20T11:00:00Z", "parents": []string{}, "changed_paths": []string{"fw/main.c"}}, clock),
	}
	n, err := s.Append(ctx, batch)
	if err != nil || n != len(batch) {
		t.Fatalf("append: %d, %v", n, err)
	}
	if n, _ := s.Append(ctx, batch); n != 0 {
		t.Errorf("re-append inserted %d; the log must deduplicate", n)
	}
	if _, err := s.db.exec(ctx, `UPDATE observations SET locator='x' WHERE locator='bom.xlsx'`); err == nil {
		t.Error("observations were updated; the log must be append-only")
	}
	if _, err := s.db.exec(ctx, `DELETE FROM observations WHERE locator='bom.xlsx'`); err == nil {
		t.Error("observations were deleted; the log must be append-only")
	}
	fp1, count, err := s.LogFingerprint(ctx)
	if err != nil || count != int64(len(batch)) {
		t.Fatalf("fingerprint %v count %d", err, count)
	}

	// --- projection ---------------------------------------------------------
	st, err := s.ApplyCurrentFiles(ctx)
	if err != nil || st.Applied != 4 {
		t.Fatalf("apply: %+v %v", st, err)
	}
	files, err := s.PresentFiles(ctx)
	if err != nil || len(files) != 4 {
		t.Fatalf("present: %d %v", len(files), err)
	}
	if files[0].Locator != ".gyst/project.yaml" || files[1].Digest != d1 || files[1].ObsID == "" {
		t.Errorf("present rows %+v", files[:2])
	}
	if !files[0].ObservedAt.Equal(clock) {
		t.Errorf("observed_at round trip: %v", files[0].ObservedAt)
	}
	if f, err := s.FindPresentFile(ctx, "copy).xlsx"); err != nil || f.Locator != "bom (copy).xlsx" {
		t.Errorf("suffix match: %+v %v", f, err)
	}
	if _, err := s.FindPresentFile(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing file: %v", err)
	}
	if f, err := s.PresentFileAt(ctx, src, "fw/main.c"); err != nil || f.Digest != "" || f.Size != 7 {
		t.Errorf("metadata-only file: %+v %v", f, err)
	}
	if ids, _ := s.LatestObservationOf(ctx, src); ids == "" {
		t.Error("no latest observation")
	}
	if rows, err := s.ObservationsOf(ctx, src, ".gyst/project.yaml"); err != nil || len(rows) != 2 || rows[0].ContentLevel != "fingerprint" {
		t.Errorf("observations of manifest: %d %v", len(rows), err)
	}
	if rows, _ := s.RecentObservations(ctx, clock.Add(-time.Hour), 10); len(rows) != len(batch) {
		t.Errorf("recent: %d", len(rows))
	}
	before, after, _, err := s.VerifyProjection(ctx)
	if err != nil || before != after {
		t.Errorf("verify: %s vs %s %v", before[:8], after[:8], err)
	}
	if err := s.ClearProjection(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyCurrentFiles(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := s.ProjectionFingerprint(ctx); again != before {
		t.Error("rebuild changed the projection")
	}
	inv, err := s.Inventory(ctx)
	if err != nil || len(inv) != 4 || inv[0].ContentLevel != "fingerprint" {
		t.Errorf("inventory: %d %v", len(inv), err)
	}
	if c, _ := s.CountPresentFiles(ctx, src); c != 4 {
		t.Errorf("count present %d", c)
	}

	// --- sources and passes ------------------------------------------------
	loc := location.Location{Kind: location.KindLocal, Provider: "apfs", Evidence: "test", Confidence: 0.9}
	if err := s.RegisterSource(ctx, src, "local-folder", t.TempDir(), loc); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCadence(ctx, src, 7*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.SourceCadence(ctx, src); c != 7*24*3600 {
		t.Errorf("cadence %d", c)
	}
	if srcs, _ := s.Sources(ctx); len(srcs) != 1 || srcs[0].Location.Provider != "apfs" || srcs[0].Location.Confidence != 0.9 {
		t.Errorf("sources %+v", srcs)
	}
	if roots, _ := s.SourceRoots(ctx); roots[src] == "" {
		t.Error("no root")
	}
	if err := s.SetCursor(ctx, src, "fw/main.c"); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Cursor(ctx, src); c != "fw/main.c" {
		t.Errorf("cursor %q", c)
	}
	p1, err := s.BeginPass(ctx, PassStart{SourceID: src, Connector: "local-folder", StartedAt: clock})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.BeginPass(ctx, PassStart{SourceID: src, Connector: "local-folder", StartedAt: clock.Add(time.Minute), Resumed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishPass(ctx, p2, PassResult{Status: PassPartial, Detail: "resumed", Scanned: 1, Unchanged: 3}); err != nil {
		t.Fatal(err)
	}
	passes, err := s.LatestPasses(ctx)
	if err != nil || len(passes) != 1 {
		t.Fatalf("passes %d %v", len(passes), err)
	}
	if p := passes[0]; p.PassID != p2 || p.Status != PassPartial || !p.Resumed || p.FinishedAt == nil || !p.StartedAt.Equal(clock.Add(time.Minute)) {
		t.Errorf("latest pass %+v", p)
	}
	var st1 string
	if err := s.db.queryRow(ctx, `SELECT status FROM scan_passes WHERE pass_id=$1`, p1).Scan(&st1); err != nil || st1 != PassInterrupted {
		t.Errorf("superseded pass status %q %v", st1, err)
	}
	states, err := s.SourceStates(ctx)
	if err != nil || len(states) != 1 || states[0].PassStatus != PassPartial || states[0].CadenceSeconds != 7*24*3600 || states[0].LatestObsID == "" {
		t.Errorf("source states %+v %v", states, err)
	}

	// --- identity ------------------------------------------------------------
	art := ArtifactRow{ArtifactID: "art_1", SourceID: src, GroupingKey: "bom.xlsx", MemberCount: 2, Confidence: 0.88}
	members := []ArtifactMemberRow{
		{ArtifactID: "art_1", SourceID: src, Locator: "bom.xlsx", LatestSeq: 1, IsCurrent: true, Rule: "r", Confidence: 0.88, Explanation: "e"},
		{ArtifactID: "art_1", SourceID: src, Locator: "bom (copy).xlsx", LatestSeq: 2, Rule: "r", Confidence: 0.88, Explanation: "e"},
	}
	// from is the arrival, to is the gone side, as the rename detector writes it.
	rel := RelationRow{RelationID: "rel_1", Type: "compare-set-with", FromSource: src, FromLocator: "bom.xlsx",
		ToSource: src, ToLocator: "bom (copy).xlsx", Precedence: "gyst_suggestion", ActorKind: "suggestion", ActorID: "t",
		Evidence: []string{files[1].ObsID, files[2].ObsID}, Confidence: 0.3, Explanation: "why"}
	if err := s.ApplyIdentityPlan(ctx, "ip_a", "suffix-as-identity", []ArtifactRow{art}, members, []RelationRow{rel}); err != nil {
		t.Fatal(err)
	}
	if v, p, _ := s.ActivePolicy(ctx); v != "ip_a" || p != "suffix-as-identity" {
		t.Errorf("active %s %s", v, p)
	}
	if err := s.ApplyIdentityPlan(ctx, "ip_b", "compare-set", []ArtifactRow{art}, members, nil); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := s.ActivePolicy(ctx); v != "ip_b" {
		t.Errorf("active after switch %s", v)
	}
	if fp2, _, _ := s.LogFingerprint(ctx); fp2 != fp1 {
		t.Error("applying a profile changed the log")
	}
	if ms, _ := s.ArtifactMembers(ctx, "ip_a"); len(ms) != 2 || !ms[0].IsCurrent && !ms[1].IsCurrent {
		t.Errorf("members %+v", ms)
	}
	if m, key, err := s.MembershipOf(ctx, "ip_a", src, "bom.xlsx"); err != nil || key != "bom.xlsx" || !m.IsCurrent || m.Confidence != 0.88 {
		t.Errorf("membership %+v %q %v", m, key, err)
	}
	if sib, _ := s.Siblings(ctx, "ip_a", "art_1", "bom.xlsx"); len(sib) != 1 || sib[0].Locator != "bom (copy).xlsx" {
		t.Errorf("siblings %+v", sib)
	}
	if amb, _ := s.AmbiguousArtifacts(ctx, "ip_a"); len(amb) != 1 || amb[0].MemberCount != 2 {
		t.Errorf("ambiguous %+v", amb)
	}
	if arts, _ := s.Artifacts(ctx, "ip_b"); len(arts) != 1 {
		t.Errorf("artifacts %+v", arts)
	}

	// --- relations ----------------------------------------------------------
	native := RelationRow{RelationID: "rel_c", Type: "contains", FromSource: src, FromLocator: src + "@abc",
		ToSource: src, ToLocator: "fw/main.c", Precedence: "source_native_marker", ActorKind: "connector", ActorID: "git",
		Evidence: []string{batch[6].ObservationID, files[3].ObsID}, Confidence: 1, Explanation: "commit touched it"}
	if err := s.InsertRelations(ctx, []RelationRow{native, native}); err != nil {
		t.Fatal(err)
	}
	all, err := s.AllRelations(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("relations %d %v", len(all), err)
	}
	if r := all[0]; r.Type != "compare-set-with" || len(r.Evidence) != 2 || r.PolicyVersion == nil || *r.PolicyVersion != "ip_a" || r.AssertedAt.IsZero() {
		t.Errorf("relation row %+v", r)
	}
	if rs, _ := s.RelationsOf(ctx, "ip_a", src, "bom.xlsx"); len(rs) != 1 {
		t.Errorf("relations of bom.xlsx %d", len(rs))
	}
	if rs, _ := s.RelationsOf(ctx, "ip_b", src, "fw/main.c"); len(rs) != 1 || rs[0].PolicyVersion != nil {
		t.Errorf("native relation under another policy %+v", rs)
	}
	bad := native
	bad.RelationID, bad.Evidence = "rel_bad", []string{}
	if err := s.InsertRelations(ctx, []RelationRow{bad}); err == nil {
		t.Error("a relation with no evidence was accepted")
	}

	// --- commits --------------------------------------------------------------
	cobs, err := s.CommitObservations(ctx)
	if err != nil || len(cobs) != 1 || cobs[0].OID != "abc" || cobs[0].Payload["author"] != "a" {
		t.Fatalf("commit observations %+v %v", cobs, err)
	}
	if err := s.WriteCommits(ctx, []CommitRow{{SourceID: src, OID: "abc", Seq: cobs[0].Seq, ObsID: cobs[0].ObsID,
		Author: "a", Message: "m", AuthoredAt: "2026-09-20T11:00:00Z", Parents: []string{}, ChangedPaths: []string{"fw/main.c"}}}, nil); err != nil {
		t.Fatal(err)
	}
	if rc, _ := s.RecentCommits(ctx, src, 5); len(rc) != 1 || rc[0].Files != 1 || rc[0].Author != "a" {
		t.Errorf("recent commits %+v", rc)
	}
	if h, _ := s.GitHistoryOf(ctx, src, "fw/main.c"); len(h) != 1 || h[0].OID != "abc" || len(h[0].Evidence) != 2 {
		t.Errorf("history %+v", h)
	}

	// --- tombstones, arrivals -------------------------------------------------
	var seq2 int64
	if rows, _ := s.ObservationsOf(ctx, src, "bom (copy).xlsx"); len(rows) == 1 {
		seq2 = rows[0].Seq
	}
	tomb := observe.Tombstone(src, "bom (copy).xlsx", observe.KnownState{NativeVersion: "1:10", Seq: seq2},
		observe.Policy{ContentLevel: "fingerprint", Egress: "device", EffectivePolicyVersion: "p1"}, clock.Add(time.Hour))
	if _, err := s.Append(ctx, []observe.Observation{tomb}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyCurrentFiles(ctx); err != nil {
		t.Fatal(err)
	}
	gone, err := s.Tombstones(ctx)
	if err != nil || len(gone) != 1 || gone[0].Digest != d1 || gone[0].Size != 10 || gone[0].Pass == "" {
		t.Fatalf("tombstones %+v %v", gone, err)
	}
	arr, _ := s.Arrivals(ctx)
	if len(arr) != 4 || arr[0].Pass != gone[0].Pass && !strings.HasPrefix(arr[0].Pass, "2026-09-20T12:00:00") {
		t.Errorf("arrivals %d %q", len(arr), arr[0].Pass)
	}
	if fs, _ := s.PresentFiles(ctx); len(fs) != 3 {
		t.Errorf("present after tombstone %d", len(fs))
	}
	if amb, err := s.Ambiguities(ctx); err != nil || len(amb) != 1 || amb[0].GoneLocator != "bom (copy).xlsx" || amb[0].CandLocator != "bom.xlsx" || amb[0].CandDigest != d1 {
		t.Errorf("ambiguities %+v %v", amb, err)
	}

	// --- projects -------------------------------------------------------------
	if ms, err := s.LatestManifests(ctx); err != nil || len(ms) != 1 || jsonString(ms[0].Payload, "id") != "p" {
		t.Errorf("manifests %+v %v", ms, err)
	}
	if mk, err := s.LatestMarkers(ctx); err != nil || len(mk) != 1 || mk[0].Locator != "fw" || mk[0].Markers[0] != "git" {
		t.Errorf("markers %+v %v", mk, err)
	}
	if bad, _, _ := s.InvalidManifests(ctx); len(bad) != 0 {
		t.Errorf("invalid manifests %+v", bad)
	}
	if err := s.ReplaceProjects(ctx,
		[]ProjectRow{{ProjectID: "p", Name: "P", Basis: "manifest", SourceID: src, Locator: ".gyst/project.yaml", Evidence: []string{"obs_x"}, Confidence: 1, Explanation: "declared"}},
		[]ProjectMemberRow{{ProjectID: "p", SourceID: src, Pattern: "**", Basis: "manifest", Evidence: "obs_x", Confidence: 1}},
		[]FileProjectRow{{SourceID: src, Locator: "bom.xlsx", ProjectID: "p", Basis: "manifest", Pattern: "**", Confidence: 1},
			{SourceID: src, Locator: "fw/main.c", ProjectID: "p", Basis: "manifest", Pattern: "**", Confidence: 1}}); err != nil {
		t.Fatal(err)
	}
	sums, err := s.ProjectSummaries(ctx)
	if err != nil || len(sums) != 1 || sums[0].FileCount != 2 || len(sums[0].SourceIDs) != 1 || len(sums[0].Members) != 1 || sums[0].Evidence[0] != "obs_x" {
		t.Errorf("summaries %+v %v", sums, err)
	}
	if fps, names, _ := s.FileProjectsOf(ctx, src, "bom.xlsx"); len(fps) != 1 || names[0] != "P" {
		t.Errorf("file projects %+v %v", fps, names)
	}
	if all, _ := s.AllFileProjects(ctx); len(all) != 2 {
		t.Errorf("all file projects %d", len(all))
	}
	if err := s.ReplaceProjects(ctx, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.AllFileProjects(ctx); len(all) != 0 {
		t.Error("cascade did not clear file_projects")
	}

	// --- findings ---------------------------------------------------------------
	row := FindingRow{FindingID: "fnd_1", RuleID: "r", RuleVersion: "1", Severity: "low",
		Subjects: []byte(`[{"kind":"file","location":{"source_id":"src","locator":"bom.xlsx","native_version":{"scheme":"mtime_size","value":"1:10"}}}]`),
		Evidence: []string{files[1].ObsID}, Confidence: 1, Summary: "dup", Remediation: []byte(`{"proposal":"x","reversible":true,"requires_approval":true}`)}
	prior, err := s.UpsertFinding(ctx, row, clock)
	if err != nil || prior != nil {
		t.Fatalf("new finding prior=%v %v", prior, err)
	}
	if prior, err := s.UpsertFinding(ctx, row, clock.Add(time.Minute)); err != nil || prior == nil || *prior != "open" {
		t.Errorf("re-detected prior %v %v", deref(prior), err)
	}
	if n, _ := s.ResolveUnseenFindings(ctx, clock.Add(2*time.Minute)); n != 1 {
		t.Errorf("resolved %d", n)
	}
	if prior, err := s.UpsertFinding(ctx, row, clock.Add(3*time.Minute)); err != nil || prior == nil || *prior != "resolved" {
		t.Errorf("reopen prior %v %v", deref(prior), err)
	}
	if ok, _ := s.AcknowledgeFinding(ctx, "fnd_1", "d"); !ok {
		t.Error("ack failed")
	}
	if ok, _ := s.WaiveFinding(ctx, "fnd_1", "d", []byte(`{"actor":{"kind":"user","id":"d"},"reason":"r","waived_at":"2026-09-20T12:00:00Z","expires_at":"2026-09-20T12:10:00Z"}`)); !ok {
		t.Error("waive failed")
	}
	if prior, err := s.UpsertFinding(ctx, row, clock.Add(5*time.Minute)); err != nil || prior == nil || *prior != "waived" {
		t.Errorf("waived prior %q %v", deref(prior), err)
	}
	if l, _ := s.ListFindings(ctx, false); len(l) != 0 {
		t.Errorf("waived finding listed as open: %d", len(l))
	}
	if _, err := s.UpsertFinding(ctx, row, clock.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	l, _ := s.ListFindings(ctx, true)
	if len(l) != 1 || l[0].Status != "open" || l[0].Waiver == nil || len(l[0].Evidence) != 1 || !l[0].DetectedAt.Equal(clock) {
		t.Errorf("expired waiver did not reopen, or fields lost: %+v", l[0])
	}
	if c, _ := s.CountOpenFindings(ctx, src); c != 1 {
		t.Errorf("open for source %d", c)
	}
	if ff, _ := s.FindingsForFile(ctx, src, "bom.xlsx"); len(ff) != 1 {
		t.Errorf("findings for file %d", len(ff))
	}
	if ff, _ := s.FindingsForFile(ctx, src, "other"); len(ff) != 0 {
		t.Errorf("findings for other file %d", len(ff))
	}

	// --- assertions and authority -------------------------------------------------
	a := AssertionRow{AssertionID: "ast_1", Kind: "authority", SourceID: src, Locator: "bom.xlsx", ActorID: "d", Reason: "r",
		Evidence: []string{files[1].ObsID}, AssertedAt: clock}
	if err := s.InsertAssertion(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.exec(ctx, `DELETE FROM assertions WHERE assertion_id='ast_1'`); err == nil {
		t.Error("assertion deleted; they are durable")
	}
	if _, err := s.db.exec(ctx, `UPDATE assertions SET reason='edited' WHERE assertion_id='ast_1'`); err == nil {
		t.Error("assertion edited; only a retraction may be recorded")
	}
	if ok, err := s.RetractAssertion(ctx, "ast_1", "d", "changed my mind"); !ok || err != nil {
		t.Errorf("retract %v %v", ok, err)
	}
	if ok, _ := s.RetractAssertion(ctx, "ast_1", "d", "again"); ok {
		t.Error("retracted twice")
	}
	if l, _ := s.ListAssertions(ctx, false); len(l) != 0 {
		t.Error("retracted assertion still active")
	}
	if l, _ := s.ListAssertions(ctx, true); len(l) != 1 || l[0].RetractedAt == nil || *l[0].RetractedBy != "d" || !l[0].AssertedAt.Equal(clock) {
		t.Errorf("assertion history %+v", l)
	}
	of := "bom.xlsx"
	if err := s.ReplaceFileAuthority(ctx, []FileAuthorityRow{{SourceID: src, Locator: "bom.xlsx", State: "declared", Basis: "explicit",
		AuthoritySource: ptr(src), AuthorityLocator: &of, Confidence: 1, Evidence: []string{"obs_x"}, Explanation: "e"}}); err != nil {
		t.Fatal(err)
	}
	if r, err := s.FileAuthority(ctx, src, "bom.xlsx"); err != nil || r.State != "declared" || *r.AuthorityLocator != "bom.xlsx" || len(r.Evidence) != 1 {
		t.Errorf("authority %+v %v", r, err)
	}
	if _, err := s.FileAuthority(ctx, src, "nope"); err != ErrNotFound {
		t.Errorf("missing authority: %v", err)
	}
	if all, _ := s.AllFileAuthority(ctx); len(all) != 1 {
		t.Errorf("all authority %d", len(all))
	}
	if c, _ := s.Count(ctx); c != int64(len(batch)+1) {
		t.Errorf("count %d", c)
	}
}

func ptr(s string) *string { return &s }

// Bulk writes are chunked multi-row statements. Twenty thousand
// observations exceed one chunk on either engine, and two observations of
// one locator inside a single fold batch must leave the newer one.
func TestBulkWritesChunkAndDeduplicate(t *testing.T) {
	for name, s := range engines(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var batch []observe.Observation
			for i := 0; i < 20000; i++ {
				batch = append(batch, obs("file", "bulk", fmt.Sprintf("d/%05d.bin", i), "file.content_fingerprint",
					strings.Repeat("c", 64), int64(i%50+1), "fingerprint", nil, clock))
			}
			// The same locator twice in one batch: first with size 1, then a
			// later observation with size 2. The fold must end at size 2.
			batch = append(batch, obs("file", "bulk", "twice.bin", "file.content_fingerprint", strings.Repeat("d", 64), 1, "fingerprint", nil, clock))
			batch = append(batch, obs("file", "bulk", "twice.bin", "file.content_fingerprint", strings.Repeat("e", 64), 2, "fingerprint", nil, clock.Add(time.Second)))
			n, err := s.Append(ctx, batch)
			if err != nil || n != len(batch) {
				t.Fatalf("append %d %v", n, err)
			}
			if n, _ := s.Append(ctx, batch[:100]); n != 0 {
				t.Errorf("re-append inserted %d", n)
			}
			st, err := s.ApplyCurrentFiles(ctx)
			if err != nil || st.Applied != len(batch) {
				t.Fatalf("apply %+v %v", st, err)
			}
			if c, _ := s.CountPresentFiles(ctx, "bulk"); c != 20001 {
				t.Errorf("present %d", c)
			}
			if f, err := s.PresentFileAt(ctx, "bulk", "twice.bin"); err != nil || f.Size != 2 || !strings.HasPrefix(f.Digest, "e") {
				t.Errorf("newest within a batch did not win: %+v %v", f, err)
			}
			before, after, _, err := s.VerifyProjection(ctx)
			if err != nil || before != after {
				t.Errorf("verify %v", err)
			}
			rels := make([]RelationRow, 0, 5000)
			for i := 0; i < 5000; i++ {
				rels = append(rels, RelationRow{RelationID: fmt.Sprintf("rel_%d", i), Type: "duplicate-of",
					FromSource: "bulk", FromLocator: fmt.Sprintf("d/%05d.bin", i), ToSource: "bulk", ToLocator: "d/00000.bin",
					Precedence: "gyst_suggestion", ActorKind: "suggestion", ActorID: "t", Evidence: []string{"obs_x"}, Confidence: 0.5, Explanation: "e"})
			}
			rels = append(rels, rels[0]) // duplicate id in one call
			if err := s.InsertRelations(ctx, rels); err != nil {
				t.Fatal(err)
			}
			if all, _ := s.AllRelations(ctx); len(all) != 5000 {
				t.Errorf("relations %d", len(all))
			}
		})
	}
}

// Bookkeeping retention touches no evidence: passes beyond the newest few
// plus the first go, resolved findings past the cutoff go unless waived.
func TestBookkeepingRetention(t *testing.T) {
	for name, s := range engines(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			for i := 0; i < 40; i++ {
				id, err := s.BeginPass(ctx, PassStart{SourceID: "ret", Connector: "local-folder", StartedAt: clock.Add(time.Duration(i) * time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.FinishPass(ctx, id, PassResult{Status: PassComplete}); err != nil {
					t.Fatal(err)
				}
			}
			n, err := s.PruneScanPasses(ctx, "ret", 5)
			if err != nil || n != 34 {
				t.Fatalf("pruned %d %v; want 34 (40 minus the newest 5 minus the first)", n, err)
			}
			var oldest, newest time.Time
			if err := s.db.queryRow(ctx, `SELECT min(started_at), max(started_at) FROM scan_passes WHERE source_id='ret'`).Scan(ts(&oldest), ts(&newest)); err != nil {
				t.Fatal(err)
			}
			if !oldest.Equal(clock) || !newest.Equal(clock.Add(39*time.Minute)) {
				t.Errorf("kept %v .. %v; the first and the newest must survive", oldest, newest)
			}
			if n, _ := s.PruneScanPasses(ctx, "ret", 5); n != 0 {
				t.Errorf("second prune removed %d", n)
			}

			mk := func(id string) FindingRow {
				return FindingRow{FindingID: id, RuleID: "r", RuleVersion: "1", Severity: "low",
					Subjects: []byte(`[{"kind":"file","location":{"source_id":"ret","locator":"x","native_version":{"scheme":"mtime_size","value":"1:1"}}}]`),
					Evidence: []string{"obs_x"}, Confidence: 1, Summary: "s"}
			}
			for _, id := range []string{"fnd_old", "fnd_old_waived", "fnd_recent", "fnd_open"} {
				if _, err := s.UpsertFinding(ctx, mk(id), clock); err != nil {
					t.Fatal(err)
				}
			}
			if ok, _ := s.WaiveFinding(ctx, "fnd_old_waived", "d", []byte(`{"actor":{"kind":"user","id":"d"},"reason":"r","waived_at":"2026-09-20T12:00:00Z"}`)); !ok {
				t.Fatal("waive")
			}
			// Re-detect only fnd_open so the other three resolve.
			if _, err := s.UpsertFinding(ctx, mk("fnd_open"), clock.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if n, _ := s.ResolveUnseenFindings(ctx, clock.Add(time.Hour)); n != 3 {
				t.Fatalf("resolved %d", n)
			}
			// fnd_recent: pretend it resolved later than the cutoff.
			if _, err := s.db.exec(ctx, `UPDATE findings SET resolved_at=$1 WHERE finding_id='fnd_recent'`, clock.Add(200*time.Hour)); err != nil {
				t.Fatal(err)
			}
			n, err = s.PruneResolvedFindings(ctx, clock.Add(100*time.Hour))
			if err != nil || n != 1 {
				t.Fatalf("pruned %d findings %v; want only fnd_old", n, err)
			}
			left, _ := s.ListFindings(ctx, true)
			ids := map[string]bool{}
			for _, r := range left {
				ids[r.FindingID] = true
			}
			if ids["fnd_old"] || !ids["fnd_old_waived"] || !ids["fnd_recent"] || !ids["fnd_open"] {
				t.Errorf("remaining %v", ids)
			}
		})
	}
}
