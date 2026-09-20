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

	rows, err := s.Pool().Query(ctx, `
		SELECT p.project_id, p.name, p.basis, p.confidence, p.explanation,
		       (SELECT count(*) FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT count(DISTINCT fp.source_id) FROM file_projects fp WHERE fp.project_id=p.project_id),
		       (SELECT string_agg(m.source_id || ':' || m.pattern, '  ' ORDER BY m.source_id, m.pattern)
		          FROM project_members m WHERE m.project_id=p.project_id)
		FROM projects p
		ORDER BY p.basis, p.name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tNAME\tBASIS\tCONF\tFILES\tSOURCES\tMEMBERS")
	n := 0
	var notes []string
	for rows.Next() {
		var id, name, basis, explanation string
		var conf float64
		var files, sources int64
		var members *string
		if err := rows.Scan(&id, &name, &basis, &conf, &explanation, &files, &sources, &members); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%d\t%d\t%s\n",
			id, name, basis, conf, files, sources, derefStr(members, "-"))
		notes = append(notes, fmt.Sprintf("%-14s %s", id, wrap(explanation, 64, strings.Repeat(" ", 15))))
		n++
	}
	w.Flush()
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		fmt.Println("no projects: no manifest or marker evidence has been observed")
		return nil
	}
	fmt.Println()
	for _, l := range notes {
		fmt.Println(l)
	}
	return nil
}
