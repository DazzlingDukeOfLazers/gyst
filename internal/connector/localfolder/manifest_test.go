package localfolder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DazzlingDukeOfLazers/gyst/internal/observe"
)

func manifestObs(res *Result) []observe.Observation {
	var out []observe.Observation
	for _, o := range res.Observations {
		if o.Claim.Type == "project.manifest" {
			out = append(out, o)
		}
	}
	return out
}

// A manifest is evidence about the project, not just a file. An incremental
// pass that skips it as unchanged must still produce that evidence, or a
// project declared before the projector learned about manifests never
// appears.
func TestManifestObservedOnIncrementalPass(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "boards", ".gyst"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "boards", ".gyst", "project.yaml"),
		[]byte("name: Boards\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := timeAgo(t, dir, filepath.Join("boards", ".gyst", "project.yaml"))

	first, err := Discover(Options{Root: dir, SourceID: "src", Now: past})
	if err != nil {
		t.Fatal(err)
	}
	m1 := manifestObs(first)
	if len(m1) != 1 {
		t.Fatalf("first pass produced %d manifest observations", len(m1))
	}
	if m1[0].Claim.Payload["id"] != "boards" || m1[0].Claim.Payload["valid"] != true {
		t.Errorf("payload %v", m1[0].Claim.Payload)
	}
	if ms := m1[0].Claim.Payload["members"].([]string); len(ms) != 1 || ms[0] != "**" {
		t.Errorf("members %v; the manifest's own folder is the default", ms)
	}

	// Second pass: everything is known and unchanged.
	known := map[string]observe.KnownState{}
	for _, o := range first.Observations {
		if o.Claim.Type == "file.content_fingerprint" {
			known[o.Subject.Location.Locator] = observe.KnownState{
				NativeVersion: o.Subject.Location.NativeVersion.Value, Seq: 7}
		}
	}
	second, err := Discover(Options{Root: dir, SourceID: "src", Now: past, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 0 || second.Unchanged != 1 {
		t.Errorf("second pass scanned %d unchanged %d; expected an incremental no-op for the file itself",
			second.Scanned, second.Unchanged)
	}
	m2 := manifestObs(second)
	if len(m2) != 1 {
		t.Fatalf("incremental pass produced %d manifest observations, want 1", len(m2))
	}
	// Same file, same version, same prior: same id, so the log deduplicates.
	m3, _ := Discover(Options{Root: dir, SourceID: "src", Now: past, Known: known})
	if manifestObs(m3)[0].ObservationID != m2[0].ObservationID {
		t.Error("re-observing an unchanged manifest derived a different id; the log would grow on every pass")
	}
}

func TestMarkerFolderObserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fw", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fw", "main.c"), []byte("int main(){}"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := timeAgo(t, dir, filepath.Join("fw", "main.c"))
	res, err := Discover(Options{Root: dir, SourceID: "src", Now: past})
	if err != nil {
		t.Fatal(err)
	}
	var folders []observe.Observation
	for _, o := range res.Observations {
		if o.Subject.Kind == "folder" {
			folders = append(folders, o)
		}
	}
	if len(folders) != 1 || folders[0].Subject.Location.Locator != "fw" {
		t.Fatalf("folder observations %+v", folders)
	}
	if ms := folders[0].Claim.Payload["markers"].([]string); len(ms) != 1 || ms[0] != "git" {
		t.Errorf("markers %v", ms)
	}
	if res.MarkedFolders != 1 {
		t.Errorf("MarkedFolders = %d", res.MarkedFolders)
	}
	for _, o := range res.Observations {
		if o.Subject.Location.Locator == "fw/.git" || filepath.Base(o.Subject.Location.Locator) == "HEAD" {
			t.Errorf("walked into .git: %s", o.Subject.Location.Locator)
		}
	}
}
