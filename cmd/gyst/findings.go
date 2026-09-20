package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

func cmdFindings(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "ack":
			return cmdFindingsAck(ctx, args[1:])
		case "waive":
			return cmdFindingsWaive(ctx, args[1:])
		}
	}
	fs := flag.NewFlagSet("findings", flag.ExitOnError)
	all := fs.Bool("all", false, "include waived and resolved findings")
	asJSON := fs.Bool("json", false, "print findings as JSON in the v0 schema")
	fs.Parse(args)

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	// Detection is cheap and freshness depends on the clock, so the list is
	// always current rather than as of the last scan.
	if _, err := findings.Project(ctx, s, time.Now().UTC()); err != nil {
		return err
	}
	rows, err := findings.List(ctx, s, *all)
	if err != nil {
		return err
	}
	if *asJSON {
		out := make([]findings.Finding, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.Finding)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if len(rows) == 0 {
		fmt.Println("no open findings")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FINDING\tSEV\tSTATUS\tRULE\tSUBJECTS\tSUMMARY")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			r.FindingID, r.Severity, statusWith(r), r.Rule.ID, len(r.Subjects), short(r.Summary, 70))
	}
	w.Flush()
	return nil
}

func statusWith(r findings.Row) string {
	switch r.Status {
	case findings.StatusWaived:
		if r.Waiver != nil {
			return "waived by " + r.Waiver.Actor.ID
		}
	case findings.StatusAcknowledged:
		if r.DisposedBy != "" {
			return "ack by " + r.DisposedBy
		}
	}
	return r.Status
}

// takeID pulls the finding id out of the arguments wherever it sits, so
// "ack <id> --by x" and "ack --by x <id>" both work. The flag package stops
// at the first positional argument otherwise.
func takeID(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") && (i == 0 || !strings.HasPrefix(args[i-1], "-") ||
			strings.Contains(args[i-1], "=")) {
			return a, append(append([]string{}, args[:i]...), args[i+1:]...)
		}
	}
	return "", args
}

func cmdFindingsAck(ctx context.Context, args []string) error {
	id, rest := takeID(args)
	fs := flag.NewFlagSet("findings ack", flag.ExitOnError)
	by := fs.String("by", "", "who is acknowledging (required)")
	fs.Parse(rest)
	if id == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: gyst findings ack <finding-id> --by <name>")
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := findings.Acknowledge(ctx, s, id, *by); err != nil {
		return err
	}
	fmt.Printf("acknowledged %s by %s\n", id, *by)
	return nil
}

func cmdFindingsWaive(ctx context.Context, args []string) error {
	id, rest := takeID(args)
	fs := flag.NewFlagSet("findings waive", flag.ExitOnError)
	by := fs.String("by", "", "who is waiving (required)")
	reason := fs.String("reason", "", "why (required)")
	until := fs.String("until", "", "expiry, YYYY-MM-DD (optional)")
	fs.Parse(rest)
	if id == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: gyst findings waive <finding-id> --by <name> --reason <text> [--until YYYY-MM-DD]")
	}
	w := findings.Waiver{
		Actor:    observe.Actor{Kind: "user", ID: *by},
		Reason:   strings.TrimSpace(*reason),
		WaivedAt: time.Now().UTC(),
	}
	if *until != "" {
		t, err := time.Parse("2006-01-02", *until)
		if err != nil {
			return fmt.Errorf("--until: %w", err)
		}
		w.ExpiresAt = &t
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := findings.Waive(ctx, s, id, w); err != nil {
		return err
	}
	fmt.Printf("waived %s by %s\n", id, *by)
	return nil
}
