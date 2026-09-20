package bundle

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

func obsWith(egress string) observe.Observation {
	o := observe.Observation{
		SchemaVersion: observe.SchemaVersion, ObservedAt: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
		Source: observe.Source{SourceID: "eng", Connector: "local-folder", ConnectorVersion: "0.1.0"},
		Subject: observe.ArtifactRef{Kind: "file", Location: observe.Location{SourceID: "eng", Locator: "a/b.pdf",
			NativeVersion: observe.NativeVersion{Scheme: "mtime_size", Value: "1:2"}}, Version: &observe.Version{SizeBytes: 2}},
		Claim:      observe.Claim{Type: "file.metadata", Payload: map[string]any{"extension": ".pdf"}},
		Extractor:  observe.Extractor{Name: "t", Version: "1", OutputSchema: "t/1", Warnings: []string{}, Confidence: 1},
		Policy:     observe.Policy{ContentLevel: "metadata", Egress: egress, EffectivePolicyVersion: "p"},
		Visibility: observe.Visibility{Labels: []string{"src:eng:read"}, SourceACLComplete: true},
	}
	o.ObservationID = observe.DeriveID(&o, 0)
	return o
}

func TestRoundTrip(t *testing.T) {
	k, err := Generate(t.TempDir(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := NewWriter(k, "facility", []Source{{SourceID: "eng", Kind: "local-folder", Root: "/eng"}})
	if err := w.Add(obsWith("facility")); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(obsWith("device")); err != nil { // must be withheld
		t.Fatal(err)
	}
	var buf bytes.Buffer
	h, err := w.Finish(&buf, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if h.Counts.Observations != 1 || h.Counts.Withheld != 1 {
		t.Fatalf("counts %+v; a device-only observation must not leave", h.Counts)
	}
	if !strings.HasPrefix(buf.String(), `{"schema":"gyst.bundle/0.1.0"`) || strings.Count(buf.String(), "\n") != 2 {
		t.Errorf("shape: %.80s", buf.String())
	}
	h2, obs, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if h2.BundleID != h.BundleID || len(obs) != 1 || obs[0].Subject.Location.Locator != "a/b.pdf" || h2.Sender.PublicKey != k.PublicBase64() {
		t.Errorf("read back %+v %d", h2, len(obs))
	}
	ns := Namespace(obs[0], "alice")
	if ns.Source.SourceID != "alice/eng" || ns.Subject.Location.SourceID != "alice/eng" || ns.Visibility.Labels[0] != "bundle:alice" || ns.ObservationID != obs[0].ObservationID {
		t.Errorf("namespace %+v", ns)
	}
}

func TestTamperIsRefused(t *testing.T) {
	k, _ := Generate(t.TempDir(), "alice")
	w, _ := NewWriter(k, "facility", nil)
	_ = w.Add(obsWith("facility"))
	var buf bytes.Buffer
	if _, err := w.Finish(&buf, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Change one byte of the body.
	tampered := bytes.Replace(buf.Bytes(), []byte(`"a/b.pdf"`), []byte(`"a/c.pdf"`), 1)
	if _, _, err := Read(bytes.NewReader(tampered)); !errors.Is(err, ErrBadDigest) {
		t.Errorf("body tamper: %v", err)
	}
	// Recompute the digest but leave the signature: the signature must fail.
	lines := bytes.SplitN(tampered, []byte("\n"), 2)
	var h Header
	_ = jsonUnmarshal(lines[0], &h)
	h.BodySHA256 = sha256Hex(lines[1])
	head, _ := jsonMarshal(h)
	resigned := append(append(head, '\n'), lines[1]...)
	if _, _, err := Read(bytes.NewReader(resigned)); !errors.Is(err, ErrBadSignature) {
		t.Errorf("header tamper: %v", err)
	}
	// A different key cannot sign as alice.
	other, _ := Generate(t.TempDir(), "mallory")
	w2, _ := NewWriter(Key{ID: "alice", Public: k.Public, private: other.private}, "facility", nil)
	_ = w2.Add(obsWith("facility"))
	var buf2 bytes.Buffer
	_, _ = w2.Finish(&buf2, time.Now())
	if _, _, err := Read(bytes.NewReader(buf2.Bytes())); !errors.Is(err, ErrBadSignature) {
		t.Errorf("wrong key: %v", err)
	}
}

func TestEgressOrder(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"device", "device", true}, {"device", "facility", false}, {"facility", "device", true},
		{"facility", "facility", true}, {"facility", "connected-server", false}, {"connected-server", "facility", true},
		{"", "device", false}, {"device", "elsewhere", false},
	}
	for _, c := range cases {
		if Permits(c.have, c.want) != c.ok {
			t.Errorf("Permits(%q, %q) != %v", c.have, c.want, c.ok)
		}
	}
}

func TestKeysDoNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := Generate(dir, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir, "alice"); err == nil {
		t.Fatal("overwrote an existing key")
	}
	k, err := Load(dir, "alice")
	if err != nil || k.ID != "alice" {
		t.Fatal(err)
	}
}
