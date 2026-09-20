package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

const assertUsage = `usage:
  gyst assert authority     <locator> --by <name> --reason <text>
  gyst assert not-authority <locator> --by <name> --reason <text>
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
	if _, err := authority.Project(ctx, s); err != nil {
		return err
	}
	fmt.Printf("retracted %s by %s; the record remains\n", id, *by)
	return nil
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Kind, r.ActorID,
			r.AssertedAt.Local().Format("2006-01-02 15:04"), status, r.Subject.Locator, short(r.Reason, 40))
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
