package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ActivePolicy returns the active identity policy version and profile name,
// or empty strings when none is active.
func (s *Store) ActivePolicy(ctx context.Context) (version, profile string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT version, profile FROM identity_policies WHERE active`).Scan(&version, &profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return version, profile, err
}

// ArtifactRow is a grouping under a policy.
type ArtifactRow struct {
	ArtifactID  string
	SourceID    string
	GroupingKey string
	MemberCount int
	Confidence  float64
}

// ArtifactMemberRow is one locator's place in a grouping.
type ArtifactMemberRow struct {
	ArtifactID   string
	SourceID     string
	Locator      string
	LatestSeq    int64
	VersionLabel *string
	IsCurrent    bool
	Rule         string
	Confidence   float64
	Explanation  string
}

// ApplyIdentityPlan writes a grouping under its policy version and makes
// it active. Previously active policies are deactivated but retained, so a
// release that pinned an earlier interpretation can still resolve it. The
// version's rows are rebuilt from scratch; cascades clear members and
// relations, so a rerun cannot leave stale rows behind.
func (s *Store) ApplyIdentityPlan(ctx context.Context, version, profile string,
	artifacts []ArtifactRow, members []ArtifactMemberRow, relations []RelationRow) error {

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE identity_policies SET active=FALSE WHERE active`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity_policies (version, profile, active) VALUES ($1,$2,TRUE)
		ON CONFLICT (version) DO UPDATE SET profile=EXCLUDED.profile, active=TRUE`, version, profile); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM artifacts WHERE identity_policy_version=$1`, version); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM relations WHERE identity_policy_version=$1`, version); err != nil {
		return err
	}
	for _, a := range artifacts {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifacts (identity_policy_version, artifact_id, source_id, grouping_key, member_count, confidence)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			version, a.ArtifactID, a.SourceID, a.GroupingKey, a.MemberCount, a.Confidence); err != nil {
			return err
		}
	}
	for _, m := range members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_members (identity_policy_version, artifact_id, source_id,
				locator, latest_seq, version_label, is_current, rule, confidence, explanation)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			version, m.ArtifactID, m.SourceID, m.Locator, m.LatestSeq,
			m.VersionLabel, m.IsCurrent, m.Rule, m.Confidence, m.Explanation); err != nil {
			return err
		}
	}
	for _, r := range relations {
		v := version
		r.PolicyVersion = &v
		if _, err := tx.Exec(ctx, `
			INSERT INTO relations (identity_policy_version, relation_id, type,
				from_source, from_locator, to_source, to_locator,
				precedence, actor_kind, actor_id, evidence, confidence, explanation)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			r.PolicyVersion, r.RelationID, r.Type, r.FromSource, r.FromLocator, r.ToSource, r.ToLocator,
			r.Precedence, r.ActorKind, r.ActorID, r.Evidence, r.Confidence, r.Explanation); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// AmbiguousArtifacts lists multi-member groupings, least confident first.
func (s *Store) AmbiguousArtifacts(ctx context.Context, version string) ([]ArtifactRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, source_id, grouping_key, member_count, confidence
		FROM artifacts WHERE identity_policy_version=$1 AND member_count > 1
		ORDER BY confidence, grouping_key`, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactRow
	for rows.Next() {
		var a ArtifactRow
		if err := rows.Scan(&a.ArtifactID, &a.SourceID, &a.GroupingKey, &a.MemberCount, &a.Confidence); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Artifacts lists every grouping under a policy.
func (s *Store) Artifacts(ctx context.Context, version string) ([]ArtifactRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, source_id, grouping_key, member_count, confidence
		FROM artifacts WHERE identity_policy_version=$1 ORDER BY source_id, grouping_key`, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactRow
	for rows.Next() {
		var a ArtifactRow
		if err := rows.Scan(&a.ArtifactID, &a.SourceID, &a.GroupingKey, &a.MemberCount, &a.Confidence); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ArtifactMembers lists every membership under a policy, by artifact then
// locator.
func (s *Store) ArtifactMembers(ctx context.Context, version string) ([]ArtifactMemberRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, source_id, locator, latest_seq, version_label, is_current, rule, confidence, explanation
		FROM artifact_members WHERE identity_policy_version=$1 ORDER BY artifact_id, locator`, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactMemberRow
	for rows.Next() {
		var m ArtifactMemberRow
		if err := rows.Scan(&m.ArtifactID, &m.SourceID, &m.Locator, &m.LatestSeq, &m.VersionLabel,
			&m.IsCurrent, &m.Rule, &m.Confidence, &m.Explanation); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MembershipOf returns a locator's grouping under a policy, with the
// artifact's grouping key, or ErrNotFound.
func (s *Store) MembershipOf(ctx context.Context, version, sourceID, locator string) (ArtifactMemberRow, string, error) {
	var m ArtifactMemberRow
	var key string
	err := s.pool.QueryRow(ctx, `
		SELECT m.artifact_id, a.grouping_key, m.rule, m.explanation, m.version_label, m.is_current, m.confidence
		FROM artifact_members m
		JOIN artifacts a ON a.identity_policy_version=m.identity_policy_version AND a.artifact_id=m.artifact_id
		WHERE m.identity_policy_version=$1 AND m.source_id=$2 AND m.locator=$3`, version, sourceID, locator).
		Scan(&m.ArtifactID, &key, &m.Rule, &m.Explanation, &m.VersionLabel, &m.IsCurrent, &m.Confidence)
	m.SourceID, m.Locator = sourceID, locator
	return m, key, notFound(err)
}

// Siblings lists the other members of a locator's artifact.
func (s *Store) Siblings(ctx context.Context, version, artifactID, locator string) ([]ArtifactMemberRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT locator, version_label, is_current, confidence
		FROM artifact_members WHERE identity_policy_version=$1 AND artifact_id=$2 AND locator <> $3
		ORDER BY locator`, version, artifactID, locator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactMemberRow
	for rows.Next() {
		var m ArtifactMemberRow
		if err := rows.Scan(&m.Locator, &m.VersionLabel, &m.IsCurrent, &m.Confidence); err != nil {
			return nil, err
		}
		m.ArtifactID = artifactID
		out = append(out, m)
	}
	return out, rows.Err()
}
