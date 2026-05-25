package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// fetchOpItemReal reads a 1Password item referenced as "op://<vault>/<item>"
// and returns its fields keyed by both label and id (app_id / installation_id /
// private_key).
//
// When the private key is stored as a native SSH Key field (type SSHKEY), its
// `op item get` JSON value is OpenSSH-format, which Go's JWT parser can't read.
// We re-fetch that field with `op read`, which returns the original PKCS#1/#8
// PEM. Plain text fields keep using their JSON value directly.
func fetchOpItemReal(ref string) (map[string]string, error) {
	vault, item, err := parseOpReference(ref)
	if err != nil {
		return nil, err
	}

	out, err := runOp("item", "get", item, "--vault", vault, "--format", "json")
	if err != nil {
		return nil, err
	}

	fields, sshKeyID, err := parseOpFields(out)
	if err != nil {
		return nil, err
	}

	if sshKeyID != "" {
		pem, err := runOp("read", fmt.Sprintf("op://%s/%s/%s", vault, item, sshKeyID))
		if err != nil {
			return nil, err
		}
		fields["private_key"] = strings.TrimRight(string(pem), "\r\n")
	}
	return fields, nil
}

// parseOpReference splits "op://<vault>/<item>[/...]" into vault and item.
func parseOpReference(ref string) (vault, item string, err error) {
	trimmed := strings.TrimPrefix(ref, "op://")
	var parts []string
	for _, p := range strings.Split(trimmed, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return "", "", fmt.Errorf("credentials must look like op://<vault>/<item>, got %q", ref)
	}
	return parts[0], parts[1], nil
}

// parseOpFields builds a field lookup (keyed by both id and label) from
// `op item get --format json` output. The id of an SSH-key-typed field is
// returned separately, since its value must be fetched via `op read` instead.
func parseOpFields(data []byte) (fields map[string]string, sshKeyID string, err error) {
	var parsed struct {
		Fields []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, "", fmt.Errorf("parse op output: %w", err)
	}

	fields = map[string]string{}
	for _, f := range parsed.Fields {
		if f.Type == "SSHKEY" {
			if sshKeyID == "" {
				sshKeyID = f.ID
			}
			continue // OpenSSH-format value; fetched via `op read` instead
		}
		if f.Value == "" {
			continue
		}
		if f.ID != "" {
			if _, ok := fields[f.ID]; !ok {
				fields[f.ID] = f.Value
			}
		}
		if f.Label != "" {
			fields[f.Label] = f.Value // label takes priority
		}
	}
	return fields, sshKeyID, nil
}

// runOp executes the 1Password CLI and returns stdout, surfacing stderr on error.
func runOp(args ...string) ([]byte, error) {
	cmd := exec.Command("op", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
