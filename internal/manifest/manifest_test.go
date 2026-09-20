package manifest

import "testing"

func TestParseFixtureManifest(t *testing.T) {
	m, warn, err := Parse([]byte("name: Widget\nmembers:\n  - '**'\n"), "engineering/widget")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "widget" || m.Name != "Widget" {
		t.Errorf("id/name = %q/%q", m.ID, m.Name)
	}
	if len(m.Members) != 1 || m.Members[0] != "**" {
		t.Errorf("members = %v", m.Members)
	}
	if got := Resolve("engineering/widget", m.Members[0]); got != "engineering/widget/**" {
		t.Errorf("resolved = %q", got)
	}
	if len(warn) != 1 {
		t.Errorf("expected one warning about the derived id, got %v", warn)
	}
}

func TestMembersDefaultToOwnFolder(t *testing.T) {
	m, _, err := Parse([]byte("id: fw\n"), "firmware")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Members) != 1 || m.Members[0] != "**" || Resolve("firmware", m.Members[0]) != "firmware/**" {
		t.Errorf("members = %v", m.Members)
	}
	if Resolve("firmware", "/shared/**") != "shared/**" {
		t.Error("a leading slash must mean root-relative")
	}
	if Resolve("", "**") != "**" {
		t.Error("root manifest")
	}
}

func TestUnknownKeysWarnRatherThanReject(t *testing.T) {
	m, warn, err := Parse([]byte("name: X\nfuture_field: 1\n"), "x")
	if err != nil {
		t.Fatalf("a newer manifest must remain readable: %v", err)
	}
	if m.Name != "X" || len(warn) == 0 {
		t.Errorf("name %q warnings %v", m.Name, warn)
	}
}

func TestRejectsEmptyAndBroken(t *testing.T) {
	if _, _, err := Parse([]byte("members: [a]\n"), "x"); err == nil {
		t.Error("manifest with no name or id accepted")
	}
	if _, _, err := Parse([]byte("name: [unclosed\n"), "x"); err == nil {
		t.Error("broken YAML accepted")
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, locator string
		want             bool
	}{
		{"engineering/widget/**", "engineering/widget/widget.kicad_pcb", true},
		{"engineering/widget/**", "engineering/widget/output/widget.drl", true},
		{"engineering/widget/**", "engineering/widgets/x", false},
		{"engineering/widget/**", "engineering/widget", false},
		{"**", "anything/at/all", true},
		{"**/*.pdf", "engineering/connectors/connector_123.pdf", true},
		{"**/*.pdf", "engineering/connectors/notes.md", false},
		{"firmware/src/*.c", "firmware/src/main.c", true},
		{"firmware/src/*.c", "firmware/src/sub/x.c", false},
		{"**/output/**", "engineering/widget/output/a.gbr", true},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.locator); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.locator, got, c.want)
		}
	}
}

func TestDirAndIsManifest(t *testing.T) {
	if !IsManifest(".gyst/project.yaml") || !IsManifest("a/b/.gyst/project.yaml") || IsManifest("a/project.yaml") {
		t.Error("IsManifest")
	}
	if Dir(".gyst/project.yaml") != "" || Dir("a/b/.gyst/project.yaml") != "a/b" {
		t.Error("Dir")
	}
}
