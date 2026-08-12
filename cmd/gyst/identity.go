package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/DazzlingDukeOfLazers/gyst/internal/identity"
)

func cmdIdentity(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gyst identity <preview|apply|verify|status> [flags]")
	}
	switch args[0] {
	case "preview":
		return identityPreview(ctx, args[1:], false)
	case "apply":
		return identityPreview(ctx, args[1:], true)
	case "verify":
		return identityVerify(ctx)
	case "status":
		return identityStatus(ctx)
	default:
		return fmt.Errorf("unknown identity subcommand %q", args[0])
	}
}

// identityPreview computes a grouping and shows it. With apply set, the same
// plan is then written. Preview and apply run identical code so that what a
// user approves is what gets stored.
func identityPreview(ctx context.Context, args []string, apply bool) error {
	name := "preview"
	if apply {
		name = "apply"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	profile := fs.String("profile", string(identity.SuffixAsVersion), "identity profile")
	version := fs.String("version", "", "policy version label (default ip_<profile>)")
	verbose := fs.Bool("v", false, "show every member, not just ambiguous groups")
	fs.Parse(args)

	p := identity.Profile(*profile)
	if !identity.Valid(p) {
		return fmt.Errorf("unknown profile %q; one of %v", *profile, identity.Profiles())
	}
	policyVersion := *version
	if policyVersion == "" {
		policyVersion = "ip_" + *profile
	}

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	plan, err := identity.Build(ctx, s, p, policyVersion)
	if err != nil {
		return err
	}

	grouped, ambiguous := 0, 0
	for _, g := range plan.Groups {
		if len(g.Members) > 1 {
			grouped++
			if g.Confidence < identity.SupersedesThreshold {
				ambiguous++
			}
		}
	}

	fmt.Printf("profile    %s  (policy %s)\n", p, policyVersion)
	fmt.Printf("artifacts  %d from %d files\n", len(plan.Groups), countMembers(plan))
	fmt.Printf("grouped    %d artifacts have more than one member (%d below the %.2f confidence threshold)\n",
		grouped, ambiguous, identity.SupersedesThreshold)

	supersedes, compareSet := 0, 0
	for _, r := range plan.Relations {
		switch r.Type {
		case "supersedes":
			supersedes++
		case "compare-set-with":
			compareSet++
		}
	}
	fmt.Printf("relations  %d supersedes, %d compare-set-with\n\n", supersedes, compareSet)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONF\tCURRENT\tVER\tLOCATOR\tWHY")
	for _, g := range plan.Groups {
		if len(g.Members) < 2 && !*verbose {
			continue
		}
		fmt.Fprintf(w, "\t\t\t%s\t\n", "── "+g.GroupingKey)
		for _, m := range g.Members {
			cur := ""
			if m.IsCurrent {
				cur = "current"
			}
			fmt.Fprintf(w, "%.2f\t%s\t%s\t  %s\t%s\n",
				m.Match.Confidence, cur, dash(m.Match.VersionLabel), m.Locator,
				short(m.Match.Explanation, 72))
		}
	}
	w.Flush()

	if len(plan.Relations) > 0 {
		fmt.Println("\nrelations:")
		rw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(rw, "TYPE\tCONF\tFROM\tTO\tWHY")
		for _, r := range plan.Relations {
			fmt.Fprintf(rw, "%s\t%.2f\t%s\t%s\t%s\n",
				r.Type, r.Confidence, r.FromLocator, r.ToLocator, short(r.Explanation, 76))
		}
		rw.Flush()
	}

	if !apply {
		fmt.Printf("\npreview only; nothing written. Re-run with 'gyst identity apply --profile %s' to activate.\n", p)
		return nil
	}

	before, obsCount, err := identity.LogFingerprint(ctx, s)
	if err != nil {
		return err
	}
	if err := identity.Apply(ctx, s, plan); err != nil {
		return err
	}
	after, _, err := identity.LogFingerprint(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("\napplied policy %s\n", policyVersion)
	if before != after {
		return fmt.Errorf("EVIDENCE MUTATED: the observation log changed while applying a profile")
	}
	fmt.Printf("observation log unchanged (%d observations, %s)\n", obsCount, before[:16])
	return nil
}

// identityVerify is the day 3 exit criterion: switching profiles rebuilds the
// grouping and leaves the evidence untouched.
func identityVerify(ctx context.Context) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	start, obsCount, err := identity.LogFingerprint(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("observation log   %s  (%d observations)\n\n", start[:32], obsCount)

	type row struct {
		profile  identity.Profile
		groups   int
		super    int
		compare  int
		logAfter string
	}
	var rows []row

	for _, p := range identity.Profiles() {
		plan, err := identity.Build(ctx, s, p, "ip_verify_"+string(p))
		if err != nil {
			return err
		}
		if err := identity.Apply(ctx, s, plan); err != nil {
			return err
		}
		fp, _, err := identity.LogFingerprint(ctx, s)
		if err != nil {
			return err
		}
		r := row{profile: p, groups: len(plan.Groups), logAfter: fp}
		for _, rel := range plan.Relations {
			switch rel.Type {
			case "supersedes":
				r.super++
			case "compare-set-with":
				r.compare++
			}
		}
		rows = append(rows, r)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROFILE\tARTIFACTS\tSUPERSEDES\tCOMPARE-SET\tLOG AFTER")
	diverged := false
	for _, r := range rows {
		mark := "unchanged"
		if r.logAfter != start {
			mark = "MUTATED"
			diverged = true
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", r.profile, r.groups, r.super, r.compare, mark)
	}
	w.Flush()

	if diverged {
		return fmt.Errorf("a profile switch altered the observation log")
	}
	fmt.Printf("\nevery profile rebuilt grouping from the same evidence; the log is byte-identical throughout\n")
	return nil
}

func identityStatus(ctx context.Context) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	version, profile, err := identity.ActivePolicy(ctx, s)
	if err != nil {
		return err
	}
	if version == "" {
		fmt.Println("no identity policy is active; run 'gyst identity apply'")
		return nil
	}
	fmt.Printf("active policy %s (profile %s)\n", version, profile)

	rows, err := s.Pool().Query(ctx, `
		SELECT a.grouping_key, a.member_count, a.confidence
		FROM artifacts a WHERE a.identity_policy_version=$1 AND a.member_count > 1
		ORDER BY a.confidence, a.grouping_key`, version)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONF\tMEMBERS\tARTIFACT")
	for rows.Next() {
		var key string
		var n int
		var conf float64
		if err := rows.Scan(&key, &n, &conf); err != nil {
			return err
		}
		fmt.Fprintf(w, "%.2f\t%d\t%s\n", conf, n, key)
	}
	w.Flush()
	return rows.Err()
}

func countMembers(p *identity.Plan) int {
	n := 0
	for _, g := range p.Groups {
		n += len(g.Members)
	}
	return n
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
