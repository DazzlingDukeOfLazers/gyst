package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/report"
)

// cmdReview is the queue: which project records still need a person's
// judgment, ordered by consequence, and one candidate explained with the
// consequence of confirming or ignoring it. It reads and does not write;
// the decision is recorded with gyst assert project.
func cmdReview(ctx context.Context, args []string) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	doc, err := report.Build(ctx, s, time.Now().UTC(), Version)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return reviewQueue(doc)
	}
	return reviewOne(doc, strings.Join(args, " "))
}

type candidate struct {
	p        *report.Project
	parent   string
	findings int
	cross    int
	nameDupe bool
}

// candidates are the records awaiting judgment, ordered by expected
// consequence: nested boundaries first, then open findings, then size,
// then generic names, then the rest.
func candidates(doc *report.Document) ([]candidate, map[string]*report.Project) {
	byID := map[string]*report.Project{}
	names := map[string]int{}
	for i := range doc.Projects {
		byID[doc.Projects[i].ProjectID] = &doc.Projects[i]
		names[doc.Projects[i].Name]++
	}
	open := map[string]int{}
	cross := map[string]int{}
	for _, f := range doc.Findings {
		if f.Status == "resolved" {
			continue
		}
		for _, id := range f.Projects {
			open[id]++
			if f.CrossProject {
				cross[id]++
			}
		}
	}
	var out []candidate
	for i := range doc.Projects {
		p := &doc.Projects[i]
		if p.State != "candidate" {
			continue
		}
		c := candidate{p: p, findings: open[p.ProjectID], cross: cross[p.ProjectID], nameDupe: names[p.Name] > 1}
		if p.PhysicallyWithin != nil {
			if q := byID[*p.PhysicallyWithin]; q != nil {
				c.parent = q.Name
			}
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.parent != "") != (b.parent != "") {
			return a.parent != ""
		}
		if a.cross != b.cross {
			return a.cross > b.cross
		}
		if a.findings != b.findings {
			return a.findings > b.findings
		}
		if a.nameDupe != b.nameDupe {
			return a.nameDupe
		}
		if a.p.FileCount != b.p.FileCount {
			return a.p.FileCount > b.p.FileCount
		}
		return a.p.Name < b.p.Name
	})
	return out, byID
}

func reviewQueue(doc *report.Document) error {
	c := doc.Report.Counts
	reviewed := c.ProjectsConfirmed + c.ProjectsIgnored
	fmt.Printf("%d project records: %d declared by manifests, %d confirmed, %d ignored, %d candidates from markers\n",
		c.Projects, c.ProjectsDeclared, c.ProjectsConfirmed, c.ProjectsIgnored, c.ProjectsCandidate)
	if c.ProjectsCandidate == 0 {
		fmt.Printf("\nnothing to review: every candidate has been judged (%d reviewed)\n", reviewed)
		return nil
	}
	fmt.Printf("\nWe found %d possible projects. Help Gyst identify what they are.\n", c.ProjectsCandidate+reviewed)
	fmt.Printf("%d reviewed · %d remaining\n\n", reviewed, c.ProjectsCandidate)
	cands, _ := candidates(doc)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tCANDIDATE\tWHERE\tFILES\tFINDINGS\tWHY REVIEW")
	for i, cd := range cands {
		where := cd.p.Boundary.SourceID
		if cd.parent != "" {
			where = "inside " + cd.parent
		}
		var why []string
		if cd.parent != "" {
			why = append(why, "nested boundary")
		}
		if cd.cross > 0 {
			why = append(why, fmt.Sprintf("%d cross-project findings", cd.cross))
		}
		if cd.nameDupe {
			why = append(why, "generic name")
		}
		if len(why) == 0 {
			why = append(why, "marker suggestion")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%d\t%s\n", i+1, cd.p.Name, where, cd.p.FileCount, cd.findings, strings.Join(why, ", "))
	}
	w.Flush()
	fmt.Printf("\nnext: gyst review %s\n", cands[0].p.Name)
	return nil
}

func reviewOne(doc *report.Document, needle string) error {
	cands, byID := candidates(doc)
	var pick *candidate
	for i := range cands {
		if cands[i].p.ProjectID == needle || cands[i].p.Name == needle {
			if pick != nil {
				return fmt.Errorf("%q names more than one candidate; use the project id (gyst projects)", needle)
			}
			pick = &cands[i]
		}
	}
	if pick == nil {
		if p := byID[needle]; p != nil {
			fmt.Printf("%s is %s, not a candidate; retract its assertion to review it again\n", p.Name, p.State)
			return nil
		}
		return fmt.Errorf("no candidate named %q", needle)
	}
	p := pick.p
	fmt.Printf("%s\n", p.Name)
	fmt.Printf("  record     %s  candidate · %s\n", p.ProjectID, p.Basis)
	fmt.Printf("  boundary   %s:%s\n", p.Boundary.SourceID, orRoot(p.Boundary.Locator))
	if pick.parent != "" {
		fmt.Printf("  inside     %s (physically; a fact about folders, not organisation)\n", pick.parent)
	}
	fmt.Printf("  why        %s\n", wrap(p.Explanation, 60, "             "))
	fmt.Printf("  evidence   %s\n", strings.Join(p.Evidence, ", "))
	owned, vendored := 0, 0
	for _, f := range doc.Files {
		if !f.Present {
			continue
		}
		for _, m := range f.Projects {
			if m.ProjectID == p.ProjectID {
				if f.Vendored {
					vendored++
				} else {
					owned++
				}
				break
			}
		}
	}
	fmt.Printf("  files      %d, of which %d vendored or build output\n", owned+vendored, vendored)
	fmt.Printf("  findings   %d open, %d of them crossing into other projects\n", pick.findings, pick.cross)
	fmt.Printf("\nIf you confirm it as a project:\n")
	fmt.Printf("  the record becomes confirmed at confidence 1.00; its %d member files keep this membership;\n", owned+vendored)
	fmt.Printf("  it leaves the review queue; nothing on disk changes.\n")
	fmt.Printf("If you ignore it as a project:\n")
	fmt.Printf("  the record stays visible as ignored and claims no files; its %d member files keep any other\n", owned+vendored)
	fmt.Printf("  memberships they have; it leaves the review queue; nothing on disk changes.\n")
	fmt.Printf("Either way you can retract the decision later and the marker suggestion returns.\n")
	fmt.Printf("\n  gyst assert project confirm %s --by <you> --reason \"...\"\n", p.ProjectID)
	fmt.Printf("  gyst assert project ignore  %s --by <you> --reason \"...\"\n", p.ProjectID)
	return nil
}

func orRoot(s string) string {
	if s == "" {
		return "(root)"
	}
	return s
}
