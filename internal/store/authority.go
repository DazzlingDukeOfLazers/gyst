package store

import (
	"context"
	"time"
)

// AssertionRow mirrors the assertions table.
type AssertionRow struct {
	AssertionID   string
	Kind          string
	SourceID      string
	Locator       string
	ActorID       string
	Reason        string
	Evidence      []string
	AssertedAt    time.Time
	RetractedAt   *time.Time
	RetractedBy   *string
	RetractReason *string
}

// InsertAssertion records a person's statement. Only the user actor kind
// exists in this table; the check constraint enforces it.
func (s *Store) InsertAssertion(ctx context.Context, a AssertionRow) error {
	_, err := s.db.exec(ctx, `
		INSERT INTO assertions (assertion_id, kind, source_id, locator, actor_kind, actor_id, reason, evidence, asserted_at)
		VALUES ($1,$2,$3,$4,'user',$5,$6,$7,$8)`,
		a.AssertionID, a.Kind, a.SourceID, a.Locator, a.ActorID, a.Reason, a.Evidence, a.AssertedAt)
	return err
}

// RetractAssertion records a retraction on an active assertion. Returns
// false when there was no active assertion with that id.
func (s *Store) RetractAssertion(ctx context.Context, id, by, reason string) (bool, error) {
	tag, err := s.db.exec(ctx, `
		UPDATE assertions SET retracted_at=$4, retracted_by=$2, retract_reason=$3
		WHERE assertion_id=$1 AND retracted_at IS NULL`, id, by, reason, time.Now().UTC())
	return affected(tag) > 0, err
}

// ListAssertions returns assertions in the order made; with all=false,
// only active ones.
func (s *Store) ListAssertions(ctx context.Context, all bool) ([]AssertionRow, error) {
	rows, err := s.db.query(ctx, `
		SELECT assertion_id, kind, source_id, locator, actor_id, reason, evidence,
		       asserted_at, retracted_at, retracted_by, retract_reason
		FROM assertions WHERE $1 OR retracted_at IS NULL ORDER BY asserted_at`, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssertionRow
	for rows.Next() {
		var r AssertionRow
		if err := rows.Scan(&r.AssertionID, &r.Kind, &r.SourceID, &r.Locator, &r.ActorID, &r.Reason,
			jsl(&r.Evidence), ts(&r.AssertedAt), tsp(&r.RetractedAt), &r.RetractedBy, &r.RetractReason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FileAuthorityRow mirrors the file_authority projection.
type FileAuthorityRow struct {
	SourceID, Locator string
	State, Basis      string
	AuthoritySource   *string
	AuthorityLocator  *string
	Confidence        float64
	Evidence          []string
	AssertionID       *string
	Explanation       string
}

// ReplaceFileAuthority rebuilds the projection in one transaction.
func (s *Store) ReplaceFileAuthority(ctx context.Context, rows []FileAuthorityRow) error {
	tx, err := s.db.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.exec(ctx, `DELETE FROM file_authority`); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.exec(ctx, `INSERT INTO file_authority (source_id, locator, state, basis, authority_source,
			authority_locator, confidence, evidence, assertion_id, explanation)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			r.SourceID, r.Locator, r.State, r.Basis, r.AuthoritySource, r.AuthorityLocator,
			r.Confidence, r.Evidence, r.AssertionID, r.Explanation); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const selectAuthority = `
	SELECT source_id, locator, state, basis, authority_source, authority_locator,
	       confidence, evidence, assertion_id, explanation FROM file_authority `

// FileAuthority returns one file's resolved state, or ErrNotFound.
func (s *Store) FileAuthority(ctx context.Context, sourceID, locator string) (FileAuthorityRow, error) {
	var r FileAuthorityRow
	err := s.db.queryRow(ctx, selectAuthority+`WHERE source_id=$1 AND locator=$2`, sourceID, locator).
		Scan(&r.SourceID, &r.Locator, &r.State, &r.Basis, &r.AuthoritySource, &r.AuthorityLocator,
			&r.Confidence, jsl(&r.Evidence), &r.AssertionID, &r.Explanation)
	return r, notFound(err)
}

// AllFileAuthority returns every file's resolved state.
func (s *Store) AllFileAuthority(ctx context.Context) ([]FileAuthorityRow, error) {
	rows, err := s.db.query(ctx, selectAuthority+`ORDER BY source_id, locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FileAuthorityRow
	for rows.Next() {
		var r FileAuthorityRow
		if err := rows.Scan(&r.SourceID, &r.Locator, &r.State, &r.Basis, &r.AuthoritySource, &r.AuthorityLocator,
			&r.Confidence, jsl(&r.Evidence), &r.AssertionID, &r.Explanation); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
