package localfolder

import (
	"io/fs"
	"syscall"
	"testing"
	"time"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

// fakeInfo stands in for a stat result on a file that does not exist on
// disk, which is what a dataless placeholder effectively is.
type fakeInfo struct {
	name  string
	size  int64
	flags uint32
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return 0o644 }
func (f fakeInfo) ModTime() time.Time { return time.Now().Add(-time.Hour) }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return &syscall.Stat_t{Flags: f.flags} }

func TestDatalessFlagIsPlaceholder(t *testing.T) {
	if !isPlaceholder(fakeInfo{flags: sfDataless}) {
		t.Fatal("SF_DATALESS not recognised")
	}
	if isPlaceholder(fakeInfo{flags: 0}) {
		t.Fatal("ordinary file reported as placeholder")
	}
}

// The file in this test does not exist. If observeFile tries to hash a
// placeholder it will fail on open, so a nil error here proves the content
// was never touched.
func TestPlaceholderIsObservedWithoutReading(t *testing.T) {
	info := fakeInfo{name: "board.kicad_pcb", size: 4096, flags: sfDataless}
	obs, placeholder, err := observeFile("/nonexistent/board.kicad_pcb", "board.kicad_pcb",
		info, nativeVersion(info), 0,
		Options{SourceID: "src", ContentLevel: observe.ContentFingerprint, Egress: "device"}, time.Now())
	if err != nil {
		t.Fatalf("placeholder was opened: %v", err)
	}
	if !placeholder {
		t.Fatal("not reported as placeholder")
	}
	if obs.Subject.Version.ContentDigest != nil {
		t.Fatal("placeholder carries a content digest; nothing was read, so nothing can be hashed")
	}
	if obs.Claim.Type != "file.metadata" {
		t.Errorf("claim type %q, want file.metadata", obs.Claim.Type)
	}
	if obs.Claim.Payload["placeholder"] != true {
		t.Error("payload does not say placeholder")
	}
	if len(obs.Extractor.Warnings) == 0 {
		t.Error("no warning explaining why content is absent")
	}
	if obs.Subject.Version.SizeBytes != 4096 {
		t.Errorf("size %d; the logical size is still known and worth keeping", obs.Subject.Version.SizeBytes)
	}
}
