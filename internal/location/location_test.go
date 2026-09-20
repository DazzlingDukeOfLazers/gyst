package location

import (
	"path/filepath"
	"testing"
)

func home(parts ...string) string {
	return filepath.Join(append([]string{filepath.FromSlash("/Users/dee")}, parts...)...)
}

func TestNetworkFilesystemsAreShares(t *testing.T) {
	for fs, prov := range map[string]string{
		"smbfs": "smb", "cifs": "smb", "afpfs": "afp", "nfs": "nfs", "webdav": "webdav", "fuse.sshfs": "sshfs",
	} {
		l := Classify(filepath.FromSlash("/Volumes/eng/boards"), Env{FSType: fs, Mount: filepath.FromSlash("/Volumes/eng")})
		if l.Kind != KindNetworkShare || l.Provider != prov {
			t.Errorf("%s: got %s, want network-share/%s", fs, l, prov)
		}
	}
}

func TestOrdinaryVolumeIsLocal(t *testing.T) {
	l := Classify(home("src"), Env{Home: home(), FSType: "apfs", Mount: "/"})
	if l.Kind != KindLocal || l.Provider != "apfs" {
		t.Fatalf("got %s, want local/apfs", l)
	}
}

func TestUnknownWhenNothingIsKnown(t *testing.T) {
	l := Classify(home("src"), Env{})
	if l.Kind != KindUnknown || l.Confidence != 0 {
		t.Fatalf("no evidence classified as %s at %.2f; unknown is the honest answer", l, l.Confidence)
	}
}

func TestFuseIsNotAssumedLocal(t *testing.T) {
	l := Classify("/mnt/x", Env{FSType: "fuse", Mount: "/mnt/x"})
	if l.Kind != KindUnknown {
		t.Fatalf("a generic fuse mount classified as %s; it could be a cloud drive or a share", l)
	}
}

// A synced folder sits on an ordinary local volume. The sync is what changes
// the rules, so it must win over the filesystem type.
func TestCloudWinsOverLocalFilesystem(t *testing.T) {
	env := Env{Home: home(), FSType: "apfs", Mount: "/"}
	cases := map[string]string{
		home("Dropbox", "eng"): "dropbox",
		home("Library", "CloudStorage", "OneDrive-Acme", "Boards"):            "onedrive",
		home("Library", "CloudStorage", "GoogleDrive-dee@x.com", "My Drive"):  "google-drive",
		home("Library", "CloudStorage", "Dropbox-Team", "eng"):                "dropbox",
		home("Library", "Mobile Documents", "com~apple~CloudDocs", "Designs"): "icloud",
		home("OneDrive - Personal", "Documents"):                              "onedrive",
		home("Google Drive", "My Drive", "proj"):                              "google-drive",
	}
	for p, want := range cases {
		l := Classify(p, env)
		if l.Kind != KindCloudSynced || l.Provider != want {
			t.Errorf("%s: got %s, want cloud-synced/%s", p, l, want)
		}
	}
}

// The CloudStorage container itself is not a synced folder; only its
// per-account children are.
func TestCloudStorageContainerIsNotSynced(t *testing.T) {
	l := Classify(home("Library", "CloudStorage"), Env{Home: home(), FSType: "apfs", Mount: "/"})
	if l.Kind == KindCloudSynced {
		t.Fatalf("the CloudStorage container classified as %s", l)
	}
}

func TestDeclaredDropboxRootBeatsHeuristics(t *testing.T) {
	// Dropbox moved to a non-default place. Only info.json knows.
	env := Env{Home: home(), FSType: "apfs", Mount: "/",
		DropboxRoots: []string{filepath.FromSlash("/Data/DbxTeam")}}
	l := Classify(filepath.FromSlash("/Data/DbxTeam/eng"), env)
	if l.Kind != KindCloudSynced || l.Provider != "dropbox" || l.Confidence < 0.9 {
		t.Fatalf("got %s at %.2f", l, l.Confidence)
	}
}

func TestOneDriveEnvVar(t *testing.T) {
	root := filepath.FromSlash("/Users/dee/Work/OneDrive - Acme")
	env := Env{Home: home(), FSType: "ntfs", Vars: map[string]string{"OneDriveCommercial": root}}
	l := Classify(filepath.Join(root, "Boards"), env)
	if l.Kind != KindCloudSynced || l.Provider != "onedrive" {
		t.Fatalf("got %s", l)
	}
}

func TestMarkerFileAboveThePath(t *testing.T) {
	marker := filepath.FromSlash("/Data/Sync/.dropbox")
	env := Env{Home: home(), FSType: "apfs", Mount: "/",
		Exists: func(p string) bool { return p == marker }}
	l := Classify(filepath.FromSlash("/Data/Sync/eng/boards"), env)
	if l.Kind != KindCloudSynced || l.Provider != "dropbox" {
		t.Fatalf("got %s", l)
	}
	if l.Confidence >= 0.9 {
		t.Errorf("a marker file is weaker evidence than a declared root, but scored %.2f", l.Confidence)
	}
	if l.Mount != filepath.FromSlash("/Data/Sync") {
		t.Errorf("sync root = %q, want the directory holding the marker", l.Mount)
	}
}

func TestMarkerSearchStopsAtHome(t *testing.T) {
	// A marker in the home directory itself must not make everything under
	// home look synced.
	marker := home(".dropbox")
	env := Env{Home: home(), FSType: "apfs", Mount: "/",
		Exists: func(p string) bool { return p == marker }}
	if l := Classify(home("src", "proj"), env); l.Kind == KindCloudSynced {
		t.Fatalf("home-level marker leaked: %s", l)
	}
}

func TestUNCPathIsAShare(t *testing.T) {
	l := Classify(`\\server\eng\boards`, Env{UNC: true, Mount: `\\server\eng`})
	if l.Kind != KindNetworkShare {
		t.Fatalf("got %s", l)
	}
}

// Every classification must explain itself. An empty evidence string is a
// decision a person cannot check.
func TestEveryOutcomeHasEvidence(t *testing.T) {
	envs := []Env{
		{}, {FSType: "apfs"}, {FSType: "smbfs"}, {FSType: "fuse"}, {UNC: true},
		{Home: home(), FSType: "apfs"},
	}
	for _, env := range envs {
		for _, p := range []string{home("Dropbox", "x"), home("src")} {
			if l := Classify(p, env); l.Evidence == "" {
				t.Errorf("%+v %s: %s has no evidence", env, p, l)
			}
		}
	}
}

func TestProbeRunsOnThisPlatform(t *testing.T) {
	l := Probe(t.TempDir())
	if l.Kind == KindUnknown {
		t.Logf("probe could not classify a temp dir on this platform: %s", l.Evidence)
	}
	t.Logf("temp dir: %s (%s)", l, l.Evidence)
}
