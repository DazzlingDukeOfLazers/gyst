// Package git observes a Git repository through its own history rather than by
// walking the working tree.
//
// The distinction matters: the local-folder connector sees bytes at a path with
// an mtime, while Git knows the commit that put them there, who wrote it, and
// what changed alongside. Both observations of the same file are valid and must
// reconcile rather than compete.
//
// This connector shells out to git. It never writes: no checkout, no fetch, no
// config change, and every command is a read.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

const (
	ConnectorName    = "git"
	ConnectorVersion = "0.1.0"

	// ASCII record and unit separators. Not NUL: an argv string cannot contain
	// one, so a NUL field separator fails at exec with "invalid argument"
	// rather than anywhere useful.
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

type Options struct {
	Repo          string
	SourceID      string
	Ref           string
	PolicyVersion string

	// Cursor is the last commit already observed. Discovery resumes after it.
	Cursor string

	MaxCommits int
}

type Result struct {
	Observations []observe.Observation
	NextCursor   string
	Commits      int
	Ref          string
	Head         string
}

// Discover reads commits in ancestry order, oldest first, and emits one
// observation per commit.
//
// Unlike a file, a commit is immutable: its object id is a digest of its own
// content, so re-observing one always yields identical state. That is why the
// prior-seq argument to DeriveID is zero here -- there is no such thing as a
// commit reverting to an earlier version of itself.
func Discover(opts Options) (*Result, error) {
	if opts.Ref == "" {
		opts.Ref = "HEAD"
	}
	if opts.MaxCommits == 0 {
		opts.MaxCommits = 10_000
	}

	head, err := run(opts.Repo, "rev-parse", opts.Ref)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", opts.Ref, err)
	}
	head = strings.TrimSpace(head)

	args := []string{
		"log", "--reverse", "--no-merges",
		"--format=" + recordSep + "%H" + fieldSep + "%P" + fieldSep +
			"%an <%ae>" + fieldSep + "%aI" + fieldSep + "%s",
		"--name-only",
		fmt.Sprintf("--max-count=%d", opts.MaxCommits),
	}
	if opts.Cursor != "" {
		// Everything reachable from the ref but not from the cursor.
		args = append(args, opts.Cursor+".."+opts.Ref)
	} else {
		args = append(args, opts.Ref)
	}

	out, err := run(opts.Repo, args...)
	if err != nil {
		return nil, err
	}

	res := &Result{Ref: opts.Ref, Head: head, NextCursor: opts.Cursor}
	now := time.Now().UTC()

	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}
		obs, oid, err := parseRecord(record, opts, now)
		if err != nil {
			return nil, err
		}
		res.Observations = append(res.Observations, obs)
		res.NextCursor = oid
		res.Commits++
	}
	return res, nil
}

func parseRecord(record string, opts Options, now time.Time) (observe.Observation, string, error) {
	// The header fields come first, NUL-separated; --name-only appends the
	// changed paths on the lines that follow.
	head, rest, _ := strings.Cut(record, "\n")
	fields := strings.Split(head, fieldSep)
	if len(fields) < 5 {
		return observe.Observation{}, "", fmt.Errorf("malformed git record: %q", head)
	}
	oid, parentList, author, authoredAt, subject := fields[0], fields[1], fields[2], fields[3], fields[4]

	var parents []string
	if strings.TrimSpace(parentList) != "" {
		parents = strings.Fields(parentList)
	}
	var changed []string
	for _, line := range strings.Split(rest, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			changed = append(changed, observe.NormalizeLocator(line))
		}
	}

	when, err := time.Parse(time.RFC3339, authoredAt)
	if err != nil {
		when = now
	}

	obs := observe.Observation{
		SchemaVersion: observe.SchemaVersion,
		ObservedAt:    now,
		Source: observe.Source{
			SourceID:         opts.SourceID,
			Connector:        ConnectorName,
			ConnectorVersion: ConnectorVersion,
			Cursor:           opts.Ref + "@" + oid,
		},
		Subject: observe.ArtifactRef{
			Kind: "commit",
			Location: observe.Location{
				SourceID: opts.SourceID,
				Locator:  opts.SourceID + "@" + oid,
				NativeVersion: observe.NativeVersion{
					Scheme: "git_oid",
					Value:  oid,
				},
			},
		},
		Claim: observe.Claim{
			Type: "git.commit",
			Payload: map[string]any{
				"message":       subject,
				"author":        author,
				"authored_at":   when.Format(time.RFC3339),
				"parents":       parents,
				"changed_paths": changed,
			},
		},
		Extractor: observe.Extractor{
			Name:         "git-history",
			Version:      ConnectorVersion,
			OutputSchema: "gyst.claim.git.commit/0.1.0",
			Warnings:     []string{},
			Confidence:   1.0,
		},
		Policy: observe.Policy{
			// Commit metadata is read locally and is not file content, but it
			// still carries names, messages, and paths. It travels under the
			// same policy as anything else.
			ContentLevel:           observe.ContentExtractLocal,
			Egress:                 "device",
			EffectivePolicyVersion: opts.PolicyVersion,
		},
		Visibility: observe.Visibility{
			Labels:            []string{"src:" + opts.SourceID + ":read"},
			SourceACLComplete: true,
		},
	}
	obs.ObservationID = observe.DeriveID(&obs, 0)
	return obs, oid, nil
}

// IsRepo reports whether path contains a Git repository.
func IsRepo(path string) bool {
	out, err := run(path, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

func run(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	// Ignore the user's global config so observation is not perturbed by local
	// settings, and never prompt for credentials.
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
