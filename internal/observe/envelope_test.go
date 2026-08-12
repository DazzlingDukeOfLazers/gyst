package observe

import (
	"encoding/json"
	"testing"
	"time"
)

func sample(locator, nativeVersion string) *Observation {
	return &Observation{
		SchemaVersion: SchemaVersion,
		ObservedAt:    time.Date(2024, 8, 12, 9, 14, 3, 0, time.UTC),
		Source: Source{
			SourceID:         "src_test",
			Connector:        "local-folder",
			ConnectorVersion: "0.1.0",
		},
		Subject: ArtifactRef{
			Kind: "file",
			Location: Location{
				SourceID: "src_test",
				Locator:  locator,
				NativeVersion: NativeVersion{
					Scheme: "mtime_size",
					Value:  nativeVersion,
				},
			},
			Version: &Version{SizeBytes: 42},
		},
		Claim:     Claim{Type: "file.content_fingerprint", Payload: map[string]any{}},
		Extractor: Extractor{Name: "file-fingerprint", Version: "0.1.0", Warnings: []string{}, Confidence: 1},
		Policy:    Policy{ContentLevel: ContentFingerprint, Egress: "device", EffectivePolicyVersion: "pol_test"},
	}
}

// An unchanged file re-observed must derive the same id, or an idle tree grows
// the log on every scan.
func TestDeriveIDStableAcrossRescan(t *testing.T) {
	a := DeriveID(sample("a.txt", "100:42"), 0)
	b := DeriveID(sample("a.txt", "100:42"), 0)
	if a != b {
		t.Fatalf("id not stable for identical state: %s != %s", a, b)
	}
}

// Regression. A file edited and then restored to a byte-identical earlier
// state -- same mtime, same size, exactly what a checkout or backup restore
// reproduces -- used to re-derive its original id. The insert collided, nothing
// was appended, and the projection kept asserting the intermediate content
// forever.
//
// priorSeq is what separates the two observations: same bytes, different
// history, therefore a different observation.
func TestDeriveIDDistinguishesRevertToEarlierState(t *testing.T) {
	original := DeriveID(sample("a.txt", "100:42"), 0)  // first sighting
	reverted := DeriveID(sample("a.txt", "100:42"), 77) // same state, after an edit at seq 77

	if original == reverted {
		t.Fatal("a revert to a byte-identical earlier state derived the same id; " +
			"the transition would be swallowed and the projection would go stale")
	}
}

// A changed file must always produce a new id regardless of prior state.
func TestDeriveIDChangesWithNativeVersion(t *testing.T) {
	before := DeriveID(sample("a.txt", "100:42"), 0)
	after := DeriveID(sample("a.txt", "200:99"), 0)
	if before == after {
		t.Fatal("changed native version derived the same id")
	}
}

// A new extractor version re-reading the same bytes is a new claim about them,
// not a duplicate.
func TestDeriveIDChangesWithExtractorVersion(t *testing.T) {
	o := sample("a.txt", "100:42")
	first := DeriveID(o, 0)
	o.Extractor.Version = "0.2.0"
	if second := DeriveID(o, 0); first == second {
		t.Fatal("extractor version does not affect the id; a re-extraction would be dropped")
	}
}

func TestPermitsDigest(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  bool
	}{
		{ContentExclude, false},
		{ContentMetadata, false},
		{ContentFingerprint, true},
		{ContentExtractLocal, true},
		{ContentFull, true},
	} {
		if got := PermitsDigest(tc.level); got != tc.want {
			t.Errorf("PermitsDigest(%q) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

// The envelope must not serialise a logical artifact grouping. The JSON Schema
// rejects one; this catches a struct field added on the Go side that would
// produce a record the schema refuses.
func TestObservationCarriesNoArtifactGrouping(t *testing.T) {
	blob, err := json.Marshal(sample("a.txt", "100:42"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(blob, &decoded); err != nil {
		t.Fatal(err)
	}
	subject, ok := decoded["subject"].(map[string]any)
	if !ok {
		t.Fatal("subject missing from serialised observation")
	}
	if _, present := subject["artifact"]; present {
		t.Fatal("observation serialised an artifact grouping; identity is a projection " +
			"and must never appear on evidence")
	}
}

func TestNormalizeLocatorUsesForwardSlashes(t *testing.T) {
	if got := NormalizeLocator(`engineering\widget\a.pdf`); got != "engineering/widget/a.pdf" {
		t.Fatalf("got %q", got)
	}
}
