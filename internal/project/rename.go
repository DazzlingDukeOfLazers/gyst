package project

import (
	"context"
	"fmt"
	"sort"

	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// EmptySHA256 is the digest of zero bytes. Every empty file in existence shares
// it, so it carries no identifying information whatsoever.
const EmptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// maxCandidates bounds how many possible origins are offered for one ambiguous
// arrival. Beyond a handful the list stops being a question a person can answer.
const maxCandidates = 4

// Gone is a file that was present and is now tombstoned.
type Gone struct {
	SourceID string
	Locator  string
	Digest   string
	Size     int64
	ObsID    string
	Pass     string // observed_at of the scan that noticed the absence
}

// Arrived is a file observed for the first time at a locator.
type Arrived struct {
	SourceID string
	Locator  string
	Digest   string
	Size     int64
	ObsID    string
	Pass     string
}

// RenameCall is what detection concluded about one disappearance.
type RenameCall struct {
	From       Gone
	To         Arrived
	Type       string // renamed-from, or compare-set-with when ambiguous
	Confidence float64
	Reason     string
}

// RenameReport summarises a detection pass.
type RenameReport struct {
	Calls []RenameCall

	Renamed   int
	Ambiguous int
	Unmatched int
	SkippedNoContent int
	Truncated int
}

// DetectRenames pairs disappearances with arrivals by content.
//
// A rename leaves no trace of itself: the filesystem reports a file gone and
// another present, and only the content connects them. Everything here is
// therefore inference, and the interesting question is when to refuse.
//
// The rule is that content must actually identify something. Three cases where
// it does not:
//
//   - Empty files. Every zero-byte file shares one digest, so a directory of
//     them yields every possible pairing and none of them mean anything.
//   - Duplicated content. If two files already hold identical bytes, deleting
//     one and adding a third elsewhere gives no basis for choosing which moved.
//   - Many-to-many. Several files gone and several arrived with one digest is a
//     reorganisation, not a rename, and picking pairs would be invention.
//
// Only a one-to-one match on a digest unique among both sides is called a
// rename. Ambiguous cases still produce output -- low-confidence
// compare-set-with candidates a person can resolve -- because silently dropping
// them would lose the fact that something moved at all.
//
// Matching is scoped to a single scan pass. A file deleted in March and an
// identical one created in July are not a rename in any useful sense.
func DetectRenames(gone []Gone, arrived []Arrived) RenameReport {
	var rep RenameReport

	// Index by (pass, source, digest). A rename cannot cross sources: locators
	// in different sources are rooted differently and a match would be a
	// coincidence of content, not a move.
	type bucket struct {
		gone    []Gone
		arrived []Arrived
	}
	buckets := map[string]*bucket{}
	key := func(pass, source, digest string) string {
		return pass + "\x00" + source + "\x00" + digest
	}

	for _, g := range gone {
		if g.Digest == "" || g.Digest == EmptySHA256 || g.Size == 0 {
			rep.SkippedNoContent++
			continue
		}
		k := key(g.Pass, g.SourceID, g.Digest)
		if buckets[k] == nil {
			buckets[k] = &bucket{}
		}
		buckets[k].gone = append(buckets[k].gone, g)
	}
	for _, a := range arrived {
		if a.Digest == "" || a.Digest == EmptySHA256 || a.Size == 0 {
			continue
		}
		k := key(a.Pass, a.SourceID, a.Digest)
		if buckets[k] == nil {
			continue // nothing disappeared with this content; an arrival is just an arrival
		}
		buckets[k].arrived = append(buckets[k].arrived, a)
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		b := buckets[k]
		sort.Slice(b.gone, func(i, j int) bool { return b.gone[i].Locator < b.gone[j].Locator })
		sort.Slice(b.arrived, func(i, j int) bool { return b.arrived[i].Locator < b.arrived[j].Locator })

		switch {
		case len(b.arrived) == 0:
			// Gone and nothing took its place. A plain deletion.
			rep.Unmatched += len(b.gone)

		case len(b.gone) == 1 && len(b.arrived) == 1:
			g, a := b.gone[0], b.arrived[0]
			rep.Calls = append(rep.Calls, RenameCall{
				From: g, To: a, Type: "renamed-from", Confidence: 0.95,
				Reason: fmt.Sprintf(
					"%q disappeared and %q appeared in the same pass with identical content "+
						"(sha256 %s, %d bytes), and no other file on either side shares it",
					g.Locator, a.Locator, short12(a.Digest), a.Size),
			})
			rep.Renamed++

		default:
			// Ambiguous. Offer candidates rather than choosing one.
			for _, g := range b.gone {
				shown := b.arrived
				if len(shown) > maxCandidates {
					shown = shown[:maxCandidates]
					rep.Truncated += len(b.arrived) - maxCandidates
				}
				for _, a := range shown {
					rep.Calls = append(rep.Calls, RenameCall{
						From: g, To: a, Type: "compare-set-with", Confidence: 0.3,
						Reason: fmt.Sprintf(
							"%d file(s) disappeared and %d appeared with identical content "+
								"(sha256 %s); which moved where cannot be determined from content alone, "+
								"so %q is offered as one possible origin of %q",
							len(b.gone), len(b.arrived), short12(a.Digest), g.Locator, a.Locator),
					})
				}
				rep.Ambiguous++
			}
		}
	}
	return rep
}

// LoadRenameCandidates reads disappearances and arrivals from the log.
//
// A disappearance is a tombstone. Its content digest is not on the tombstone
// itself -- a vanished file has none -- and is recovered from the observation
// it superseded, via the last_known_seq recorded in its payload.
//
// An arrival is the first observation of a locator: one with no earlier
// observation for the same source and locator. A file that merely changed is
// not an arrival, so an edit cannot be mistaken for the destination of a move.
//
// Both sides are keyed by observed_at, which every observation in one scan pass
// shares, so matching stays within a pass.
func LoadRenameCandidates(ctx context.Context, s *store.Store) ([]Gone, []Arrived, error) {
	goneRows, err := s.Pool().Query(ctx, `
		SELECT t.source_id, t.locator, t.observation_id, t.observed_at::text,
		       coalesce(prior.content_digest_hex, ''), coalesce(prior.size_bytes, 0)
		FROM observations t
		LEFT JOIN observations prior
		       ON prior.seq = (t.claim_payload->>'last_known_seq')::bigint
		WHERE t.claim_type = 'artifact.absent'
		ORDER BY t.seq`)
	if err != nil {
		return nil, nil, err
	}
	var gone []Gone
	for goneRows.Next() {
		var g Gone
		if err := goneRows.Scan(&g.SourceID, &g.Locator, &g.ObsID, &g.Pass, &g.Digest, &g.Size); err != nil {
			goneRows.Close()
			return nil, nil, err
		}
		gone = append(gone, g)
	}
	goneRows.Close()
	if err := goneRows.Err(); err != nil {
		return nil, nil, err
	}
	if len(gone) == 0 {
		return nil, nil, nil
	}

	arrivedRows, err := s.Pool().Query(ctx, `
		SELECT o.source_id, o.locator, o.observation_id, o.observed_at::text,
		       coalesce(o.content_digest_hex,''), coalesce(o.size_bytes,0)
		FROM observations o
		WHERE o.subject_kind = 'file'
		  AND o.claim_type <> 'artifact.absent'
		  AND o.seq = (
		        SELECT min(first.seq) FROM observations first
		        WHERE first.source_id = o.source_id AND first.locator = o.locator
		          AND first.claim_type <> 'artifact.absent')
		ORDER BY o.seq`)
	if err != nil {
		return nil, nil, err
	}
	defer arrivedRows.Close()

	var arrived []Arrived
	for arrivedRows.Next() {
		var a Arrived
		if err := arrivedRows.Scan(&a.SourceID, &a.Locator, &a.ObsID, &a.Pass, &a.Digest, &a.Size); err != nil {
			return nil, nil, err
		}
		arrived = append(arrived, a)
	}
	return gone, arrived, arrivedRows.Err()
}

// ProjectRenames detects renames and stores them as relations.
//
// The relations carry no identity policy version: a rename is evidence about
// what happened in the source, not an interpretation imposed by a profile.
func ProjectRenames(ctx context.Context, s *store.Store) (RenameReport, error) {
	gone, arrived, err := LoadRenameCandidates(ctx, s)
	if err != nil {
		return RenameReport{}, err
	}
	rep := DetectRenames(gone, arrived)
	if len(rep.Calls) == 0 {
		return rep, nil
	}

	tx, err := s.Pool().Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer tx.Rollback(ctx)

	for _, c := range rep.Calls {
		relID := "rel_" + shortHash(c.Type, c.To.SourceID+"\x00"+c.To.Locator,
			c.From.SourceID+"\x00"+c.From.Locator, c.To.Pass)
		if _, err := tx.Exec(ctx, `
			INSERT INTO relations (identity_policy_version, relation_id, type,
				from_source, from_locator, to_source, to_locator,
				precedence, actor_kind, actor_id, evidence, confidence, explanation)
			VALUES (NULL,$1,$2,$3,$4,$5,$6,'gyst_suggestion','suggestion','rename-detector/1',$7,$8,$9)
			ON CONFLICT (relation_id) DO NOTHING`,
			relID, c.Type,
			// from is the new locator, to is where it came from: "B renamed-from A".
			c.To.SourceID, c.To.Locator, c.From.SourceID, c.From.Locator,
			[]string{c.To.ObsID, c.From.ObsID}, c.Confidence, c.Reason); err != nil {
			return rep, err
		}
	}
	return rep, tx.Commit(ctx)
}
