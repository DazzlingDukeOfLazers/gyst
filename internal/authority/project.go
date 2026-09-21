package authority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// Stats counts files by resolved state.
type Stats struct{ Declared, Likely, Multiple, None int }

// Project rebuilds file_authority from assertions, the active identity
// policy's groups, and the current inventory.
func Project(ctx context.Context, s *store.Store) (Stats, error) {
	var st Stats
	in, err := load(ctx, s)
	if err != nil {
		return st, err
	}
	res := Resolve(in)

	rows := make([]store.FileAuthorityRow, 0, len(in.Files))
	for _, f := range in.Files {
		a := res[f.Key]
		row := store.FileAuthorityRow{
			SourceID: f.SourceID, Locator: f.Locator, State: a.State, Basis: a.Basis,
			Confidence: a.Confidence, Evidence: a.Evidence, Explanation: a.Explanation,
		}
		if a.Of != nil {
			src, loc := a.Of.SourceID, a.Of.Locator
			row.AuthoritySource, row.AuthorityLocator = &src, &loc
		}
		if a.AssertionID != "" {
			id := a.AssertionID
			row.AssertionID = &id
		}
		rows = append(rows, row)
		switch a.State {
		case StateDeclared:
			st.Declared++
		case StateLikely:
			st.Likely++
		case StateMultiple:
			st.Multiple++
		default:
			st.None++
		}
	}
	return st, s.ReplaceFileAuthority(ctx, rows)
}

func load(ctx context.Context, s *store.Store) (Input, error) {
	var in Input
	present, err := s.PresentFiles(ctx)
	if err != nil {
		return in, err
	}
	for _, f := range present {
		in.Files = append(in.Files, File{Key: Key{f.SourceID, f.Locator}, Digest: f.Digest, ObsID: f.ObsID})
	}

	policy, _, err := s.ActivePolicy(ctx)
	if err != nil {
		return in, err
	}
	if policy != "" {
		members, err := s.ArtifactMembers(ctx, policy)
		if err != nil {
			return in, err
		}
		var cur *Group
		for _, m := range members {
			if cur == nil || cur.ArtifactID != m.ArtifactID {
				in.Groups = append(in.Groups, Group{ArtifactID: m.ArtifactID})
				cur = &in.Groups[len(in.Groups)-1]
			}
			cur.Members = append(cur.Members, Member{Key: Key{m.SourceID, m.Locator},
				IsCurrent: m.IsCurrent, Confidence: m.Confidence, Rule: m.Rule})
		}
	}

	asts, err := s.ListAssertions(ctx, false)
	if err != nil {
		return in, err
	}
	for _, a := range asts {
		if a.SubjectKind != "file" {
			continue
		}
		in.Assertions = append(in.Assertions, Assertion{ID: a.AssertionID, Kind: a.Kind,
			Subject: Key{a.SourceID, a.Locator}, ActorID: a.ActorID, Reason: a.Reason, Evidence: a.Evidence})
	}
	return in, nil
}

// Assert records a person's statement about a file. The subject must be a
// present file; the assertion cites its latest observation, which is what
// the person was looking at.
func Assert(ctx context.Context, s *store.Store, kind string, subject Key, by, reason string) (string, error) {
	if kind != KindAuthority && kind != KindNotAuthority {
		return "", fmt.Errorf("unknown assertion kind %q", kind)
	}
	if by == "" || reason == "" {
		return "", fmt.Errorf("an assertion needs who (--by) and why (--reason)")
	}
	f, err := s.PresentFileAt(ctx, subject.SourceID, subject.Locator)
	if err != nil {
		return "", fmt.Errorf("%s is not a present file: %w", subject, err)
	}
	now := time.Now().UTC()
	h := sha256.Sum256([]byte(kind + "\x00" + subject.String() + "\x00" + by + "\x00" + now.Format(time.RFC3339Nano)))
	id := "ast_" + hex.EncodeToString(h[:])[:24]
	return id, s.InsertAssertion(ctx, store.AssertionRow{
		AssertionID: id, Kind: kind, SourceID: subject.SourceID, Locator: subject.Locator,
		ActorID: by, Reason: reason, Evidence: []string{f.ObsID}, AssertedAt: now,
	})
}

// Retract records that an assertion no longer stands. The row remains.
func Retract(ctx context.Context, s *store.Store, id, by, reason string) error {
	if by == "" || reason == "" {
		return fmt.Errorf("a retraction needs who (--by) and why (--reason)")
	}
	ok, err := s.RetractAssertion(ctx, id, by, reason)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no active assertion %s", id)
	}
	return nil
}

// Row is an assertion as read back.
type Row struct {
	Assertion
	AssertedAt    time.Time
	RetractedAt   *time.Time
	RetractedBy   *string
	RetractReason *string
}

func List(ctx context.Context, s *store.Store, all bool) ([]Row, error) {
	rows, err := s.ListAssertions(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, Row{
			Assertion: Assertion{ID: r.AssertionID, Kind: r.Kind, SubjectKind: r.SubjectKind, Subject: Key{r.SourceID, r.Locator},
				Object: r.Object, Value: r.Value, ActorID: r.ActorID, Reason: r.Reason, Evidence: r.Evidence},
			AssertedAt: r.AssertedAt, RetractedAt: r.RetractedAt, RetractedBy: r.RetractedBy, RetractReason: r.RetractReason,
		})
	}
	return out, nil
}

// FromRow converts a stored projection row.
func FromRow(r store.FileAuthorityRow) Authority {
	a := Authority{State: r.State, Basis: r.Basis, Confidence: r.Confidence, Evidence: r.Evidence, Explanation: r.Explanation}
	if r.AuthoritySource != nil {
		a.Of = &Key{*r.AuthoritySource, *r.AuthorityLocator}
	}
	if r.AssertionID != nil {
		a.AssertionID = *r.AssertionID
	}
	return a
}

// Of returns the resolved authority for one file, or nil when the
// projection has not run for it.
func Of(ctx context.Context, s *store.Store, key Key) (*Authority, error) {
	r, err := s.FileAuthority(ctx, key.SourceID, key.Locator)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a := FromRow(r)
	return &a, nil
}
