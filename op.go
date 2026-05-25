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
// and returns its fields keyed by both label and id.
func fetchOpItemReal(ref string) (map[string]string, error) {
	trimmed := strings.TrimPrefix(ref, "op://")
	var parts []string
	for _, p := range strings.Split(trimmed, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("credentials must look like op://<vault>/<item>, got %q", ref)
	}
	vault, item := parts[0], parts[1]

	cmd := exec.Command("op", "item", "get", item, "--vault", vault, "--format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}

	var parsed struct {
		Fields []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("parse op output: %w", err)
	}

	// Prefer label over id when both name the same value.
	fields := map[string]string{}
	for _, f := range parsed.Fields {
		if f.Value == "" {
			continue
		}
		if f.ID != "" {
			if _, ok := fields[f.ID]; !ok {
				fields[f.ID] = f.Value
			}
		}
		if f.Label != "" {
			fields[f.Label] = f.Value
		}
	}
	return fields, nil
}
