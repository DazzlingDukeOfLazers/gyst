package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// RelationRow mirrors the relations table. PolicyVersion is nil for a
// relation derived from source-native evidence rather than a profile.
type RelationRow struct {
	PolicyVersion *string
	RelationID    string
	Type          string
	FromSource    string
	FromLocator   string
	ToSource      string
	ToLocator     string
	Precedence    string
	ActorKind     string
	ActorID       string
	Evidence      []string
	Confidence    float64
	Explanation   string
	AssertedAt    time.Time
}

const insertRelation = `
	INSERT INTO relations (identity_policy_version, relation_id, type,
		from_source, from_locator, to_source, to_locator,
		precedence, actor_kind, actor_id, evidence, confidence, explanation)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (relation_id) DO NOTHING`

// InsertRelations adds relations, ignoring ones already present.
func (s *Store) InsertRelations(ctx context.Context, rels []RelationRow) error {
	if len(rels) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, r := range rels {
		if err := execRelation(ctx, tx, r); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func execRelation(ctx context.Context, tx pgx.Tx, r RelationRow) error {
	_, err := tx.Exec(ctx, insertRelation, r.PolicyVersion, r.RelationID, r.Type,
		r.FromSource, r.FromLocator, r.ToSource, r.ToLocator,
		r.Precedence, r.ActorKind, r.ActorID, r.Evidence, r.Confidence, r.Explanation)
	return err
}

func scanRelations(rows pgx.Rows) ([]RelationRow, error) {
	defer rows.Close()
	var out []RelationRow
	for rows.Next() {
		var r RelationRow
		if err := rows.Scan(&r.RelationID, &r.Type, &r.FromSource, &r.FromLocator, &r.ToSource, &r.ToLocator,
			&r.Precedence, &r.ActorKind, &r.ActorID, &r.Evidence, &r.Confidence, &r.Explanation,
			&r.AssertedAt, &r.PolicyVersion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const selectRelation = `
	SELECT relation_id, type, from_source, from_locator, to_source, to_locator,
	       precedence, actor_kind, actor_id, evidence, confidence, explanation, asserted_at,
	       identity_policy_version FROM relations `

// AllRelations lists every relation, in a stable order.
func (s *Store) AllRelations(ctx context.Context) ([]RelationRow, error) {
	rows, err := s.pool.Query(ctx, selectRelation+`ORDER BY type, from_source, from_locator, to_locator`)
	if err != nil {
		return nil, err
	}
	return scanRelations(rows)
}

// RelationsOf lists relations touching a locator under a policy version or
// derived from native evidence.
func (s *Store) RelationsOf(ctx context.Context, policyVersion, sourceID, locator string) ([]RelationRow, error) {
	rows, err := s.pool.Query(ctx, selectRelation+`
		WHERE (identity_policy_version IS NULL OR identity_policy_version=$1)
		  AND ((from_source=$2 AND from_locator=$3) OR (to_source=$2 AND to_locator=$3))
		ORDER BY type, to_locator`, policyVersion, sourceID, locator)
	if err != nil {
		return nil, err
	}
	return scanRelations(rows)
}

// CommitTouch is a commit that changed a file, reachable through a
// contains relation.
type CommitTouch struct {
	OID        string
	Author     string
	Message    string
	AuthoredAt time.Time
	Evidence   []string
}

// GitHistoryOf lists the commits whose contains relations point at a file.
func (s *Store) GitHistoryOf(ctx context.Context, sourceID, locator string) ([]CommitTouch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.oid, c.author, c.message, c.authored_at, r.evidence
		FROM relations r
		JOIN commits c ON (c.source_id || '@' || c.oid) = r.from_locator AND c.source_id = r.from_source
		WHERE r.type='contains' AND r.to_source=$1 AND r.to_locator=$2
		ORDER BY c.authored_at DESC, c.seq DESC`, sourceID, locator)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommitTouch
	for rows.Next() {
		var t CommitTouch
		if err := rows.Scan(&t.OID, &t.Author, &t.Message, &t.AuthoredAt, &t.Evidence); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GoneFile is a tombstoned locator with the content it last had.
type GoneFile struct {
	SourceID, Locator, ObsID, Pass, Digest string
	Size                                   int64
}

// Tombstones lists disappearances with the digest recovered from the
// observation each tombstone superseded, via last_known_seq.
func (s *Store) Tombstones(ctx context.Context) ([]GoneFile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.source_id, t.locator, t.observation_id, t.observed_at::text,
		       coalesce(prior.content_digest_hex, ''), coalesce(prior.size_bytes, 0)
		FROM observations t
		LEFT JOIN observations prior ON prior.seq = (t.claim_payload->>'last_known_seq')::bigint
		WHERE t.claim_type = 'artifact.absent' ORDER BY t.seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GoneFile
	for rows.Next() {
		var g GoneFile
		if err := rows.Scan(&g.SourceID, &g.Locator, &g.ObsID, &g.Pass, &g.Digest, &g.Size); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Arrivals lists the first observation of every locator: a file that merely
// changed is not an arrival.
func (s *Store) Arrivals(ctx context.Context) ([]GoneFile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.source_id, o.locator, o.observation_id, o.observed_at::text,
		       coalesce(o.content_digest_hex,''), coalesce(o.size_bytes,0)
		FROM observations o
		WHERE o.subject_kind = 'file' AND o.claim_type <> 'artifact.absent'
		  AND o.seq = (SELECT min(first.seq) FROM observations first
		               WHERE first.source_id = o.source_id AND first.locator = o.locator
		                 AND first.claim_type <> 'artifact.absent')
		ORDER BY o.seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GoneFile
	for rows.Next() {
		var a GoneFile
		if err := rows.Scan(&a.SourceID, &a.Locator, &a.ObsID, &a.Pass, &a.Digest, &a.Size); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AmbiguityRow is one compare-set-with relation whose gone side is still
// absent and whose candidate is still present.
type AmbiguityRow struct {
	GoneSource, GoneLocator, GoneNativeVersion, GoneObsID             string
	CandSource, CandLocator, CandNativeVersion, CandDigest, CandObsID string
	CandSize                                                          int64
	Evidence                                                          []string
	Confidence                                                        float64
}

// Ambiguities lists live compare-set-with relations, grouped by gone file
// through the ordering.
func (s *Store) Ambiguities(ctx context.Context) ([]AmbiguityRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.to_source, r.to_locator, t.native_version_value, t.observation_id,
		       r.from_source, r.from_locator, cf.native_version_value, coalesce(cf.size_bytes,0),
		       coalesce(cf.content_digest_hex,''), o.observation_id, r.evidence, r.confidence
		FROM relations r
		JOIN current_files gone ON gone.source_id=r.to_source AND gone.locator=r.to_locator AND NOT gone.present
		JOIN observations t ON t.seq = gone.latest_seq
		JOIN current_files cf ON cf.source_id=r.from_source AND cf.locator=r.from_locator AND cf.present
		JOIN observations o ON o.seq = cf.latest_seq
		WHERE r.type = 'compare-set-with'
		ORDER BY r.to_source, r.to_locator, r.from_source, r.from_locator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AmbiguityRow
	for rows.Next() {
		var a AmbiguityRow
		if err := rows.Scan(&a.GoneSource, &a.GoneLocator, &a.GoneNativeVersion, &a.GoneObsID,
			&a.CandSource, &a.CandLocator, &a.CandNativeVersion, &a.CandSize, &a.CandDigest, &a.CandObsID,
			&a.Evidence, &a.Confidence); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
