package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repos.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	cfg := writeConfig(t, `["github.com/td72"]
credentials = "op://Personal/agent-gh-repo-token"
permissions = { contents = "write", pull_requests = "write" }
`)

	origFetch, origMint := fetchOpItem, mintToken
	defer func() { fetchOpItem, mintToken = origFetch, origMint }()

	fetchOpItem = func(ref string) (map[string]string, error) {
		return map[string]string{
			"app_id":          "1234567",
			"installation_id": "78901234",
			"private_key":     string(keyPEM),
		}, nil
	}
	var gotRepo, gotInstall string
	var gotPerms map[string]string
	mintToken = func(_ context.Context, _, installationID, _, repo string, permissions map[string]string) (string, error) {
		gotRepo, gotInstall, gotPerms = repo, installationID, permissions
		return "ghs_faketoken", nil
	}

	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "github.com/td72/foo", "--config", cfg}, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "ghs_faketoken" {
		t.Errorf("stdout = %q, want token only", out.String())
	}
	if gotRepo != "foo" || gotInstall != "78901234" {
		t.Errorf("repo=%q install=%q, want foo/78901234", gotRepo, gotInstall)
	}
	if gotPerms["contents"] != "write" || gotPerms["pull_requests"] != "write" {
		t.Errorf("perms = %v, want contents+pull_requests write", gotPerms)
	}
}

func TestRunInvalidRepoArg(t *testing.T) {
	cfg := writeConfig(t, `["github.com/td72"]
credentials = "op://Personal/x"
`)
	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "foo", "--config", cfg}, &out, &errOut)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d (usage)", code, exitUsage)
	}
}

func TestRunNoConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "github.com/td72/foo", "--config", "/no/such/repos.toml"}, &out, &errOut)
	if code != exitNoConfig {
		t.Errorf("exit = %d, want %d", code, exitNoConfig)
	}
}

func TestRunNoEntry(t *testing.T) {
	cfg := writeConfig(t, `["github.com/other"]
credentials = "op://x/y"
`)
	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "github.com/td72/foo", "--config", cfg}, &out, &errOut)
	if code != exitNoEntry {
		t.Errorf("exit = %d, want %d", code, exitNoEntry)
	}
}

func TestRunMissingField(t *testing.T) {
	cfg := writeConfig(t, `["github.com/td72"]
credentials = "op://Personal/x"
`)
	orig := fetchOpItem
	defer func() { fetchOpItem = orig }()
	fetchOpItem = func(string) (map[string]string, error) {
		return map[string]string{"app_id": "1"}, nil // installation_id + private_key absent
	}

	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "github.com/td72/foo", "--config", cfg}, &out, &errOut)
	if code != exitMissingField {
		t.Errorf("exit = %d, want %d", code, exitMissingField)
	}
}

func TestRunOpFailure(t *testing.T) {
	cfg := writeConfig(t, `["github.com/td72"]
credentials = "op://Personal/x"
`)
	orig := fetchOpItem
	defer func() { fetchOpItem = orig }()
	fetchOpItem = func(string) (map[string]string, error) {
		return nil, errors.New("op exploded")
	}

	var out, errOut bytes.Buffer
	code := run([]string{"--repo", "github.com/td72/foo", "--config", cfg}, &out, &errOut)
	if code != exitOpFailed {
		t.Errorf("exit = %d, want %d", code, exitOpFailed)
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--version"}, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("stdout = %q, want %q", out.String(), version)
	}
}
