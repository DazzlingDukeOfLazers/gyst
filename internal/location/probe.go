package location

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Probe classifies a path using what the operating system reports.
func Probe(path string) Location {
	env := Env{
		Home:   homeDir(),
		Vars:   map[string]string{},
		Exists: func(p string) bool { _, err := os.Lstat(p); return err == nil },
	}
	for _, v := range []string{"OneDrive", "OneDriveCommercial", "OneDriveConsumer"} {
		if x := os.Getenv(v); x != "" {
			env.Vars[v] = x
		}
	}
	env.DropboxRoots = dropboxRoots(env.Home)
	fillVolume(clean(path), &env)
	return Classify(path, env)
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// dropboxRoots reads the sync roots Dropbox records about itself. This is the
// one authoritative source: the folder can be moved anywhere, and this file
// is what the client itself consults.
func dropboxRoots(home string) []string {
	if home == "" {
		return nil
	}
	candidates := []string{filepath.Join(home, ".dropbox", "info.json")}
	if appdata := os.Getenv("LOCALAPPDATA"); appdata != "" {
		candidates = append(candidates, filepath.Join(appdata, "Dropbox", "info.json"))
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		candidates = append(candidates, filepath.Join(appdata, "Dropbox", "info.json"))
	}
	for _, c := range candidates {
		if roots := parseDropboxInfo(c); len(roots) > 0 {
			return roots
		}
	}
	return nil
}

func parseDropboxInfo(path string) []string {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var accounts map[string]struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(body, &accounts); err != nil {
		return nil
	}
	var roots []string
	for _, a := range accounts {
		if a.Path != "" {
			roots = append(roots, a.Path)
		}
	}
	return roots
}
