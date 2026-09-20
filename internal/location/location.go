// Package location classifies where a path physically lives: a local volume,
// a network share, or a folder synchronised by a cloud provider.
//
// This is not decoration. The architecture gives each kind different rules:
// shares get scheduled, rate-limited passes and may be unavailable; synced
// folders may hold placeholder files whose content is not present locally and
// must never contain Git metadata; local folders are fast and complete. The
// kind is a policy input, so it is recorded on the source and shown next to
// every root.
//
// Classify is pure and takes everything it needs in Env, so every rule is
// testable on any platform. Probe fills Env from the operating system.
package location

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Kind string

const (
	KindLocal        Kind = "local"
	KindNetworkShare Kind = "network-share"
	KindCloudSynced  Kind = "cloud-synced"
	KindUnknown      Kind = "unknown"
)

// Location is the classification of one path.
type Location struct {
	Kind Kind `json:"kind"`
	// Provider names the filesystem for local and network kinds ("apfs",
	// "smbfs") and the sync product for cloud kinds ("dropbox", "onedrive").
	Provider string `json:"provider,omitempty"`
	// Mount is the mount point or sync root the path lives under.
	Mount string `json:"mount,omitempty"`
	// Evidence says how the decision was made, for a person.
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
}

func (l Location) String() string {
	if l.Provider == "" {
		return string(l.Kind)
	}
	return string(l.Kind) + "/" + l.Provider
}

// Env is everything Classify consults. Probe fills it from the OS; tests fill
// it by hand.
type Env struct {
	Home string
	// FSType and Mount describe the volume the path is on, as the OS reports
	// them. Empty when the probe could not tell.
	FSType string
	Mount  string
	// Vars holds environment variables of interest, e.g. OneDrive on Windows.
	Vars map[string]string
	// DropboxRoots are sync roots read from Dropbox's own info.json, the one
	// authoritative statement of where Dropbox folders are.
	DropboxRoots []string
	// Exists reports whether a marker path exists. Nil means "no markers".
	Exists func(string) bool
	// UNC reports the path is a Windows \\server\share path.
	UNC bool
}

// Network filesystem types across platforms. A match means bytes cross a
// network on every read, whatever the path looks like.
var networkFS = map[string]string{
	"smbfs": "smb", "smb": "smb", "smb3": "smb", "cifs": "smb",
	"afpfs": "afp", "afp": "afp",
	"nfs": "nfs", "nfs4": "nfs",
	"webdav": "webdav", "davfs": "webdav", "davfs2": "webdav",
	"fuse.sshfs": "sshfs", "sshfs": "sshfs",
	"9p": "9p", "ncpfs": "ncp", "fuse.rclone": "rclone",
}

// cloudRoot is a well-known sync folder relative to the home directory.
type cloudRoot struct {
	rel      string // relative to home, slash-separated
	provider string
	prefix   bool // rel names a parent whose children are <Provider>-<account>
}

var cloudRoots = []cloudRoot{
	{"Library/CloudStorage", "", true}, // macOS File Provider: OneDrive-X, GoogleDrive-X, Dropbox-X, Box-X
	{"Library/Mobile Documents/com~apple~CloudDocs", "icloud", false},
	{"Library/Mobile Documents", "icloud", false},
	{"Dropbox", "dropbox", false},
	{"OneDrive", "onedrive", false},
	{"OneDrive - Personal", "onedrive", false},
	{"Google Drive", "google-drive", false},
	{"iCloudDrive", "icloud", false},
	{"iCloud Drive", "icloud", false},
	{"Box", "box", false},
	{"Nextcloud", "nextcloud", false},
	{"ownCloud", "owncloud", false},
	{"Proton Drive", "proton-drive", false},
}

// Marker files a sync client leaves in or above a synced folder. Weaker than
// a declared root: a copied tree carries them too.
var cloudMarkers = []struct {
	name, provider string
}{
	{".dropbox", "dropbox"},
	{".dropbox.cache", "dropbox"},
	{".tmp.drivedownload", "google-drive"},
	{".tmp.driveupload", "google-drive"},
	{".sync_ffs.db", "syncthing"},
	{".stfolder", "syncthing"},
}

