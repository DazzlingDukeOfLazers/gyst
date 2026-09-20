// Package manifest reads .gyst/project.yaml, the checked-in statement of what
// a project is and which files belong to it.
//
// A manifest is the second-highest form of membership evidence, below only an
// explicit assertion by an authorized person and above anything Gyst infers
// from markers, rules, or content. It is configuration addressed to Gyst by
// the people who own the tree, which is why the scanner reads it under any
// content policy short of exclude: it is not engineering data leaving the
// boundary, it is the boundary being described.
package manifest

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const Filename = ".gyst/project.yaml"

// Manifest is the parsed file. Members are glob patterns relative to the
// source root, matched against locators; ** spans directories.
type Manifest struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Members     []string `yaml:"members" json:"members"`
	Owners      []string `yaml:"owners,omitempty" json:"owners,omitempty"`
}

// Parse decodes a manifest. dir is the manifest's own directory as a locator
// ("" for the source root); it supplies the default membership and the
// fallback id. Warnings report what was tolerated; an error means the file
// could not be used at all.
func Parse(body []byte, dir string) (Manifest, []string, error) {
	var m Manifest
	var warnings []string

	dec := yaml.NewDecoder(strings.NewReader(string(body)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		if strings.Contains(err.Error(), "field") && strings.Contains(err.Error(), "not found") {
			// Unknown keys are a warning, not a rejection: a newer manifest
			// must remain readable by an older Gyst.
			warnings = append(warnings, "unknown keys ignored: "+err.Error())
			m = Manifest{}
			if err := yaml.Unmarshal(body, &m); err != nil {
				return Manifest{}, warnings, fmt.Errorf("manifest: %w", err)
			}
		} else if err.Error() != "EOF" {
			return Manifest{}, warnings, fmt.Errorf("manifest: %w", err)
		}
	}

	if m.Name == "" && m.ID == "" {
		return Manifest{}, warnings, fmt.Errorf("manifest: neither name nor id is set")
	}
	if m.ID == "" {
		m.ID = Slug(m.Name)
		warnings = append(warnings, "id derived from name: "+m.ID)
	}
	if m.Name == "" {
		m.Name = m.ID
	}
	if len(m.Members) == 0 {
		// The folder holding the manifest is the project unless told otherwise.
		if dir == "" {
			m.Members = []string{"**"}
		} else {
			m.Members = []string{dir + "/**"}
		}
		warnings = append(warnings, "no members listed; the manifest's own folder is assumed")
	}
	for i, p := range m.Members {
		m.Members[i] = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "./")
	}
	return m, warnings, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes a stable, readable id from a name.
func Slug(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}

// Match reports whether a locator satisfies a member pattern. Segments are
// matched with path.Match; a ** segment matches any number of segments. As
// in .gitignore, a trailing /** means what is inside the folder, not the
// folder itself; the bare pattern ** matches everything.
func Match(pattern, locator string) bool {
	if pattern == "**" {
		return true
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(locator, "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return len(segs) > 0
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// Dir returns the manifest's directory as a locator, "" for the root.
func Dir(locator string) string {
	d := strings.TrimSuffix(locator, Filename)
	return strings.TrimSuffix(d, "/")
}

// IsManifest reports whether a locator is a project manifest.
func IsManifest(locator string) bool {
	return locator == Filename || strings.HasSuffix(locator, "/"+Filename)
}
