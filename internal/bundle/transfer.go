package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

// ExportStats reports what an export wrote and withheld.
type ExportStats struct {
	Header   Header
	Sources  int
	Written  int
	Withheld int
}

// Export writes a bundle of the named sources' observations, or of every
// local source when none are named. Sources that were themselves imported
// are never re-exported: a bundle carries what this Gyst observed, and
// forwarding another sender's evidence under one's own signature would
// misattribute it.
func Export(ctx context.Context, s *store.Store, key Key, egress string, sourceIDs []string, out io.Writer, now time.Time) (ExportStats, error) {
	var st ExportStats
	all, err := s.Sources(ctx)
	if err != nil {
		return st, err
	}
	want := map[string]bool{}
	for _, id := range sourceIDs {
		want[id] = true
	}
	var sources []Source
	for _, src := range all {
		if len(want) > 0 && !want[src.SourceID] {
			continue
		}
		if src.ImportedFrom != "" {
			continue
		}
		sources = append(sources, Source{SourceID: src.SourceID, Kind: src.Kind, Root: src.Root, Location: src.Location})
	}
	for id := range want {
		found := false
		for _, src := range sources {
			found = found || src.SourceID == id
		}
		if !found {
			return st, fmt.Errorf("source %q is not a local source", id)
		}
	}
	w, err := NewWriter(key, egress, sources)
	if err != nil {
		return st, err
	}
	for _, src := range sources {
		if _, err := s.Observations(ctx, src.SourceID, 0, w.Add); err != nil {
			return st, err
		}
	}
	h, err := w.Finish(out, now)
	if err != nil {
		return st, err
	}
	st.Header, st.Sources, st.Written, st.Withheld = h, len(sources), w.Counts().Observations, w.Counts().Withheld
	return st, nil
}

// ImportOptions govern trust.
type ImportOptions struct {
	// TrustOnFirstUse records an unknown sender's key rather than refusing.
	TrustOnFirstUse bool
	// By is the person importing, recorded on any key trusted here.
	By string
}

// ImportStats reports what an import did.
type ImportStats struct {
	Header     Header
	Sources    int
	Appended   int
	Known      int
	NewlyTrust bool
}

var (
	ErrUnknownSender = errors.New("sender is not trusted; import with --trust-on-first-use to record their key, or gyst trust add")
	ErrKeyChanged    = errors.New("sender's key differs from the trusted one; refusing")
)

// Import verifies a bundle, checks its sender against the trusted keys,
// registers its sources under the sender's namespace, relabels and
// appends its observations, and records the receipt.
func Import(ctx context.Context, s *store.Store, r io.Reader, opts ImportOptions, now time.Time) (ImportStats, error) {
	var st ImportStats
	h, obs, err := Read(r)
	if err != nil {
		return st, err
	}
	st.Header = h
	known, err := s.TrustedKeyOf(ctx, h.Sender.ID)
	switch {
	case err == nil:
		if known.PublicKey != h.Sender.PublicKey {
			return st, ErrKeyChanged
		}
	case errors.Is(err, store.ErrNotFound):
		if !opts.TrustOnFirstUse {
			return st, ErrUnknownSender
		}
		if err := s.TrustKey(ctx, store.TrustedKey{SenderID: h.Sender.ID, PublicKey: h.Sender.PublicKey,
			AddedAt: now, AddedBy: opts.By, Note: "trusted on first use from bundle " + h.BundleID}); err != nil {
			return st, err
		}
		st.NewlyTrust = true
	default:
		return st, err
	}

	for _, src := range h.Sources {
		id := h.Sender.ID + "/" + src.SourceID
		if err := s.RegisterSource(ctx, id, src.Kind, src.Root, src.Location); err != nil {
			return st, err
		}
		if err := s.MarkImported(ctx, id, h.Sender.ID); err != nil {
			return st, err
		}
		st.Sources++
	}
	batch := make([]observe.Observation, 0, len(obs))
	for _, o := range obs {
		batch = append(batch, Namespace(o, h.Sender.ID))
	}
	n, err := s.Append(ctx, batch)
	if err != nil {
		return st, err
	}
	st.Appended, st.Known = n, len(batch)-n
	srcs, _ := json.Marshal(h.Sources)
	if err := s.RecordBundle(ctx, store.BundleRow{
		BundleID: h.BundleID, SenderID: h.Sender.ID, CreatedAt: h.CreatedAt, ReceivedAt: now,
		Egress: h.Egress, BodySHA256: h.BodySHA256, Observations: len(obs), Appended: n, Sources: srcs,
	}); err != nil {
		return st, err
	}
	return st, nil
}