// Classify decides where path lives. Cloud sync wins over the filesystem
// type, because a synced folder sits on an ordinary local volume and the
// sync is what changes the rules.
func Classify(path string, env Env) Location {
	p := clean(path)

	if l, ok := classifyCloud(p, env); ok {
		return l
	}
	if env.UNC {
		return Location{KindNetworkShare, "smb", env.Mount,
			"UNC path; bytes cross the network on every read", 0.95}
	}
	if fs := strings.ToLower(env.FSType); fs != "" {
		if prov, ok := networkFS[fs]; ok {
			return Location{KindNetworkShare, prov, env.Mount,
				fmt.Sprintf("mounted as %s at %s", fs, env.Mount), 0.95}
		}
		if strings.HasPrefix(fs, "fuse") {
			return Location{KindUnknown, fs, env.Mount,
				fmt.Sprintf("mounted as %s at %s; a user-space filesystem that could be anything", fs, env.Mount), 0.3}
		}
		return Location{KindLocal, fs, env.Mount,
			fmt.Sprintf("mounted as %s at %s", fs, env.Mount), 0.9}
	}
	return Location{KindUnknown, "", "", "the filesystem type could not be determined", 0.0}
}

func classifyCloud(p string, env Env) (Location, bool) {
	// 1. Roots the sync client itself declared.
	for _, r := range env.DropboxRoots {
		if under(p, clean(r)) {
			return Location{KindCloudSynced, "dropbox", clean(r),
				"inside a Dropbox root declared in Dropbox's info.json", 0.95}, true
		}
	}
	for _, v := range []struct{ name, provider string }{
		{"OneDrive", "onedrive"}, {"OneDriveCommercial", "onedrive"}, {"OneDriveConsumer", "onedrive"},
	} {
		if root := env.Vars[v.name]; root != "" && under(p, clean(root)) {
			return Location{KindCloudSynced, v.provider, clean(root),
				fmt.Sprintf("inside the root named by the %s environment variable", v.name), 0.95}, true
		}
	}

	// 2. Well-known folders under the home directory.
	if env.Home != "" {
		home := clean(env.Home)
		for _, cr := range cloudRoots {
			root := filepath.Join(home, filepath.FromSlash(cr.rel))
			if !under(p, root) {
				continue
			}
			if cr.prefix {
				// The child of the container names the provider: OneDrive-Acme.
				rest := strings.TrimPrefix(p[len(root):], string(filepath.Separator))
				child, _, _ := strings.Cut(rest, string(filepath.Separator))
				if child == "" {
					continue // the container itself is not a synced folder
				}
				prov := providerFromChild(child)
				return Location{KindCloudSynced, prov, filepath.Join(root, child),
					fmt.Sprintf("inside %s, a macOS File Provider sync root", filepath.Join("~", cr.rel, child)), 0.9}, true
			}
			return Location{KindCloudSynced, cr.provider, root,
				fmt.Sprintf("inside %s, a well-known sync folder", filepath.Join("~", cr.rel)), 0.85}, true
		}
	}

	// 3. Markers left by a sync client, searched from the path upwards. The
	// home directory itself is never probed: ~/.dropbox is the client's own
	// configuration folder, not a mark on a synced tree.
	if env.Exists != nil {
		home, stop := clean(env.Home), clean(env.Mount)
		for dir := p; ; {
			if env.Home != "" && dir == home {
				break
			}
			for _, m := range cloudMarkers {
				if env.Exists(filepath.Join(dir, m.name)) {
					return Location{KindCloudSynced, m.provider, dir,
						fmt.Sprintf("%s marker found at %s", m.name, dir), 0.7}, true
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir || dir == stop {
				break
			}
			dir = parent
		}
	}
	return Location{}, false
}

// providerFromChild maps a macOS CloudStorage folder name to a provider.
func providerFromChild(child string) string {
	head, _, _ := strings.Cut(child, "-")
	switch strings.ToLower(head) {
	case "onedrive", "onedrive ":
		return "onedrive"
	case "googledrive":
		return "google-drive"
	case "dropbox":
		return "dropbox"
	case "box":
		return "box"
	case "icloud", "iclouddrive":
		return "icloud"
	case "protondrive":
		return "proton-drive"
	}
	return strings.ToLower(head)
}

func clean(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// under reports whether p is root or inside it. Case-insensitive on Windows
// and macOS, where the default filesystems are.
func under(p, root string) bool {
	if root == "" {
		return false
	}
	if caseInsensitive {
		p, root = strings.ToLower(p), strings.ToLower(root)
	}
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(filepath.Separator))
}

// Kinds lists the known kinds in display order.
func Kinds() []Kind {
	k := []Kind{KindLocal, KindNetworkShare, KindCloudSynced, KindUnknown}
	sort.Slice(k, func(i, j int) bool { return k[i] < k[j] })
	return k
}
