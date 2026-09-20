package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// cmdProjects lists projects and where each one's membership comes from.
func cmdProjects(ctx context.Context, args []string) error {
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	rows, err := s.ProjectSummaries(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("no projects: no manifest or marker evidence has been observed")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tNAME\tBASIS\tCONF\tFILES\tSOURCES\tMEMBERS")
	var notes []string
	for _, p := range rows {
		var members []string
		for _, m := range p.Members {
			members = append(members, m.SourceID+":"+m.Pattern)
		}
		ms := strings.Join(members, "  ")
		if ms == "" {
			ms = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%d\t%d\t%s\n",
			p.ProjectID, p.Name, p.Basis, p.Confidence, p.FileCount, len(p.SourceIDs), ms)
		notes = append(notes, fmt.Sprintf("%-14s %s", p.ProjectID, wrap(p.Explanation, 64, strings.Repeat(" ", 15))))
	}
	w.Flush()
	fmt.Println()
	for _, l := range notes {
		fmt.Println(l)
	}
	return nil
}
