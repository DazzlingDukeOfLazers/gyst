package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/identity"
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
	var sourceID, locator string
	var latestSeq int64
	var digest *string
	var size *int64
	err = s.Pool().QueryRow(ctx, `
		SELECT source_id, locator, latest_seq, content_digest_hex, size_bytes
		FROM current_files
		WHERE present AND (locator = $1 OR locator LIKE '%' || $1)
		ORDER BY length(locator) LIMIT 1`, needle).
		Scan(&sourceID, &locator, &latestSeq, &digest, &size)
	if err != nil {
		return fmt.Errorf("no current file matching %q: %w", needle, err)
	}

	fmt.Printf("%s\n", locator)
	fmt.Printf("  source     %s\n", sourceID)
	fmt.Printf("  size       %s\n", humanBytes(derefInt(size)))
	fmt.Printf("  digest     %s\n", derefStr(digest, "(not read under effective policy)"))

	// --- evidence ---------------------------------------------------------
	fmt.Printf("\nevidence\n")
	rows, err := s.Pool().Query(ctx, `
		SELECT observation_id, seq, observed_at, claim_type,
		       coalesce(content_digest_hex,''), native_version_value,
		       policy->>'content_level', connector, connector_version
		FROM observations
		WHERE source_id=$1 AND locator=$2
		ORDER BY seq`, sourceID, locator)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  SEQ\tOBSERVATION\tWHEN\tCLAIM\tDIGEST\tPOLICY\tBY")
	count := 0
	for rows.Next() {
		var id, claim, dg, nv, policy, connector, cver string
		var seq int64
		var at time.Time
		if err := rows.Scan(&id, &seq, &at, &claim, &dg, &nv, &policy, &connector, &cver); err != nil {
			rows.Close()
			return err
		}
		mark := "  "
		if seq == latestSeq {
			mark = "->"
		}
		fmt.Fprintf(w, "%s%d\t%s\t%s\t%s\t%s\t%s\t%s@%s\n",
			mark, seq, id, at.Local().Format("01-02 15:04:05"), short(claim, 24),
			firstN(dg, 8), policy, connector, cver)
		count++
	}
	rows.Close()
	w.Flush()
	fmt.Printf("  %d observation(s); the arrow marks the one the projection currently reflects\n", count)

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
	hist, err := s.Pool().Query(ctx, `
		SELECT c.oid, c.author, c.message, c.authored_at, r.evidence
		FROM relations r
		JOIN commits c ON (c.source_id || '@' || c.oid) = r.from_locator
		               AND c.source_id = r.from_source
		WHERE r.type='contains' AND r.to_source=$1 AND r.to_locator=$2
		ORDER BY c.authored_at DESC, c.seq DESC`, sourceID, locator)
	if err != nil {
		return err
	}
	touched := 0
	hw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for hist.Next() {
		var oid, author, message string
		var at time.Time
		var evidence []string
		if err := hist.Scan(&oid, &author, &message, &at, &evidence); err != nil {
			hist.Close()
			return err
		}
		if touched == 0 {
			fmt.Println("\nchanged by (git)")
			fmt.Fprintln(hw, "  COMMIT\tWHEN\tAUTHOR\tMESSAGE\tEVIDENCE")
		}
		fmt.Fprintf(hw, "  %s\t%s\t%s\t%s\t%s\n", firstN(oid, 12),
			at.Local().Format("2006-01-02"), short(author, 24), short(message, 40),
			firstN(evidence[0], 16))
		touched++
	}
	hist.Close()
	hw.Flush()
	if touched == 0 {
		fmt.Println("\nchanged by (git)\n  no commit in any observed repository touches this path")
	}

	// --- relations --------------------------------------------------------
	rel, err := s.Pool().Query(ctx, `
		SELECT type, from_locator, to_locator, confidence, precedence, evidence, explanation
		FROM relations
		WHERE (identity_policy_version IS NULL OR identity_policy_version=$1)
		  AND ((from_source=$2 AND from_locator=$3) OR (to_source=$2 AND to_locator=$3))
		ORDER BY type, to_locator`, version, sourceID, locator)
	if err != nil {
		return err
	}
	defer rel.Close()

	fmt.Println("\nrelations")
	n := 0
	for rel.Next() {
		var typ, from, to, precedence, why string
		var conf float64
		var evidence []string
		if err := rel.Scan(&typ, &from, &to, &conf, &precedence, &evidence, &why); err != nil {
			return err
		}
		fmt.Printf("  %s  %s -> %s  (confidence %.2f, %s)\n", typ, from, to, conf, precedence)
		fmt.Printf("      because  %s\n", wrap(why, 66, "               "))
		fmt.Printf("      evidence %s\n", strings.Join(evidence, ", "))
		n++
	}
	if n == 0 {
		fmt.Println("  none")
	}
	return rel.Err()
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
	var artifactID, groupingKey, rule, explanation string
	var versionLabel *string
	var isCurrent bool
	var conf float64
	err := s.Pool().QueryRow(ctx, `
		SELECT m.artifact_id, a.grouping_key, m.rule, m.explanation,
		       m.version_label, m.is_current, m.confidence
		FROM artifact_members m
		JOIN artifacts a ON a.identity_policy_version=m.identity_policy_version
		                AND a.artifact_id=m.artifact_id
		WHERE m.identity_policy_version=$1 AND m.source_id=$2 AND m.locator=$3`,
		version, sourceID, locator).
		Scan(&artifactID, &groupingKey, &rule, &explanation, &versionLabel, &isCurrent, &conf)
	if err != nil {
		fmt.Printf("\nidentity   not grouped under %s\n", version)
		return nil
	}

	fmt.Printf("\nidentity (policy %s, profile %s)\n", version, profile)
	fmt.Printf("  artifact   %s  %s\n", artifactID, groupingKey)
	fmt.Printf("  rule       %s (confidence %.2f)\n", rule, conf)
	fmt.Printf("  version    %s%s\n", derefStr(versionLabel, "-"), currentSuffix(isCurrent))
	fmt.Printf("  because    %s\n", wrap(explanation, 68, "             "))

	sib, err := s.Pool().Query(ctx, `
		SELECT locator, coalesce(version_label,'-'), is_current, confidence
		FROM artifact_members
		WHERE identity_policy_version=$1 AND artifact_id=$2 AND locator <> $3
		ORDER BY locator`, version, artifactID, locator)
	if err != nil {
		return err
	}
	defer sib.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	n := 0
	for sib.Next() {
		var l, vl string
		var cur bool
		var c float64
		if err := sib.Scan(&l, &vl, &cur, &c); err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintln(w, "  GROUPED WITH\tVER\tCONF\t")
		}
		fmt.Fprintf(w, "  %s\t%s\t%.2f\t%s\n", l, vl, c, currentSuffix(cur))
		n++
	}
	w.Flush()
	if n == 0 {
		fmt.Printf("  grouped with nothing else under this profile\n")
	}
	return sib.Err()
}
