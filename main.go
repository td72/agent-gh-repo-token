// Command agent-gh-repo-token mints a repository-scoped GitHub App
// installation token for coding agents and prints it (and only it) to stdout.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// exampleConfig is the starter repos.toml written by --init.
//
//go:embed examples/repos.toml
var exampleConfig string

// Exit codes — see the "終了コードの設計" table in README.md.
const (
	exitOK           = 0
	exitUsage        = 1 // argument / usage error
	exitNoConfig     = 2 // config file missing or unreadable
	exitNoEntry      = 3 // no config entry matches the repo
	exitOpFailed     = 4 // fetching the 1Password item failed
	exitMissingField = 5 // app_id / installation_id / private_key missing
	exitAPIFailed    = 6 // JWT generation or GitHub API call failed
)

// Indirected so tests can stub out the external 1Password CLI and GitHub API.
var (
	fetchOpItem = fetchOpItemReal
	mintToken   = mintTokenReal
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent-gh-repo-token", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoArg := fs.String("repo", "", "target repository as [<host>/]<owner>/<repo> (default: current git origin)")
	configPath := fs.String("config", "", "path to repos.toml (default: ~/.config/agent-gh-repo-token/repos.toml)")
	doInit := fs.Bool("init", false, "write a starter repos.toml to the config path and exit")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage:")
		fmt.Fprintln(stderr, "  agent-gh-repo-token [--repo [<host>/]<owner>/<repo>] [--config path]")
		fmt.Fprintln(stderr, "  agent-gh-repo-token --init [--config path]")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return exitOK
	}

	path := *configPath
	if path == "" {
		path = defaultConfigPath()
	}

	if *doInit {
		return runInit(path, stderr)
	}
	// --repo defaults to the current directory's git `origin` remote.
	repoSpec := *repoArg
	fromGit := false
	if repoSpec == "" {
		origin, gerr := gitOriginURL()
		if gerr != nil {
			fmt.Fprintf(stderr, "error: --repo not given and could not detect it from git (%v)\n", gerr)
			fs.Usage()
			return exitUsage
		}
		repoSpec = origin
		fromGit = true
	}

	host, owner, repo, err := parseRepo(repoSpec)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if fromGit {
		fmt.Fprintf(stderr, "using repo %s/%s/%s (from git origin)\n", host, owner, repo)
	}

	switch st, statErr := os.Stat(path); {
	case statErr == nil && st.IsDir():
		fmt.Fprintf(stderr, "config path is a directory: %s\n", path)
		return exitNoConfig
	case os.IsNotExist(statErr):
		fmt.Fprintf(stderr, "config not found: %s\n", path)
		return exitNoConfig
	case statErr != nil:
		fmt.Fprintf(stderr, "cannot access config %s: %v\n", path, statErr)
		return exitNoConfig
	}
	cfg, err := loadConfig(path)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read config %s: %v\n", path, err)
		return exitNoConfig
	}
	entry, ok := resolveEntry(cfg, host, owner, repo)
	if !ok {
		fmt.Fprintf(stderr, "no config entry for %s/%s or %s/%s/%s\n", host, owner, host, owner, repo)
		return exitNoEntry
	}

	// Inline config values win; otherwise pull the missing ones from 1Password.
	appID := string(entry.AppID)
	installID := string(entry.InstallationID)
	privKey := entry.PrivateKey
	if (appID == "" || installID == "" || privKey == "") && entry.Credentials != "" {
		fields, err := fetchOpItem(entry.Credentials)
		if err != nil {
			fmt.Fprintf(stderr, "1Password fetch failed: %v\n", err)
			return exitOpFailed
		}
		if appID == "" {
			appID = fields["app_id"]
		}
		if installID == "" {
			installID = fields["installation_id"]
		}
		if privKey == "" {
			privKey = fields["private_key"]
		}
	}

	var missing []string
	if appID == "" {
		missing = append(missing, "app_id")
	}
	if installID == "" {
		missing = append(missing, "installation_id")
	}
	if privKey == "" {
		missing = append(missing, "private_key")
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "missing required field(s): %s\n", strings.Join(missing, ", "))
		return exitMissingField
	}

	jwtToken, err := buildJWT(appID, privKey)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitAPIFailed
	}
	token, err := mintToken(context.Background(), host, installID, jwtToken, repo, entry.Permissions)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitAPIFailed
	}

	fmt.Fprintln(stdout, token)
	return exitOK
}

// runInit writes the bundled example repos.toml to path without overwriting an
// existing file, to bootstrap a new config.
func runInit(path string, stderr io.Writer) int {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(stderr, "config already exists, not overwriting: %s\n", path)
		return exitUsage
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "failed to create config directory: %v\n", err)
		return exitNoConfig
	}
	if err := os.WriteFile(path, []byte(exampleConfig), 0o600); err != nil {
		fmt.Fprintf(stderr, "failed to write config: %v\n", err)
		return exitNoConfig
	}
	fmt.Fprintf(stderr, "wrote starter config: %s\n", path)
	fmt.Fprintln(stderr, "edit it (credentials / vault / permissions) before use")
	return exitOK
}

func defaultConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "agent-gh-repo-token", "repos.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "agent-gh-repo-token", "repos.toml")
	}
	return filepath.Join(home, ".config", "agent-gh-repo-token", "repos.toml")
}
