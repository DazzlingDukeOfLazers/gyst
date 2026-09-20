// Package authority answers the product's second question, "where is the
// authority?", for every file, and is honest when the answer is nobody
// knows.
//
// Three kinds of evidence, in strict precedence:
//
//  1. A person's assertion that a file is, or is not, the authority.
//  2. The active identity profile's current member of a version group,
//     when the profile is confident.
//  3. Byte-identical copies, which can only say there are several.
//
// A manifest declaring project membership is not evidence of authority and
// is not consulted. The resolver is a pure function so every rule is
// testable without a database.
package authority

import (
	"fmt"
	"sort"
	"strings"
)

const (
	StateDeclared = "declared"
	StateLikely   = "likely"
	StateMultiple = "multiple"
	StateNone     = "none"

	BasisExplicit = "explicit"
	BasisProfile  = "identity-profile"

	KindAuthority    = "authority"
	KindNotAuthority = "not-authority"

	// A profile's current member counts as likely authority only when the
	// profile itself is confident. Below this a version series is a
	// compare-set, and a compare-set has candidates, not a likely answer.
	likelyThreshold = 0.8

	// maxNamedPeers bounds how many candidates a "multiple" state names
	// and cites. Beyond a handful the list stops being a question a person
	// can answer, and a thousand identical files would otherwise each
	// carry a thousand-entry record: the cost is quadratic and was
	// measured at a gigabyte for a hundred thousand files.
	maxNamedPeers = 5
)

type Key struct {
	SourceID string `json:"source_id"`
	Locator  string `json:"locator"`
}

func (k Key) String() string { return k.SourceID + ":" + k.Locator }

type File struct {
	Key
	Digest string
	ObsID  string
}

type Member struct {
	Key
	IsCurrent  bool
	Confidence float64
	Rule       string
}

type Group struct {
	ArtifactID string
	Members    []Member
}

type Assertion struct {
	ID       string
	Kind     string
	Subject  Key
	ActorID  string
	Reason   string
	Evidence []string
}

type Input struct {
	Files      []File
	Groups     []Group
	Assertions []Assertion // active only; retracted ones are not evidence
}

// Authority is the resolved state for one file.
type Authority struct {
	State       string   `json:"state"`
	Basis       string   `json:"basis"`
	Of          *Key     `json:"authority,omitempty"`
	Confidence  float64  `json:"confidence"`
	Evidence    []string `json:"evidence"`
	AssertionID string   `json:"assertion_id,omitempty"`
	Explanation string   `json:"explanation"`
}

// Resolve computes the authority state of every file.
func Resolve(in Input) map[Key]Authority {
	byDigest := map[string][]Key{}
	fileObs := map[Key]string{}
	for _, f := range in.Files {
		fileObs[f.Key] = f.ObsID
		if f.Digest != "" {
			byDigest[f.Digest] = append(byDigest[f.Digest], f.Key)
		}
	}
	// Sort each digest group once. Sorting the peer set again for every
	// member of a five-thousand-file group cost nineteen seconds at a
	// hundred thousand files; sorted once and merged per file, it is
	// linear.
	for _, ks := range byDigest {
		sortKeys(ks)
	}
	groupOf := map[Key]*Group{}
	groupSorted := map[*Group][]Key{}
	for i := range in.Groups {
		g := &in.Groups[i]
		ks := make([]Key, 0, len(g.Members))
		for _, m := range g.Members {
			groupOf[m.Key] = g
			ks = append(ks, m.Key)
		}
		sortKeys(ks)
		groupSorted[g] = ks
	}
	asserted := map[Key]Assertion{} // active authority assertion per file
	denied := map[Key]Assertion{}   // active not-authority per file
	for _, a := range in.Assertions {
		switch a.Kind {
		case KindAuthority:
			asserted[a.Subject] = a
		case KindNotAuthority:
			denied[a.Subject] = a
		}
	}

	out := make(map[Key]Authority, len(in.Files))
	for _, f := range in.Files {
		// Peers: everything that could be the same thing as f, by the
		// profile's grouping or by content. Two notions of sameness, both
		// consulted; each list is already sorted, so the union is a merge.
		var groupKeys []Key
		if g := groupOf[f.Key]; g != nil {
			groupKeys = groupSorted[g]
		}
		sorted := mergeKeys([]Key{f.Key}, groupKeys, byDigest[f.Digest])
		out[f.Key] = resolveOne(f, sorted, groupOf, asserted, denied, fileObs)
	}
	return out
}

func less(a, b Key) bool {
	if a.SourceID != b.SourceID {
		return a.SourceID < b.SourceID
	}
	return a.Locator < b.Locator
}

func sortKeys(ks []Key) { sort.Slice(ks, func(i, j int) bool { return less(ks[i], ks[j]) }) }

