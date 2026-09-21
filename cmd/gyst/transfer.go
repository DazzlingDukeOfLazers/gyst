package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/authority"
	"github.com/DazzlingDukeOfLazers/gyst/internal/bundle"
	"github.com/DazzlingDukeOfLazers/gyst/internal/findings"
	"github.com/DazzlingDukeOfLazers/gyst/internal/project"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

func keyDir() string { return filepath.Join(store.DataDir(), "keys") }

// cmdKey manages the signing identity a bundle carries.
func cmdKey(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: gyst key init <id> | gyst key show <id>")
	}
	switch args[0] {
	case "init":
		k, err := bundle.Generate(keyDir(), args[1])
		if err != nil {
			return err
		}
		fmt.Printf("key        %s\n", k.ID)
		fmt.Printf("public     %s\n", k.PublicBase64())
		fmt.Printf("private    %s (keep this; it is the only copy)\n", filepath.Join(keyDir(), k.ID+".key"))
		fmt.Printf("\ngive the public key to anyone who should trust your bundles:\n  gyst trust add %s %s --by <their name>\n", k.ID, k.PublicBase64())
		return nil
	case "show":
		k, err := bundle.Load(keyDir(), args[1])
		if err != nil {
			return err
		}
		fmt.Printf("key        %s\npublic     %s\n", k.ID, k.PublicBase64())
		return nil
	}
	return fmt.Errorf("usage: gyst key init <id> | gyst key show <id>")
}

// cmdTrust manages which senders' bundles are accepted.
func cmdTrust(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gyst trust add <sender> <public-key> --by <name> | gyst trust list")
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("trust add", flag.ExitOnError)
		by := fs.String("by", "", "who is deciding to trust this sender (required)")
		note := fs.String("note", "", "why, or how the key was verified")
		rest := args[1:]
		var pos []string
		for len(rest) > 0 {
			if len(rest[0]) > 0 && rest[0][0] != '-' {
				pos = append(pos, rest[0])
				rest = rest[1:]
				continue
			}
			break
		}
		fs.Parse(rest)
		pos = append(pos, fs.Args()...)
		if len(pos) != 2 || *by == "" {
			return fmt.Errorf("usage: gyst trust add <sender> <public-key> --by <name> [--note text]")
		}
		if _, err := bundle.ParsePublic(pos[1]); err != nil {
			return err
		}
		if err := s.TrustKey(ctx, store.TrustedKey{SenderID: pos[0], PublicKey: pos[1], AddedAt: time.Now().UTC(), AddedBy: *by, Note: *note}); err != nil {
			return err
		}
		fmt.Printf("trusted %s (recorded by %s)\n", pos[0], *by)
		return nil
	case "list":
		keys, err := s.TrustedKeys(ctx)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Println("no trusted senders")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SENDER\tADDED\tBY\tPUBLIC KEY\tNOTE")
		for _, k := range keys {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", k.SenderID, k.AddedAt.Local().Format("2006-01-02"), k.AddedBy, k.PublicKey, k.Note)
		}
		w.Flush()
		return nil
	}
	return fmt.Errorf("usage: gyst trust add <sender> <public-key> --by <name> | gyst trust list")
}

// cmdExport writes a signed bundle of this Gyst's own observations.
func cmdExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	keyID := fs.String("key", "", "signing key id from gyst key init (required)")
	egress := fs.String("egress", "facility", "destination class: device, facility, or connected-server")
	out := fs.String("out", "", "bundle file to write (required)")
	var sources multiFlag
	fs.Var(&sources, "source", "source to include (repeatable; default: every local source)")
	fs.Parse(args)
	if *keyID == "" || *out == "" {
		return fmt.Errorf("usage: gyst export --key <id> --egress facility [--source <id>]... --out bundle.jsonl")
	}
	key, err := bundle.Load(keyDir(), *keyID)
	if err != nil {
		return err
	}
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := bundle.Export(ctx, s, key, *egress, sources, f, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("bundle     %s\n", st.Header.BundleID)
	fmt.Printf("sender     %s\n", key.ID)
	fmt.Printf("egress     %s\n", *egress)
	fmt.Printf("sources    %d\n", st.Sources)
	fmt.Printf("written    %d observations\n", st.Written)
	if st.Withheld > 0 {
		fmt.Printf("withheld   %d observations whose recorded egress does not permit %s (rescan with --egress to change)\n", st.Withheld, *egress)
	}
	fmt.Printf("wrote      %s\n", *out)
	return nil
}

// cmdImport verifies a bundle and appends its observations.
func cmdImport(ctx context.Context, args []string) error {
	path, rest := takeID(args)
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	tofu := fs.Bool("trust-on-first-use", false, "record an unknown sender's key instead of refusing")
	by := fs.String("by", "", "who is importing; recorded with any key trusted here")
	fs.Parse(rest)
	if path == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: gyst import bundle.jsonl [--trust-on-first-use --by <name>]")
	}
	if *tofu && *by == "" {
		return fmt.Errorf("--trust-on-first-use needs --by: trusting a key is a person's decision")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	st, err := bundle.Import(ctx, s, f, bundle.ImportOptions{TrustOnFirstUse: *tofu, By: *by}, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Printf("bundle     %s from %s, created %s, egress %s\n", st.Header.BundleID, st.Header.Sender.ID,
		st.Header.CreatedAt.Local().Format("2006-01-02 15:04"), st.Header.Egress)
	fmt.Printf("verified   body digest and signature\n")
	if st.NewlyTrust {
		fmt.Printf("trusted    %s on first use, recorded by %s\n", st.Header.Sender.ID, *by)
	}
	fmt.Printf("sources    %d registered under %s/\n", st.Sources, st.Header.Sender.ID)
	fmt.Printf("appended   %d new observations (%d already known)\n", st.Appended, st.Known)

	// The same projections a scan runs, so the imported files take part in
	// grouping, membership, findings, and authority alongside local ones.
	if _, err := project.Apply(ctx, s); err != nil {
		return err
	}
	if _, err := project.ProjectRenames(ctx, s); err != nil {
		return err
	}
	if _, err := project.ProjectMembership(ctx, s); err != nil {
		return err
	}
	fnd, err := findings.Project(ctx, s, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := authority.Project(ctx, s); err != nil {
		return err
	}
	fmt.Printf("findings   %d open (%d new)\n", fnd.Open, fnd.New)
	return nil
}
