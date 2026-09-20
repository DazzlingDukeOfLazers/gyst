package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/identity"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// cmdExplain answers "why does Gyst believe this?" for one file.
//
// Every line it prints carries the observation id behind it. A fact with no
// citation is a fact Gyst should not be stating, so anything unsupported is
// reported as unknown rather than filled in.
func cmdExplain(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gyst explain <locator>")
	}
	needle := args[0]

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	// Match on suffix so a user can paste a bare filename.
	f, err := s.FindPresentFile(ctx, needle)
	if err != nil {
		return fmt.Errorf("no current file matching %q: %w", needle, err)
	}
	sourceID, locator, latestSeq := f.SourceID, f.Locator, f.LatestSeq
	var digest *string
	if f.Digest != "" {
		d := f.Digest
		digest = &d
	}
	size := &f.Size

	fmt.Printf("%s\n", locator)
	fmt.Printf("  source     %s\n", sourceID)
	fmt.Printf("  size       %s\n", humanBytes(derefInt(size)))
	fmt.Printf("  digest     %s\n", derefStr(digest, "(not read under effective policy)"))

	if err := explainAuthority(ctx, s, sourceID, locator); err != nil {
		return err
	}
	if err := explainProjects(ctx, s, sourceID, locator); err != nil {
		return err
	}
	if err := explainFindings(ctx, s, sourceID, locator); err != nil {
		return err
	}

	// --- evidence ---------------------------------------------------------
	fmt.Printf("\nevidence\n")
	obs, err := s.ObservationsOf(ctx, sourceID, locator)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  SEQ\tOBSERVATION\tWHEN\tCLAIM\tDIGEST\tPOLICY\tBY")
	for _, o := range obs {
		mark := "  "
		if o.Seq == latestSeq {
			mark = "->"
		}
		fmt.Fprintf(w, "%s%d\t%s\t%s\t%s\t%s\t%s\t%s@%s\n",
			mark, o.Seq, o.ObservationID, o.ObservedAt.Local().Format("01-02 15:04:05"), short(o.ClaimType, 24),
			firstN(o.Digest, 8), o.ContentLevel, o.Connector, o.ConnectorVersion)
	}
	w.Flush()
	fmt.Printf("  %d observation(s); the arrow marks the one the projection currently reflects\n", len(obs))

	// --- identity ---------------------------------------------------------
	version, profile, err := identity.ActivePolicy(ctx, s)
	if err != nil {
		return err
	}
	if version == "" {
		// Not a reason to stop. Git history and source-native relations are
		// derived from evidence, not from an identity interpretation.
		fmt.Printf("\nidentity   no policy active; run 'gyst identity apply'\n")
	} else if err := explainIdentity(ctx, s, version, string(profile), sourceID, locator); err != nil {
		return err
	}

	// --- git history ------------------------------------------------------
	//
	// Reachable only because reconciliation matched this file's path to the
	// paths inside commits. Both connectors observed the same bytes; this is
	// the join between them.
	hist, err := s.GitHistoryOf(ctx, sourceID, locator)
	if err != nil {
		return err
	}
	if len(hist) == 0 {
		fmt.Println("\nchanged by (git)\n  no commit in any observed repository touches this path")
	} else {
		fmt.Println("\nchanged by (git)")
		hw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(hw, "  COMMIT\tWHEN\tAUTHOR\tMESSAGE\tEVIDENCE")
		for _, t := range hist {
			fmt.Fprintf(hw, "  %s\t%s\t%s\t%s\t%s\n", firstN(t.OID, 12),
				t.AuthoredAt.Local().Format("2006-01-02"), short(t.Author, 24), short(t.Message, 40),
				firstN(t.Evidence[0], 16))
		}
		hw.Flush()
	}

	// --- relations --------------------------------------------------------
	rels, err := s.RelationsOf(ctx, version, sourceID, locator)
	if err != nil {
		return err
	}
	fmt.Println("\nrelations")
	for _, r := range rels {
		fmt.Printf("  %s  %s -> %s  (confidence %.2f, %s)\n", r.Type, r.FromLocator, r.ToLocator, r.Confidence, r.Precedence)
		fmt.Printf("      because  %s\n", wrap(r.Explanation, 66, "               "))
		fmt.Printf("      evidence %s\n", strings.Join(r.Evidence, ", "))
	}
	if len(rels) == 0 {
		fmt.Println("  none")
	}
	return nil
}

