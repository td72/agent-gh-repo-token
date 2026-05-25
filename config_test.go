package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOrigin(t *testing.T) {
	cases := map[string][3]string{
		"github.com/td72/foo":             {"github.com", "td72", "foo"},
		"https://github.com/td72/foo.git": {"github.com", "td72", "foo"},
		"git@github.com:td72/foo.git":     {"github.com", "td72", "foo"},
		"ssh://git@github.com/td72/foo":   {"github.com", "td72", "foo"},
		"github.com/td72/foo/":            {"github.com", "td72", "foo"},
	}
	for in, want := range cases {
		h, o, r, err := parseOrigin(in)
		if err != nil {
			t.Fatalf("parseOrigin(%q) error: %v", in, err)
		}
		if h != want[0] || o != want[1] || r != want[2] {
			t.Errorf("parseOrigin(%q) = %q/%q/%q, want %v", in, h, o, r, want)
		}
	}
}

func TestParseOriginInvalid(t *testing.T) {
	if _, _, _, err := parseOrigin("github.com/td72"); err == nil {
		t.Error("expected error for too-short origin")
	}
}

func TestResolveEntryRepoOverridesOrg(t *testing.T) {
	cfg := map[string]Entry{
		"github.com/td72": {
			Credentials: "op://Personal/x",
			Permissions: map[string]string{"contents": "write", "pull_requests": "write"},
		},
		"github.com/td72/foo": {
			Permissions: map[string]string{"contents": "read"},
		},
	}
	e, ok := resolveEntry(cfg, "github.com", "td72", "foo")
	if !ok {
		t.Fatal("expected ok")
	}
	if e.Credentials != "op://Personal/x" {
		t.Errorf("credentials = %q, want inherited from org", e.Credentials)
	}
	if len(e.Permissions) != 1 || e.Permissions["contents"] != "read" {
		t.Errorf("permissions = %v, want repo override {contents:read}", e.Permissions)
	}
}

func TestResolveEntryOrgOnly(t *testing.T) {
	cfg := map[string]Entry{
		"github.com/td72": {Credentials: "op://Personal/x"},
	}
	e, ok := resolveEntry(cfg, "github.com", "td72", "foo")
	if !ok || e.Credentials != "op://Personal/x" {
		t.Errorf("resolveEntry org-only = %+v, ok=%v", e, ok)
	}
}

func TestResolveEntryNone(t *testing.T) {
	if _, ok := resolveEntry(map[string]Entry{}, "github.com", "td72", "foo"); ok {
		t.Error("expected ok=false when no entry matches")
	}
}

func TestLoadConfigIntInstallationID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.toml")
	content := `["github.com/td72"]
credentials = "op://Personal/x"
installation_id = 99999999
permissions = { contents = "write" }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(cfg["github.com/td72"].InstallationID); got != "99999999" {
		t.Errorf("installation_id = %q, want 99999999", got)
	}
}
