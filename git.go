package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// gitOriginURL returns the URL of the `origin` remote in the current directory.
// Indirected so tests can stub it.
var gitOriginURL = gitOriginURLReal

func gitOriginURLReal() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	url := strings.TrimSpace(stdout.String())
	if url == "" {
		return "", errors.New("origin remote has no URL")
	}
	return url, nil
}
