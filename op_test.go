package main

import (
	"strings"
	"testing"
)

// Mirrors a real 1Password SSH Key item (private key is type SSHKEY with a
// localized label; app_id / installation_id are custom STRING fields).
func TestParseOpFieldsSSHKeyItem(t *testing.T) {
	data := []byte(`{
	  "fields": [
	    {"id":"public_key","label":"public key","type":"STRING","value":"ssh-rsa AAAA"},
	    {"id":"fingerprint","label":"fingerprint","type":"STRING","value":"SHA256:xxx"},
	    {"id":"private_key","label":"秘密鍵","type":"SSHKEY","value":"-----BEGIN OPENSSH PRIVATE KEY-----\n..."},
	    {"id":"key_type","label":"key type","type":"STRING","value":"rsa"},
	    {"id":"3moyc7bmkazsahepgyqrm4xfy4","label":"app_id","type":"STRING","value":"1234567"},
	    {"id":"t57nv5chlwsafmg7hzjmi53t4u","label":"installation_id","type":"STRING","value":"78901234"}
	  ]
	}`)
	fields, sshKeyID, err := parseOpFields(data)
	if err != nil {
		t.Fatal(err)
	}
	if sshKeyID != "private_key" {
		t.Errorf("sshKeyID = %q, want private_key", sshKeyID)
	}
	if fields["app_id"] != "1234567" || fields["installation_id"] != "78901234" {
		t.Errorf("app_id/installation_id not resolved by label: %v", fields)
	}
	// The OpenSSH-format SSHKEY value must NOT be used as private_key; it is
	// re-fetched via `op read` (which returns the PKCS#1 PEM) afterwards.
	if _, ok := fields["private_key"]; ok {
		t.Errorf("SSHKEY value must not be stored as private_key, got %q", fields["private_key"])
	}
}

// README's text-field setup: private_key is a plain STRING field, used as-is.
func TestParseOpFieldsTextField(t *testing.T) {
	data := []byte(`{
	  "fields": [
	    {"id":"app_id","label":"app_id","type":"STRING","value":"1"},
	    {"id":"installation_id","label":"installation_id","type":"STRING","value":"2"},
	    {"id":"private_key","label":"private_key","type":"STRING","value":"-----BEGIN RSA PRIVATE KEY-----\nABC\n-----END RSA PRIVATE KEY-----"}
	  ]
	}`)
	fields, sshKeyID, err := parseOpFields(data)
	if err != nil {
		t.Fatal(err)
	}
	if sshKeyID != "" {
		t.Errorf("sshKeyID = %q, want empty for a text-field key", sshKeyID)
	}
	if !strings.Contains(fields["private_key"], "BEGIN RSA PRIVATE KEY") {
		t.Errorf("text private_key not preserved: %q", fields["private_key"])
	}
}

func TestParseOpReference(t *testing.T) {
	v, i, err := parseOpReference("op://Employee/agent-gh-repo-token")
	if err != nil || v != "Employee" || i != "agent-gh-repo-token" {
		t.Errorf("parseOpReference = %q/%q err=%v", v, i, err)
	}
	if _, _, err := parseOpReference("op://Employee"); err == nil {
		t.Error("expected error for reference without item")
	}
	if _, _, err := parseOpReference("https://example.com/x/y"); err == nil {
		t.Error("expected error for non-op:// reference")
	}
}
