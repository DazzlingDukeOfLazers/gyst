package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/report"
)

// Version is what the report records as its generator. Not yet tied to a
// release; that is a packaging step.
const Version = "0.1.0-dev"

// cmdReport writes the whole picture as one JSON document: the contract the
// static report consumes.
func cmdReport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", "", "write to this file instead of stdout")
	fs.Parse(args)

	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()

	doc, err := report.Build(ctx, s, time.Now().UTC(), Version)
	if err != nil {
		return err
	}
	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if *out != "" {
		c := doc.Report.Counts
		fmt.Fprintf(os.Stderr, "wrote %s: %d sources, %d projects, %d files, %d artifacts, %d relations, %d findings (%d open)\n",
			*out, c.Sources, c.Projects, c.FilesPresent+c.FilesAbsent, c.Artifacts, c.Relations, len(doc.Findings), c.FindingsOpen)
	}
	return nil
}
