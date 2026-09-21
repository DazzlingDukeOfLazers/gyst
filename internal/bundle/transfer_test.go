package bundle

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/location"
	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
	"github.com/DazzlingDukeOfLazers/gyst/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenDSN(context.Background(), "sqlite:"+filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Two Gysts. Alice observes and exports at facility egress; Bob imports.
func TestExportImportBetweenStores(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)
	alice := newStore(t)
	loc := location.Location{Kind: location.KindNetworkShare, Provider: "smb", Evidence: "test", Confidence: 0.9}
	if err := alice.RegisterSource(ctx, "eng", "local-folder", "/eng", loc); err != nil {
		t.Fatal(err)
	}
	o1 := obsWith("facility")
	o2 := obsWith("device")
	o2.Subject.Location.Locator = "secret/x.pdf"
	o2.ObservationID = observe.DeriveID(&o2, 0)
	if _, err := alice.Append(ctx, []observe.Observation{o1, o2}); err != nil {
		t.Fatal(err)
	}
	key, _ := Generate(t.TempDir(), "alice")
	var buf bytes.Buffer
	ex, err := Export(ctx, alice, key, "facility", nil, &buf, now)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Written != 1 || ex.Withheld != 1 || ex.Sources != 1 {
		t.Fatalf("export %+v; the device-only observation must stay home", ex)
	}

	bob := newStore(t)
	if _, err := Import(ctx, bob, bytes.NewReader(buf.Bytes()), ImportOptions{}, now); !errors.Is(err, ErrUnknownSender) {
		t.Fatalf("unknown sender accepted: %v", err)
	}
	im, err := Import(ctx, bob, bytes.NewReader(buf.Bytes()), ImportOptions{TrustOnFirstUse: true, By: "bob"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if im.Appended != 1 || im.Sources != 1 || !im.NewlyTrust {
		t.Fatalf("import %+v", im)
	}
	if _, err := bob.ApplyCurrentFiles(ctx); err != nil {
		t.Fatal(err)
	}
	files, _ := bob.PresentFiles(ctx)
	if len(files) != 1 || files[0].SourceID != "alice/eng" || files[0].Locator != "a/b.pdf" || files[0].ObsID != o1.ObservationID {
		t.Fatalf("bob sees %+v", files)
	}
	srcs, _ := bob.Sources(ctx)
	if len(srcs) != 1 || srcs[0].SourceID != "alice/eng" || srcs[0].ImportedFrom != "alice" || srcs[0].Location.Provider != "smb" {
		t.Fatalf("bob's sources %+v", srcs)
	}
	if rows, _ := bob.ObservationsOf(ctx, "alice/eng", "a/b.pdf"); len(rows) != 1 {
		t.Fatal("observation not under the namespaced source")
	}
	// Twice is once.
	im2, err := Import(ctx, bob, bytes.NewReader(buf.Bytes()), ImportOptions{}, now.Add(time.Hour))
	if err != nil || im2.Appended != 0 || im2.Known != 1 {
		t.Fatalf("re-import %+v %v", im2, err)
	}
	if bs, _ := bob.Bundles(ctx); len(bs) != 1 {
		t.Errorf("bundles recorded %d", len(bs))
	}
	// A different key claiming to be alice is refused.
	mallory, _ := Generate(t.TempDir(), "alice")
	var buf2 bytes.Buffer
	if _, err := Export(ctx, alice, mallory, "facility", nil, &buf2, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(ctx, bob, bytes.NewReader(buf2.Bytes()), ImportOptions{TrustOnFirstUse: true}, now); !errors.Is(err, ErrKeyChanged) {
		t.Fatalf("key change accepted: %v", err)
	}
	// Bob cannot forward alice's evidence as his own.
	bobKey, _ := Generate(t.TempDir(), "bob")
	var buf3 bytes.Buffer
	ex3, err := Export(ctx, bob, bobKey, "facility", nil, &buf3, now)
	if err != nil || ex3.Sources != 0 || ex3.Written != 0 {
		t.Fatalf("imported sources were re-exported: %+v %v", ex3, err)
	}
	if _, err := Export(ctx, bob, bobKey, "facility", []string{"alice/eng"}, &buf3, now); err == nil {
		t.Fatal("naming an imported source for export was allowed")
	}
}
