package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeConfig(t, `
input: base.mmdb
output: extended.mmdb
sources:
  ipinfo:
    path: ipinfo_lite.mmdb
  czds:
    zones: [com, net, org]
    local_only: true
`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Input != "base.mmdb" {
		t.Errorf("Input = %q, want base.mmdb", c.Input)
	}
	if c.Output != "extended.mmdb" {
		t.Errorf("Output = %q, want extended.mmdb", c.Output)
	}
	if len(c.Sources) != 2 {
		t.Fatalf("Sources len = %d, want 2", len(c.Sources))
	}

	ipinfo, ok := c.Sources["ipinfo"]
	if !ok {
		t.Fatal("expected ipinfo source block")
	}
	if ipinfo["path"] != "ipinfo_lite.mmdb" {
		t.Errorf("ipinfo.path = %v, want ipinfo_lite.mmdb", ipinfo["path"])
	}

	czds, ok := c.Sources["czds"]
	if !ok {
		t.Fatal("expected czds source block")
	}
	if czds["local_only"] != true {
		t.Errorf("czds.local_only = %v, want true", czds["local_only"])
	}
	zones, ok := czds["zones"].([]any)
	if !ok || len(zones) != 3 {
		t.Errorf("czds.zones = %v, want 3 entries", czds["zones"])
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "input: [unterminated\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoad_Validation(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing input",
			content: "output: out.mmdb\nsources:\n  ipinfo:\n    path: x.mmdb\n",
		},
		{
			name:    "missing output",
			content: "input: in.mmdb\nsources:\n  ipinfo:\n    path: x.mmdb\n",
		},
		{
			name:    "no sources",
			content: "input: in.mmdb\noutput: out.mmdb\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.content)
			if _, err := Load(path); err == nil {
				t.Errorf("expected validation error for %q, got nil", tt.name)
			}
		})
	}
}
