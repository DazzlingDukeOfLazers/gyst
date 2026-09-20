package authority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/identity"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
	"github.com/jackc/pgx/v5"
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

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return st, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM file_authority`); err != nil {
		return st, err
	}
	batch := &pgx.Batch{}
	for _, f := range in.Files {
		a := res[f.Key]
		var ofSrc, ofLoc *string
		if a.Of != nil {
			ofSrc, ofLoc = &a.Of.SourceID, &a.Of.Locator
		}
		var astID *string
		if a.AssertionID != "" {
			astID = &a.AssertionID
		}
		batch.Queue(`INSERT INTO file_authority (source_id, locator, state, basis, authority_source,
			authority_locator, confidence, evidence, assertion_id, explanation)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			f.SourceID, f.Locator, a.State, a.Basis, ofSrc, ofLoc, a.Confidence, a.Evidence, astID, a.Explanation)
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
	r := tx.SendBatch(ctx, batch)
	for range in.Files {
		if _, err := r.Exec(); err != nil {
			r.Close()
			return st, err
		}
	}
	if err := r.Close(); err != nil {
		return st, err
	}
	return st, tx.Commit(ctx)
}

func load(ctx context.Context, s *store.Store) (Input, error) {
	var in Input
	rows, err := s.Pool().Query(ctx, `
		SELECT cf.source_id, cf.locator, coalesce(cf.content_digest_hex,''), o.observation_id
		FROM current_files cf JOIN observations o ON o.seq=cf.latest_seq
		WHERE cf.present ORDER BY cf.source_id, cf.locator`)
	if err != nil {
		return in, err
	}
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.SourceID, &f.Locator, &f.Digest, &f.ObsID); err != nil {
			rows.Close()
			return in, err
		}
		in.Files = append(in.Files, f)
	}
	rows.Close()

	policy, _, err := identity.ActivePolicy(ctx, s)
	if err != nil {
		return in, err
	}
	if policy != "" {
		rows, err = s.Pool().Query(ctx, `
			SELECT artifact_id, source_id, locator, is_current, confidence, rule
			FROM artifact_members WHERE identity_policy_version=$1
			ORDER BY artifact_id, locator`, policy)
		if err != nil {
			return in, err
		}
		var cur *Group
		for rows.Next() {
			var id string
			var m Member
			if err := rows.Scan(&id, &m.SourceID, &m.Locator, &m.IsCurrent, &m.Confidence, &m.Rule); err != nil {
				rows.Close()
				return in, err
			}
			if cur == nil || cur.ArtifactID != id {
				in.Groups = append(in.Groups, Group{ArtifactID: id})
				cur = &in.Groups[len(in.Groups)-1]
			}
			cur.Members = append(cur.Members, m)
		}
		rows.Close()
	}

	rows, err = s.Pool().Query(ctx, `
		SELECT assertion_id, kind, source_id, locator, actor_id, reason, evidence
		FROM assertions WHERE retracted_at IS NULL ORDER BY asserted_at`)
	if err != nil {
		return in, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Assertion
		if err := rows.Scan(&a.ID, &a.Kind, &a.Subject.SourceID, &a.Subject.Locator, &a.ActorID, &a.Reason, &a.Evidence); err != nil {
			return in, err
		}
		in.Assertions = append(in.Assertions, a)
	}
	return in, rows.Err()
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
	var obsID string
	err := s.Pool().QueryRow(ctx, `
		SELECT o.observation_id FROM current_files cf JOIN observations o ON o.seq=cf.latest_seq
		WHERE cf.source_id=$1 AND cf.locator=$2 AND cf.present`, subject.SourceID, subject.Locator).Scan(&obsID)
	if err != nil {
		return "", fmt.Errorf("%s is not a present file: %w", subject, err)
	}
	now := time.Now().UTC()
	h := sha256.Sum256([]byte(kind + "\x00" + subject.String() + "\x00" + by + "\x00" + now.Format(time.RFC3339Nano)))
	id := "ast_" + hex.EncodeToString(h[:])[:24]
	_, err = s.Pool().Exec(ctx, `
		INSERT INTO assertions (assertion_id, kind, source_id, locator, actor_kind, actor_id, reason, evidence, asserted_at)
		VALUES ($1,$2,$3,$4,'user',$5,$6,$7,$8)`,
		id, kind, subject.SourceID, subject.Locator, by, reason, []string{obsID}, now)
	return id, err
}

// Retract records that an assertion no longer stands. The row remains.
func Retract(ctx context.Context, s *store.Store, id, by, reason string) error {
	if by == "" || reason == "" {
		return fmt.Errorf("a retraction needs who (--by) and why (--reason)")
	}
	tag, err := s.Pool().Exec(ctx, `
		UPDATE assertions SET retracted_at=now(), retracted_by=$2, retract_reason=$3
		WHERE assertion_id=$1 AND retracted_at IS NULL`, id, by, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
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
	rows, err := s.Pool().Query(ctx, `
		SELECT assertion_id, kind, source_id, locator, actor_id, reason, evidence,
		       asserted_at, retracted_at, retracted_by, retract_reason
		FROM assertions WHERE $1 OR retracted_at IS NULL ORDER BY asserted_at`, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Kind, &r.Subject.SourceID, &r.Subject.Locator, &r.ActorID, &r.Reason,
			&r.Evidence, &r.AssertedAt, &r.RetractedAt, &r.RetractedBy, &r.RetractReason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Of returns the resolved authority for one file.
func Of(ctx context.Context, s *store.Store, key Key) (*Authority, error) {
	var a Authority
	var ofSrc, ofLoc, astID *string
	err := s.Pool().QueryRow(ctx, `
		SELECT state, basis, authority_source, authority_locator, confidence, evidence, assertion_id, explanation
		FROM file_authority WHERE source_id=$1 AND locator=$2`, key.SourceID, key.Locator).
		Scan(&a.State, &a.Basis, &ofSrc, &ofLoc, &a.Confidence, &a.Evidence, &astID, &a.Explanation)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ofSrc != nil {
		a.Of = &Key{*ofSrc, *ofLoc}
	}
	if astID != nil {
		a.AssertionID = *astID
	}
	return &a, nil
}
