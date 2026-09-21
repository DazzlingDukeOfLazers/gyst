package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const assertUsage = `usage:
  gyst assert authority     <locator> --by <name> --reason <text>
  gyst assert not-authority <locator> --by <name> --reason <text>
  gyst assert project confirm <project-id> --by <name> --reason <text>
  gyst assert project ignore  <project-id> --by <name> --reason <text>
  gyst assert retract       <assertion-id> --by <name> --reason <text>
  gyst assert list [--all]`

// cmdAssert records what a person says about a file. These are the top of
// the precedence order and nothing inferred may override them.
func cmdAssert(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", assertUsage)
	}
	switch args[0] {
	case "authority", "not-authority":
		return assertKind(ctx, args[0], args[1:])
	case "project":
		if len(args) < 2 || (args[1] != "confirm" && args[1] != "ignore") {
			return fmt.Errorf("%s", assertUsage)
		}
		return assertProject(ctx, "project."+args[1], args[2:])
	case "retract":
		return assertRetract(ctx, args[1:])
	case "list":
		return assertList(ctx, args[1:])
	}
	return fmt.Errorf("%s", assertUsage)
}

func assertKind(ctx context.Context, kind string, args []string) error {
	needle, rest := takeID(args)
	fs := flag.NewFlagSet("assert "+kind, flag.ExitOnError)
	by := fs.String("by", "", "who is asserting (required)")
	reason := fs.String("reason", "", "why (required)")
	fs.Parse(rest)
	if needle == "" || fs.NArg() != 0 {
		return fmt.Errorf("%s", assertUsage)
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	key, err := resolveLocator(ctx, s, needle)
	if err != nil {
		return err
	}
	id, err := authority.Assert(ctx, s, kind, key, *by, *reason)
	if err != nil {
		return err
	}
	st, err := authority.Project(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("recorded %s: %s is %s (by %s)\n", id, key.Locator, describeKind(kind), *by)
	fmt.Printf("authority  %d declared, %d likely, %d multiple candidates, %d none\n",
		st.Declared, st.Likely, st.Multiple, st.None)
	return nil
}

func describeKind(kind string) string {
	if kind == authority.KindNotAuthority {
		return "not the authority"
	}
	return "the authority"
}

func assertRetract(ctx context.Context, args []string) error {
	id, rest := takeID(args)
	fs := flag.NewFlagSet("assert retract", flag.ExitOnError)
	by := fs.String("by", "", "who is retracting (required)")
	reason := fs.String("reason", "", "why (required)")
	fs.Parse(rest)
	if id == "" || fs.NArg() != 0 {
		return fmt.Errorf("%s", assertUsage)
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := authority.Retract(ctx, s, id, *by, *reason); err != nil {
		return err
	}
	st, err := rebuildJudgments(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("retracted %s by %s; the record remains\n", id, *by)
	fmt.Printf("review     %d confirmed, %d ignored, %d remaining; the lower-precedence evidence stands again\n",
		st.Confirmed, st.Ignored, st.Remaining)
	fmt.Printf("on disk    nothing renamed, moved, or deleted\n")
	return nil
}

// assertProject records a person's judgment about a candidate and shows
// what changed. It refuses to judge a record twice: the existing decision
// must be retracted first, so every change is visible in the history.
func assertProject(ctx context.Context, kind string, args []string) error {
	id, rest := takeID(args)
	fs := flag.NewFlagSet("assert "+kind, flag.ExitOnError)
	by := fs.String("by", "", "who is deciding (required)")
	reason := fs.String("reason", "", "why (required)")
	fs.Parse(rest)
	if id == "" || fs.NArg() != 0 || *by == "" || *reason == "" {
		return fmt.Errorf("%s", assertUsage)
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	sums, err := s.ProjectSummaries(ctx)
	if err != nil {
		return err
	}
	var rec *store.ProjectSummary
	for i := range sums {
		if sums[i].ProjectID == id || sums[i].Name == id {
			if rec != nil {
				return fmt.Errorf("%q names more than one record; use the project id", id)
			}
			rec = &sums[i]
		}
	}
	if rec == nil {
		return fmt.Errorf("no project record %q", id)
	}
	switch rec.State {
	case "declared":
		return fmt.Errorf("%s is declared by a manifest; judge candidates, or change the manifest", rec.Name)
	case "confirmed", "ignored":
		return fmt.Errorf("%s is already %s; retract that assertion first (gyst assert list)", rec.Name, rec.State)
	}
	evidence := rec.Evidence
	if len(evidence) == 0 {
		return fmt.Errorf("%s cites no evidence; refusing to assert about it", rec.Name)
	}
	now := time.Now().UTC()
	h := sha256.Sum256([]byte(kind + "\x00" + rec.ProjectID + "\x00" + *by + "\x00" + now.Format(time.RFC3339Nano)))
	astID := "ast_" + hex.EncodeToString(h[:])[:24]
	if err := s.InsertAssertion(ctx, store.AssertionRow{
		AssertionID: astID, Kind: kind, SubjectKind: "project", SourceID: "", Locator: rec.ProjectID,
		ActorID: *by, Reason: *reason, Evidence: evidence, AssertedAt: now,
	}); err != nil {
		return err
	}
	st, err := rebuildJudgments(ctx, s)
	if err != nil {
		return err
	}
	verb := "confirmed as a project"
	if kind == "project.ignore" {
		verb = "set aside: not a project"
	}
	fmt.Printf("recorded   %s: %s %s (by %s)\n", astID, rec.Name, verb, *by)
	fmt.Printf("review     %d confirmed, %d ignored, %d remaining\n", st.Confirmed, st.Ignored, st.Remaining)
	fmt.Printf("on disk    nothing renamed, moved, or deleted\n")
	fmt.Printf("undo       gyst assert retract %s --by <you> --reason \"...\"\n", astID)
	return nil
}

// rebuildJudgments reruns the projections a judgment touches, in the
// order a scan runs them.
func rebuildJudgments(ctx context.Context, s *store.Store) (project.MembershipStats, error) {
	st, err := project.ProjectMembership(ctx, s)
	if err != nil {
		return st, err
	}
	if _, err := findings.Project(ctx, s, time.Now().UTC()); err != nil {
		return st, err
	}
	_, err = authority.Project(ctx, s)
	return st, err
}

func assertList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("assert list", flag.ExitOnError)
	all := fs.Bool("all", false, "include retracted assertions")
	fs.Parse(args)
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	rows, err := authority.List(ctx, s, *all)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("no assertions")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ASSERTION\tKIND\tWHO\tWHEN\tSTATUS\tSUBJECT\tREASON")
	for _, r := range rows {
		status := "active"
		if r.RetractedAt != nil {
			status = "retracted by " + *r.RetractedBy
		}
		subject := r.Subject.Locator
		if r.SubjectKind == "project" {
			subject = "project " + r.Subject.Locator
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.ActorID,
			r.AssertedAt.Local().Format("2006-01-02 15:04"), status, subject, short(r.Reason, 40))
	}
	w.Flush()
	return nil
}

// resolveLocator finds one present file by exact locator or suffix, the
// way explain does, so a person can paste a bare filename.
func resolveLocator(ctx context.Context, s *store.Store, needle string) (authority.Key, error) {
	f, err := s.FindPresentFile(ctx, needle)
	if err != nil {
		return authority.Key{}, fmt.Errorf("no current file matching %q", needle)
	}
	return authority.Key{SourceID: f.SourceID, Locator: f.Locator}, nil
}
