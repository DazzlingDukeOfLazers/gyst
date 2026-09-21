// Command gyst is the day 2 walking skeleton: scan a root into an append-only
// log, project it, and query the result.
//
// Every command here is read-only with respect to the user's files. The only
// thing gyst writes is its own database.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/bundle"
	"github.com/DazzlingDukeOfLazers/gyst/internal/connector/localfolder"
	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// Bookkeeping retention defaults (ADR 005): passes kept per source beyond
// the first, and how long a resolved finding without a waiver is kept.
const (
	keepPasses         = 30
	resolvedFindingAge = 90 * 24 * time.Hour
)

const usage = `gyst -- find, understand, and coordinate engineering work products

Usage:
  gyst scan    --root <path> [--source <id>] [--policy fingerprint|metadata] [--resume] [--cadence 7d] [--egress device|facility|connected-server]
  gyst changes [--since 1h] [--limit 50]
  gyst project [--rebuild]
  gyst verify
  gyst status
  gyst git     --repo <path> [--ref HEAD] [--resume]
  gyst discover [--root <path>]... [--depth 6] [--nested] [--json]
  gyst projects                               projects and where membership comes from
  gyst review [<candidate>]                   what still needs your judgment, and one candidate explained
  gyst findings [--all] [--json]              what needs attention
  gyst report [--out report.json] [--html report.html]   everything, as one document
  gyst assert authority     <locator> --by <name> --reason <text>
  gyst assert not-authority <locator> --by <name> --reason <text>
  gyst assert retract <id>  --by <name> --reason <text>
  gyst assert list [--all]
  gyst key init <id> | gyst key show <id>     a signing identity for bundles
  gyst trust add <sender> <public-key> --by <name> | gyst trust list
  gyst export --key <id> --egress facility [--source <id>]... --out bundle.jsonl
  gyst import bundle.jsonl [--trust-on-first-use --by <name>]
  gyst findings ack   <id> --by <name>
  gyst findings waive <id> --by <name> --reason <text> [--until YYYY-MM-DD]

  gyst identity preview --profile <profile>   show grouping without writing it
  gyst identity apply   --profile <profile>   activate a grouping
  gyst identity verify                        every profile, evidence unchanged
  gyst identity status                        active policy and its ambiguous groups

  gyst explain <locator>                      provenance for one file

Profiles:
  content-path-exact  suffix-as-version  suffix-as-identity
  canonical-name      compare-set

Environment:
  GYST_DATABASE_URL   where the store lives. Unset: a SQLite file in the user's
                      data directory, nothing else needed. postgres://... for a
                      shared server; sqlite:<path> for a file elsewhere.
  GYST_DATA_DIR       overrides the data directory.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdScan(ctx, os.Args[2:])
	case "changes":
		err = cmdChanges(ctx, os.Args[2:])
	case "project":
		err = cmdProject(ctx, os.Args[2:])
	case "verify":
		err = cmdVerify(ctx)
	case "git":
		err = cmdGit(ctx, os.Args[2:])
	case "discover":
		err = cmdDiscover(ctx, os.Args[2:])
	case "projects":
		err = cmdProjects(ctx, os.Args[2:])
	case "review":
		err = cmdReview(ctx, os.Args[2:])
	case "findings":
		err = cmdFindings(ctx, os.Args[2:])
	case "report":
		err = cmdReport(ctx, os.Args[2:])
	case "assert":
		err = cmdAssert(ctx, os.Args[2:])
	case "key":
		err = cmdKey(ctx, os.Args[2:])
	case "trust":
		err = cmdTrust(ctx, os.Args[2:])
	case "export":
		err = cmdExport(ctx, os.Args[2:])
	case "import":
		err = cmdImport(ctx, os.Args[2:])
	case "identity":
		err = cmdIdentity(ctx, os.Args[2:])
	case "explain":
		err = cmdExplain(ctx, os.Args[2:])
	case "status":
		err = cmdStatus(ctx)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func open(ctx context.Context) (*store.Store, error) { return store.Open(ctx) }

func cmdScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	root := fs.String("root", "", "directory to scan (required)")
	source := fs.String("source", "", "source id (defaults to the root's base name)")
	policy := fs.String("policy", "fingerprint", "content policy: fingerprint or metadata")
	resume := fs.Bool("resume", false, "resume from the stored cursor instead of a full pass")
	maxFiles := fs.Int("max-files", 0, "stop after N files (0 = no limit)")
	cadence := fs.String("cadence", "", "how often this source is expected to be scanned, e.g. 7d or 12h")
	egress := fs.String("egress", "device", "how far these observations may travel: device, facility, or connected-server")
	fs.Parse(args)
	if !bundle.ValidEgress(*egress) {
		return fmt.Errorf("--egress %q: one of device, facility, connected-server", *egress)
	}

	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	sourceID := *source
	if sourceID == "" {
		sourceID = "src_" + sanitize(*root)
	}

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	cursor := ""
	if *resume {
		if cursor, err = s.Cursor(ctx, sourceID); err != nil {
			return err
		}
	}

	// Where the root lives decides how it is governed, so it is recorded
	// with the source and printed with every scan.
	loc := location.Probe(*root)
	if err := s.RegisterSource(ctx, sourceID, "local-folder", *root, loc); err != nil {
		return err
	}
	if *cadence != "" {
		d, err := findings.ParseCadence(*cadence)
		if err != nil {
			return err
		}
		if err := s.SetCadence(ctx, sourceID, d); err != nil {
			return err
		}
	}

	// One clock reading for the whole pass. It is the pass's started_at and
	// the observed_at of everything the pass produces, so the pass row and
	// the log agree about when this happened by construction.
	started := time.Now().UTC()
	passID, err := s.BeginPass(ctx, store.PassStart{
		SourceID: sourceID, Connector: localfolder.ConnectorName,
		StartedAt: started, Resumed: *resume,
	})
	if err != nil {
		return err
	}

	// What the projection already believes. Unchanged files are skipped before
	// they are read, so an incremental scan does not re-hash a settled tree.
	known, err := s.KnownState(ctx, sourceID)
	if err != nil {
		return err
	}

	res, err := localfolder.Discover(localfolder.Options{
		Known:         known,
		Root:          *root,
		SourceID:      sourceID,
		ContentLevel:  *policy,
		Egress:        *egress,
		PolicyVersion: "pol_dev_r1",
		Cursor:        cursor,
		MaxFiles:      *maxFiles,
		Now:           started,
	})
	if localfolder.IsUnavailable(err) {
		// Not a failure of the scan: a true statement about the source. It
		// is recorded as such and nothing else moves -- no observations, no
		// cursor, no tombstones.
		if ferr := s.FinishPass(ctx, passID, store.PassResult{
			Status: store.PassUnavailable, Detail: err.Error(),
			AbsenceReason: "root was unavailable; nothing was observed",
		}); ferr != nil {
			return ferr
		}
		fmt.Printf("source       %s\n", sourceID)
		fmt.Printf("unavailable  %v\n", err)
		fmt.Printf("recorded     pass %s; no observations, cursor unchanged\n", passID)
		return nil
	}
	if err != nil {
		return finishInterrupted(ctx, s, passID, err)
	}
	walked := time.Since(started)

	// Deletion detection: anything known but not seen by a complete pass.
	opts := localfolder.Options{Cursor: cursor, SourceID: sourceID,
		ContentLevel: *policy, Egress: *egress, PolicyVersion: "pol_dev_r1"}
	tomb := localfolder.Tombstones(res, opts, known)
	res.Observations = append(res.Observations, tomb.Tombstones...)

	inserted, err := s.Append(ctx, res.Observations)
	if err != nil {
		return finishInterrupted(ctx, s, passID, err)
	}
	if res.NextCursor != "" {
		if err := s.SetCursor(ctx, sourceID, res.NextCursor); err != nil {
			return finishInterrupted(ctx, s, passID, err)
		}
	}

	cov := res.Coverage(*resume)
	if err := s.FinishPass(ctx, passID, store.PassResult{
		Status: cov.Status, Detail: cov.Detail,
		Scanned: res.Scanned, Unchanged: res.Unchanged, Skipped: res.Skipped,
		Ignored: res.Ignored, Unstable: res.Unstable,
		Bytes: res.Bytes, HashedBytes: res.HashedBytes, Appended: inserted,
		AbsenceChecked: tomb.Eligible, AbsenceReason: tomb.Reason,
	}); err != nil {
		return err
	}
	elapsed := time.Since(started)

	stats, err := project.Apply(ctx, s)
	if err != nil {
		return err
	}
	renames, err := project.ProjectRenames(ctx, s)
	if err != nil {
		return err
	}
	members, err := project.ProjectMembership(ctx, s)
	if err != nil {
		return err
	}
	fnd, err := findings.Project(ctx, s, time.Now().UTC())
	if err != nil {
		return err
	}
	auth, err := authority.Project(ctx, s)
	if err != nil {
		return err
	}
	// Bookkeeping retention: passes and resolved findings are about Gyst's
	// own activity, not the sources, and are bounded here. Nothing a
	// finding, relation, or release cites is touched (ADR 005).
	prunedPasses, err := s.PruneScanPasses(ctx, sourceID, keepPasses)
	if err != nil {
		return err
	}
	prunedFindings, err := s.PruneResolvedFindings(ctx, time.Now().UTC().Add(-resolvedFindingAge))
	if err != nil {
		return err
	}

	fmt.Printf("source     %s\n", sourceID)
	fmt.Printf("location   %s   %s\n", loc, loc.Evidence)
	fmt.Printf("seen       %d files, %s\n", res.Scanned+res.Unchanged, humanBytes(res.Bytes))
	fmt.Printf("unchanged  %d (not read)   changed %d, %s hashed\n",
		res.Unchanged, res.Scanned, humanBytes(res.HashedBytes))
	fmt.Printf("ignored    %d   skipped %d   unstable %d   symlinks %d (not followed)\n", res.Ignored, res.Skipped, res.Unstable, res.Symlinks)
	if res.Placeholders > 0 {
		fmt.Printf("placeholder %d file(s) whose content is held by a sync engine; observed by metadata, not read\n",
			res.Placeholders)
	}
	if tomb.Eligible {
		fmt.Printf("absent     %d file(s) gone", len(tomb.Tombstones))
		if tomb.Suppressed > 0 {
			fmt.Printf("   (%d newly ignored, not tombstoned)", tomb.Suppressed)
		}
		fmt.Println()
	} else {
		fmt.Printf("absent     not checked: %s\n", tomb.Reason)
	}
	fmt.Printf("appended   %d new observations (%d already known)\n",
		inserted, len(res.Observations)-inserted)
	fmt.Printf("projected  %d rows applied (seq %d -> %d)\n", stats.Applied, stats.FromSeq, stats.ToSeq)
	if renames.Renamed > 0 || renames.Ambiguous > 0 {
		fmt.Printf("renames    %d detected, %d ambiguous (candidates offered), %d plain deletions\n",
			renames.Renamed, renames.Ambiguous, renames.Unmatched)
		if renames.SkippedNoContent > 0 {
			fmt.Printf("           %d skipped: no content to match on (empty files)\n",
				renames.SkippedNoContent)
		}
	}
	fmt.Printf("elapsed    %s walk, %s total", walked.Round(time.Millisecond), elapsed.Round(time.Millisecond))
	if res.HashedBytes > 0 {
		fmt.Printf("   (%s/s hashed)", humanBytes(int64(float64(res.HashedBytes)/elapsed.Seconds())))
	}
	fmt.Println()
	fmt.Printf("projects   %d (%d from manifests, %d from markers, %d marker(s) covered by a manifest); %d file memberships\n",
		members.Projects, members.Manifests-members.InvalidManifests,
		members.Projects-(members.Manifests-members.InvalidManifests),
		members.SuppressedMarkers, members.FileMemberships)
	if members.Remaining+members.Confirmed+members.Ignored > 0 {
		fmt.Printf("review     %d confirmed, %d ignored, %d candidates remaining (gyst review)\n",
			members.Confirmed, members.Ignored, members.Remaining)
	}
	if members.InvalidManifests > 0 {
		fmt.Printf("           %d manifest(s) could not be parsed; see gyst explain\n", members.InvalidManifests)
	}
	fmt.Printf("authority  %d declared, %d likely, %d multiple candidates, %d none\n",
		auth.Declared, auth.Likely, auth.Multiple, auth.None)
	fmt.Printf("findings   %d open (%d new, %d reopened, %d resolved this pass)\n",
		fnd.Open, fnd.New, fnd.Reopened, fnd.Resolved)
	if prunedPasses+prunedFindings > 0 {
		fmt.Printf("retention  pruned %d old passes and %d resolved findings older than %d days\n",
			prunedPasses, prunedFindings, int(resolvedFindingAge.Hours()/24))
	}
	fmt.Printf("coverage   %s: %s\n", cov.Status, cov.Detail)
	if !res.Complete {
		fmt.Printf("partial    stopped at --max-files; rerun with --resume\n")
	}
	return nil
}

// finishInterrupted records that a pass died mid-way, then returns the cause.
// A pass that errors after appending some observations has produced true
// observations and unknown coverage, which is exactly what interrupted means.
func finishInterrupted(ctx context.Context, s *store.Store, passID string, cause error) error {
	if ferr := s.FinishPass(ctx, passID, store.PassResult{
		Status: store.PassInterrupted, Detail: cause.Error(),
	}); ferr != nil {
		return fmt.Errorf("%w (and recording the interruption failed: %v)", cause, ferr)
	}
	return cause
}

func cmdChanges(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("changes", flag.ExitOnError)
	since := fs.Duration("since", time.Hour, "window, e.g. 1h, 30m, 24h")
	limit := fs.Int("limit", 50, "maximum rows")
	fs.Parse(args)

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	cutoff := time.Now().Add(-*since)
	rows, err := s.RecentObservations(ctx, cutoff, *limit)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tCLAIM\tSIZE\tDIGEST\tLOCATOR")
	for _, o := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			o.ObservedAt.Local().Format("15:04:05"), short(o.ClaimType, 24), humanBytes(o.Size),
			o.Digest[:min(8, len(o.Digest))], o.Locator)
	}
	w.Flush()
	if len(rows) == 0 {
		fmt.Printf("no observations in the last %s\n", *since)
	}
	return nil
}

func cmdProject(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("project", flag.ExitOnError)
	rebuild := fs.Bool("rebuild", false, "drop the projection and replay the whole log")
	fs.Parse(args)

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	if *rebuild {
		if err := s.ClearProjection(ctx); err != nil {
			return err
		}
		fmt.Println("projection cleared; replaying from seq 0")
	}
	stats, err := project.Apply(ctx, s)
	if err != nil {
		return err
	}
	fp, rows, err := project.Fingerprint(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("applied %d observations (seq %d -> %d)\n", stats.Applied, stats.FromSeq, stats.ToSeq)
	fmt.Printf("current_files: %d rows, fingerprint %s\n", rows, fp[:16])
	return nil
}

// cmdVerify is the day 2 exit criterion as a command: replay must reproduce an
// equivalent projection.
func cmdVerify(ctx context.Context) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	if _, err := project.Apply(ctx, s); err != nil {
		return err
	}
	before, after, rows, err := project.Verify(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("live projection    %s  (%d rows)\n", before[:32], rows)
	fmt.Printf("replayed from 0    %s\n", after[:32])
	if before != after {
		return fmt.Errorf("REPLAY DIVERGED: the projection is not reproducible from the log")
	}
	fmt.Println("\nreplay reproduces the projection exactly")
	return nil
}

func cmdStatus(ctx context.Context) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	n, err := s.Count(ctx)
	if err != nil {
		return err
	}
	fp, rows, err := project.Fingerprint(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("database     %s\n", store.DSN())
	fmt.Printf("observations %d\n", n)
	fmt.Printf("current_files %d rows, fingerprint %s\n", rows, fp[:16])

	passes, err := s.LatestPasses(ctx)
	if err != nil {
		return err
	}
	if len(passes) == 0 {
		fmt.Println("sources      none registered")
		return nil
	}
	srcs, err := s.Sources(ctx)
	if err != nil {
		return err
	}
	importedFrom := map[string]string{}
	for _, r := range srcs {
		importedFrom[r.SourceID] = r.ImportedFrom
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nSOURCE\tKIND\tLOCATION\tLAST PASS\tCOVERAGE\tDETAIL")
	for _, p := range passes {
		status, detail := passStatus(p), passDetail(p)
		if from := importedFrom[p.SourceID]; from != "" && p.Status == "" {
			// An imported source is never scanned here; its passes happened
			// at the sender. Saying "never scanned" would read as neglect.
			status, detail = "imported", "observations received from "+from+"; scanned there, not here"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.SourceID, p.Kind, p.Location, passAge(p), status, short(detail, 64))
	}
	w.Flush()
	return nil
}

func passAge(p store.Pass) string {
	if p.Status == "" {
		return "-"
	}
	return humanAgo(time.Since(p.StartedAt))
}

// passStatus is the coverage word a person reads first. "never scanned" is a
// state, not a missing row; a registered source nobody has looked at must not
// disappear from the table.
func passStatus(p store.Pass) string {
	switch p.Status {
	case "":
		return "never scanned"
	case store.PassRunning:
		return "running"
	default:
		return p.Status
	}
}

func passDetail(p store.Pass) string {
	switch p.Status {
	case "":
		return "registered, no pass recorded"
	case store.PassComplete, store.PassPartial:
		d := fmt.Sprintf("%d changed, %d unchanged", p.Scanned, p.Unchanged)
		if p.Connector == "git" {
			d = fmt.Sprintf("%d commits read", p.Scanned)
		}
		if p.Skipped > 0 {
			d += fmt.Sprintf(", %d unreadable", p.Skipped)
		}
		if p.Status == store.PassPartial {
			d += "; " + p.Detail
		}
		return d
	default:
		return p.Detail
	}
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func sanitize(s string) string {
	out := []rune{}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == '/' || r == ' ' || r == '-':
			if len(out) > 0 && out[len(out)-1] != '_' {
				out = append(out, '_')
			}
		}
	}
	if len(out) > 40 {
		out = out[len(out)-40:]
	}
	return string(out)
}

func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGT"[exp])
}
