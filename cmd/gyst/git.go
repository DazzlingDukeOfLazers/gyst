package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	gitconn "github.com/DazzlingDukeOfLazers/gyst/internal/connector/git"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
)

func cmdGit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("git", flag.ExitOnError)
	repo := fs.String("repo", "", "path to a Git repository (required)")
	source := fs.String("source", "", "source id (defaults to src_git_<basename>)")
	ref := fs.String("ref", "HEAD", "ref to walk")
	resume := fs.Bool("resume", false, "resume from the stored cursor")
	maxCommits := fs.Int("max-commits", 0, "stop after N commits (0 = default cap)")
	fs.Parse(args)

	if *repo == "" {
		return fmt.Errorf("--repo is required")
	}
	if !gitconn.IsRepo(*repo) {
		return fmt.Errorf("%s is not a Git work tree", *repo)
	}
	sourceID := *source
	if sourceID == "" {
		sourceID = "src_git_" + sanitize(*repo)
	}

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.RegisterSource(ctx, sourceID, "git", *repo); err != nil {
		return err
	}

	cursor := ""
	if *resume {
		if cursor, err = s.Cursor(ctx, sourceID); err != nil {
			return err
		}
	}

	res, err := gitconn.Discover(gitconn.Options{
		Repo:          *repo,
		SourceID:      sourceID,
		Ref:           *ref,
		Cursor:        cursor,
		PolicyVersion: "pol_dev_r1",
		MaxCommits:    *maxCommits,
	})
	if err != nil {
		return err
	}

	inserted, err := s.Append(ctx, res.Observations)
	if err != nil {
		return err
	}
	if res.NextCursor != "" {
		if err := s.SetCursor(ctx, sourceID, res.NextCursor); err != nil {
			return err
		}
	}
	if _, err := project.Apply(ctx, s); err != nil {
		return err
	}
	cs, err := project.ProjectCommits(ctx, s)
	if err != nil {
		return err
	}

	fmt.Printf("source     %s\n", sourceID)
	fmt.Printf("ref        %s at %s\n", res.Ref, firstN(res.Head, 12))
	fmt.Printf("commits    %d read\n", res.Commits)
	fmt.Printf("appended   %d new observations (%d already known)\n",
		inserted, len(res.Observations)-inserted)
	fmt.Printf("projected  %d commits, %d file touches\n", cs.Commits, cs.Files)
	fmt.Printf("reconciled %d touches matched a scanned file, %d unmatched\n", cs.Bridged, cs.Orphaned)

	rows, err := s.Pool().Query(ctx, `
		SELECT native_version_value, claim_payload->>'author', claim_payload->>'message',
		       jsonb_array_length(coalesce(claim_payload->'changed_paths','[]'::jsonb))
		FROM observations
		WHERE source_id=$1 AND claim_type='git.commit'
		ORDER BY seq DESC LIMIT 10`, sourceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nCOMMIT\tFILES\tAUTHOR\tMESSAGE")
	for rows.Next() {
		var oid, author, message string
		var n int
		if err := rows.Scan(&oid, &author, &message, &n); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", firstN(oid, 12), n, short(author, 28), short(message, 48))
	}
	w.Flush()
	return rows.Err()
}
