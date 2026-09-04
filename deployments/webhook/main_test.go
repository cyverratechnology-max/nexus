package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestValidSignature(t *testing.T) {
	secret := []byte("test-secret-long-enough")
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	service := &server{secret: secret}
	if !service.validSignature(signature, body) {
		t.Fatal("expected valid signature")
	}
	if service.validSignature(signature, []byte("tampered")) {
		t.Fatal("tampered payload must be rejected")
	}
}