// mergeKeys unions sorted key lists, dropping duplicates.
func mergeKeys(lists ...[]Key) []Key {
	n := 0
	for _, l := range lists {
		n += len(l)
	}
	out := make([]Key, 0, n)
	idx := make([]int, len(lists))
	for {
		best := -1
		for i, l := range lists {
			if idx[i] < len(l) && (best < 0 || less(l[idx[i]], lists[best][idx[best]])) {
				best = i
			}
		}
		if best < 0 {
			return out
		}
		k := lists[best][idx[best]]
		idx[best]++
		if len(out) == 0 || out[len(out)-1] != k {
			out = append(out, k)
		}
	}
}

func resolveOne(f File, sorted []Key, groupOf map[Key]*Group,
	asserted, denied map[Key]Assertion, fileObs map[Key]string) Authority {

	// 1. Declared by a person.
	var declared []Assertion
	for _, k := range sorted {
		if a, ok := asserted[k]; ok {
			if _, no := denied[k]; !no {
				declared = append(declared, a)
			}
		}
	}
	switch {
	case len(declared) == 1:
		a := declared[0]
		of := a.Subject
		return Authority{State: StateDeclared, Basis: BasisExplicit, Of: &of, Confidence: 1.0,
			Evidence: a.Evidence, AssertionID: a.ID,
			Explanation: fmt.Sprintf("%s asserted %s is the authority: %s", a.ActorID, a.Subject.Locator, a.Reason)}
	case len(declared) > 1:
		names := make([]string, 0, len(declared))
		ev := []string{}
		for _, a := range declared {
			names = append(names, a.Subject.Locator+" ("+a.ActorID+")")
			ev = append(ev, a.Evidence...)
		}
		return Authority{State: StateMultiple, Basis: BasisExplicit, Confidence: 1.0, Evidence: ev,
			Explanation: "more than one file is asserted to be the authority, by different people or at different times: " +
				strings.Join(names, ", ") + ". One must be retracted."}
	}

	// 2. The profile's current member, if the profile is confident and
	// nobody has said otherwise.
	if g := groupOf[f.Key]; g != nil && len(g.Members) > 1 {
		for _, m := range g.Members {
			if m.IsCurrent && m.Confidence >= likelyThreshold {
				if _, no := denied[m.Key]; no {
					break
				}
				of := m.Key
				return Authority{State: StateLikely, Basis: BasisProfile, Of: &of, Confidence: m.Confidence,
					Evidence: evidenceFor(g.Members, fileObs),
					Explanation: fmt.Sprintf("the active identity profile (%s) treats %s as the current version of a %d-member series; inferred, not declared",
						m.Rule, m.Key.Locator, len(g.Members))}
			}
		}
	}

	// 3. Several candidates, nothing to choose between them. Peers in
	// another source are named with it, or the same path three times over
	// reads as one file listed thrice.
	if len(sorted) > 1 {
		names := make([]string, 0, len(sorted))
		var live []Key
		for _, k := range sorted {
			if _, no := denied[k]; no {
				continue
			}
			live = append(live, k)
			if len(names) >= maxNamedPeers {
				continue
			}
			if k.SourceID == f.SourceID {
				names = append(names, k.Locator)
			} else {
				names = append(names, k.String())
			}
		}
		if more := len(live) - len(names); more > 0 {
			names = append(names, fmt.Sprintf("and %d more", more))
		}
		if len(live) == 1 {
			// Everything else was ruled out by a person. The remaining one
			// is not thereby declared, but it is no longer ambiguous.
			return Authority{State: StateNone, Basis: "", Confidence: 0, Evidence: evidenceKeys(cited(f.Key, sorted), fileObs),
				Explanation: fmt.Sprintf("every other copy was asserted not to be the authority; %s remains but has not been declared",
					live[0].Locator)}
		}
		return Authority{State: StateMultiple, Basis: "", Confidence: 0, Evidence: evidenceKeys(cited(f.Key, live), fileObs),
			Explanation: fmt.Sprintf("%d files could each be the authority and nothing distinguishes them: %s",
				len(live), strings.Join(names, ", "))}
	}

	return Authority{State: StateNone, Basis: "", Confidence: 0, Evidence: []string{fileObs[f.Key]},
		Explanation: "the only copy observed, and nothing declared"}
}

func evidenceFor(ms []Member, fileObs map[Key]string) []string {
	var out []string
	for _, m := range ms {
		if o := fileObs[m.Key]; o != "" {
			out = append(out, o)
		}
	}
	return out
}

func evidenceKeys(ks []Key, fileObs map[Key]string) []string {
	var out []string
	for _, k := range ks {
		if o := fileObs[k]; o != "" {
			out = append(out, o)
		}
	}
	return out
}

// cited is the file itself plus the first few peers: the evidence a
// person would look at, not the whole group.
func cited(self Key, peers []Key) []Key {
	out := []Key{self}
	for _, k := range peers {
		if k == self {
			continue
		}
		if len(out) > maxNamedPeers {
			break
		}
		out = append(out, k)
	}
	return out
}
