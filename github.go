package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// apiBase returns the REST API root for a host. github.com uses api.github.com;
// GitHub Enterprise Server hosts use https://<host>/api/v3.
func apiBase(host string) string {
	if host == "github.com" || host == "www.github.com" {
		return "https://api.github.com"
	}
	return "https://" + host + "/api/v3"
}

// buildJWT mints a short-lived (10 min) RS256 JWT signed with the App's private
// key, used to authenticate as the GitHub App itself.
func buildJWT(appID, privateKeyPEM string) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // allow clock drift
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    appID,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// mintTokenReal exchanges the App JWT for an installation access token scoped to
// a single repository and the given permissions. The token expires in 1 hour.
func mintTokenReal(ctx context.Context, host, installationID, jwtToken, repo string, permissions map[string]string) (string, error) {
	body := map[string]any{"repositories": []string{repo}}
	if len(permissions) > 0 {
		body["permissions"] = permissions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", apiBase(host), installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agent-gh-repo-token/"+version)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if out.Token == "" {
		return "", errors.New("token missing from GitHub response")
	}
	return out.Token, nil
}
