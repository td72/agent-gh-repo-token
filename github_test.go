package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func testPrivateKeyPEM(t *testing.T) (priv string, pub *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return string(pemBytes), &key.PublicKey
}

func TestApiBase(t *testing.T) {
	if got := apiBase("github.com"); got != "https://api.github.com" {
		t.Errorf("apiBase(github.com) = %q", got)
	}
	if got := apiBase("ghe.example.com"); got != "https://ghe.example.com/api/v3" {
		t.Errorf("apiBase(ghe.example.com) = %q", got)
	}
}

func TestBuildJWT(t *testing.T) {
	priv, pub := testPrivateKeyPEM(t)
	tokStr, err := buildJWT("1234567", priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(tokStr, func(*jwt.Token) (any, error) { return pub, nil })
	if err != nil {
		t.Fatalf("parse signed jwt: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iss"] != "1234567" {
		t.Errorf("iss = %v, want 1234567", claims["iss"])
	}
}

func TestBuildJWTBadKey(t *testing.T) {
	if _, err := buildJWT("1", "not a pem key"); err == nil {
		t.Error("expected error for invalid private key")
	}
}
