package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// tomlValue accepts either a TOML string or integer and stores it as a string,
// so app_id / installation_id may be written either way in repos.toml.
type tomlValue string

func (v *tomlValue) UnmarshalTOML(data any) error {
	switch x := data.(type) {
	case string:
		*v = tomlValue(x)
	case int64:
		*v = tomlValue(strconv.FormatInt(x, 10))
	default:
		// Reject floats (e.g. 123.0, 1e6) rather than silently truncating an ID.
		return fmt.Errorf("expected string or integer, got %T", data)
	}
	return nil
}

// Entry is one config block, keyed by "<host>/<owner>" or "<host>/<owner>/<repo>".
type Entry struct {
	// Credentials references the App credentials in a secret store. The URI
	// scheme selects the backend (currently only op:// for 1Password).
	Credentials    string            `toml:"credentials"`
	Permissions    map[string]string `toml:"permissions"`
	AppID          tomlValue         `toml:"app_id"`
	InstallationID tomlValue         `toml:"installation_id"`
	PrivateKey     string            `toml:"private_key"`
}

func loadConfig(path string) (map[string]Entry, error) {
	var cfg map[string]Entry
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveEntry shallow-merges the owner-level block under the repo-level block,
// with the repo-level value winning per top-level key. ok is false when neither
// "<host>/<owner>" nor "<host>/<owner>/<repo>" exists.
func resolveEntry(cfg map[string]Entry, host, owner, repo string) (Entry, bool) {
	orgKey := host + "/" + owner
	repoKey := orgKey + "/" + repo
	org, hasOrg := cfg[orgKey]
	rep, hasRepo := cfg[repoKey]
	if !hasOrg && !hasRepo {
		return Entry{}, false
	}

	merged := Entry{}
	if hasOrg {
		merged = org
	}
	if hasRepo {
		if rep.Credentials != "" {
			merged.Credentials = rep.Credentials
		}
		if rep.Permissions != nil {
			merged.Permissions = rep.Permissions
		}
		if rep.AppID != "" {
			merged.AppID = rep.AppID
		}
		if rep.InstallationID != "" {
			merged.InstallationID = rep.InstallationID
		}
		if rep.PrivateKey != "" {
			merged.PrivateKey = rep.PrivateKey
		}
	}
	return merged, true
}

// parseRepo normalizes a repo argument to (host, owner, repo). It accepts the
// gh-CLI-style "[<host>/]<owner>/<repo>", defaulting the host to github.com when
// omitted, and tolerates a leading scheme, an scp-like "git@host:owner/repo",
// and a trailing ".git" or "/".
func parseRepo(arg string) (host, owner, repo string, err error) {
	s := strings.TrimSpace(arg)
	hadAt := strings.Contains(s, "@")
	for _, scheme := range []string{"https://", "http://", "ssh://", "git://"} {
		s = strings.TrimPrefix(s, scheme)
	}
	if at := strings.Index(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	// scp syntax: host:owner/repo -> host/owner/repo (only when ':' precedes '/').
	if hadAt {
		if c := strings.Index(s, ":"); c >= 0 {
			if sl := strings.Index(s, "/"); sl < 0 || c < sl {
				s = s[:c] + "/" + s[c+1:]
			}
		}
	}
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")

	var parts []string
	for _, p := range strings.Split(s, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	switch {
	case len(parts) >= 3:
		return parts[0], parts[1], parts[2], nil
	case len(parts) == 2:
		return "github.com", parts[0], parts[1], nil // host omitted
	default:
		return "", "", "", fmt.Errorf("repo must be [<host>/]<owner>/<repo>, got %q", arg)
	}
}
