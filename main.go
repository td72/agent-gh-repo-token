// Command agent-gh-repo-token mints a repository-scoped GitHub App
// installation token for coding agents and prints it (and only it) to stdout.
package main

import (
	"context"
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
	repoArg := fs.String("repo", "", "target repository as [<host>/]<owner>/<repo> (host defaults to github.com)")
	configPath := fs.String("config", "", "path to repos.toml (default: ~/.config/agent-gh-repo-token/repos.toml)")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: agent-gh-repo-token --repo [<host>/]<owner>/<repo> [--config path]")
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
	if *repoArg == "" {
		fmt.Fprintln(stderr, "error: --repo is required")
		fs.Usage()
		return exitUsage
	}

	path := *configPath
	if path == "" {
		path = defaultConfigPath()
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

	host, owner, repo, err := parseRepo(*repoArg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
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
