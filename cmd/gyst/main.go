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

	"github.com/DazzlingDukeOfLazers/gyst/internal/connector/localfolder"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const usage = `gyst -- find, understand, and coordinate engineering work products

Usage:
  gyst scan    --root <path> [--source <id>] [--policy fingerprint|metadata] [--resume]
  gyst changes [--since 1h] [--limit 50]
  gyst project [--rebuild]
  gyst verify
  gyst status

Environment:
  GYST_DATABASE_URL   default postgres:///gyst
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
	fs.Parse(args)

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

	// What the projection already believes. Unchanged files are skipped before
	// they are read, so an incremental scan does not re-hash a settled tree.
	known, err := s.KnownState(ctx, sourceID)
	if err != nil {
		return err
	}

	start := time.Now()
	res, err := localfolder.Discover(localfolder.Options{
		Known:         known,
		Root:          *root,
		SourceID:      sourceID,
		ContentLevel:  *policy,
		Egress:        "device",
		PolicyVersion: "pol_dev_r1",
		Cursor:        cursor,
		MaxFiles:      *maxFiles,
	})
	if err != nil {
		return err
	}
	walked := time.Since(start)

	inserted, err := s.Append(ctx, res.Observations)
	if err != nil {
		return err
	}
	if res.NextCursor != "" {
		if err := s.SetCursor(ctx, sourceID, res.NextCursor); err != nil {
			return err
		}
	}
	elapsed := time.Since(start)

	stats, err := project.Apply(ctx, s)
	if err != nil {
		return err
	}

	fmt.Printf("source     %s\n", sourceID)
	fmt.Printf("seen       %d files, %s\n", res.Scanned+res.Unchanged, humanBytes(res.Bytes))
	fmt.Printf("unchanged  %d (not read)   changed %d, %s hashed\n",
		res.Unchanged, res.Scanned, humanBytes(res.HashedBytes))
	fmt.Printf("ignored    %d   skipped %d   unstable %d\n", res.Ignored, res.Skipped, res.Unstable)
	fmt.Printf("appended   %d new observations (%d already known)\n",
		inserted, len(res.Observations)-inserted)
	fmt.Printf("projected  %d rows applied (seq %d -> %d)\n", stats.Applied, stats.FromSeq, stats.ToSeq)
	fmt.Printf("elapsed    %s walk, %s total", walked.Round(time.Millisecond), elapsed.Round(time.Millisecond))
	if res.HashedBytes > 0 {
		fmt.Printf("   (%s/s hashed)", humanBytes(int64(float64(res.HashedBytes)/elapsed.Seconds())))
	}
	fmt.Println()
	if !res.Complete {
		fmt.Printf("partial    stopped at --max-files; rerun with --resume\n")
	}
	return nil
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
	rows, err := s.Pool().Query(ctx, `
		SELECT o.observed_at, o.claim_type, o.source_id, o.locator,
		       coalesce(o.content_digest_hex,'-'), coalesce(o.size_bytes,0)
		FROM observations o
		WHERE o.observed_at >= $1
		ORDER BY o.seq DESC LIMIT $2`, cutoff, *limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WHEN\tCLAIM\tSIZE\tDIGEST\tLOCATOR")
	n := 0
	for rows.Next() {
		var at time.Time
		var claim, src, loc, digest string
		var size int64
		if err := rows.Scan(&at, &claim, &src, &loc, &digest, &size); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			at.Local().Format("15:04:05"), short(claim, 24), humanBytes(size), digest[:min(8, len(digest))], loc)
		n++
	}
	w.Flush()
	if n == 0 {
		fmt.Printf("no observations in the last %s\n", *since)
	}
	return rows.Err()
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
		if _, err := s.Pool().Exec(ctx, `DELETE FROM current_files`); err != nil {
			return err
		}
		if _, err := s.Pool().Exec(ctx,
			`UPDATE projector_state SET last_seq=0 WHERE projector=$1`, project.Name); err != nil {
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

	cur, err := s.Pool().Query(ctx, `SELECT source_id, cursor FROM source_cursors ORDER BY source_id`)
	if err != nil {
		return err
	}
	defer cur.Close()
	for cur.Next() {
		var src, c string
		if err := cur.Scan(&src, &c); err != nil {
			return err
		}
		fmt.Printf("cursor       %s -> %s\n", src, c)
	}
	return cur.Err()
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