func currentSuffix(b bool) string {
	if b {
		return "  (current)"
	}
	return ""
}

func derefStr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

func derefInt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func firstN(s string, n int) string {
	if s == "" {
		return "-"
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// explainAuthority says where the authority is, or that nobody knows.
func explainAuthority(ctx context.Context, s *store.Store, sourceID, locator string) error {
	a, err := authority.Of(ctx, s, authority.Key{SourceID: sourceID, Locator: locator})
	if err != nil {
		return err
	}
	if a == nil {
		fmt.Printf("  authority  not resolved yet; run a scan\n")
		return nil
	}
	head := map[string]string{
		authority.StateDeclared: "declared", authority.StateLikely: "likely",
		authority.StateMultiple: "multiple candidates", authority.StateNone: "none identified",
	}[a.State]
	if a.Of != nil {
		head += fmt.Sprintf(": %s (%s %.2f)", a.Of.Locator, a.Basis, a.Confidence)
	}
	fmt.Printf("  authority  %s\n", head)
	fmt.Printf("             %s\n", wrap(a.Explanation, 60, "             "))
	return nil
}

// explainProjects lists the projects a file belongs to and on what basis.
// Several is normal. None is reported as such, not left blank.
func explainProjects(ctx context.Context, s *store.Store, sourceID, locator string) error {
	ps, err := project.ProjectsOf(ctx, s, sourceID, locator)
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		fmt.Printf("  projects   no project membership\n")
		return nil
	}
	for i, p := range ps {
		label := "  projects   "
		if i > 0 {
			label = "             "
		}
		fmt.Printf("%s%s (%s)  %s %.2f  via %s\n", label, p.ProjectID, p.Name, p.Basis, p.Confidence, p.Pattern)
	}
	return nil
}

// explainFindings lists open findings that name this file.
func explainFindings(ctx context.Context, s *store.Store, sourceID, locator string) error {
	rows, err := findings.ForFile(ctx, s, sourceID, locator)
	if err != nil {
		return err
	}
	for i, r := range rows {
		label := "  findings   "
		if i > 0 {
			label = "             "
		}
		fmt.Printf("%s%s  %s %s  %s\n", label, r.FindingID, r.Severity, r.Status,
			wrap(r.Summary, 60, "             "))
	}
	return nil
}

// wrap breaks explanation text so a long reason stays readable in a terminal.
func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	var b strings.Builder
	line := 0
	for i, word := range words {
		if line > 0 && line+len(word)+1 > width {
			b.WriteString("\n" + indent)
			line = 0
		} else if i > 0 {
			b.WriteString(" ")
			line++
		}
		b.WriteString(word)
		line += len(word)
	}
	return b.String()
}

// explainIdentity reports how the active profile grouped this locator and what
// it was grouped with.
func explainIdentity(ctx context.Context, s *store.Store, version, profile, sourceID, locator string) error {
	m, groupingKey, err := s.MembershipOf(ctx, version, sourceID, locator)
	if err != nil {
		fmt.Printf("\nidentity   not grouped under %s\n", version)
		return nil
	}

	fmt.Printf("\nidentity (policy %s, profile %s)\n", version, profile)
	fmt.Printf("  artifact   %s  %s\n", m.ArtifactID, groupingKey)
	fmt.Printf("  rule       %s (confidence %.2f)\n", m.Rule, m.Confidence)
	fmt.Printf("  version    %s%s\n", derefStr(m.VersionLabel, "-"), currentSuffix(m.IsCurrent))
	fmt.Printf("  because    %s\n", wrap(m.Explanation, 68, "             "))

	sib, err := s.Siblings(ctx, version, m.ArtifactID, locator)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, x := range sib {
		if i == 0 {
			fmt.Fprintln(w, "  GROUPED WITH\tVER\tCONF\t")
		}
		fmt.Fprintf(w, "  %s\t%s\t%.2f\t%s\n", x.Locator, derefStr(x.VersionLabel, "-"), x.Confidence, currentSuffix(x.IsCurrent))
	}
	w.Flush()
	if len(sib) == 0 {
		fmt.Printf("  grouped with nothing else under this profile\n")
	}
	return nil
}
