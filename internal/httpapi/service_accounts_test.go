package httpapi

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestNormalizeServiceScopes(t *testing.T) {
	got, valid := normalizeServiceScopes([]string{"work:write", "work:read", "work:write"})
	if !valid || len(got) != 2 || got[0] != "work:read" || got[1] != "work:write" {
		t.Fatalf("normalizeServiceScopes() = %#v, %t", got, valid)
	}
	if _, valid := normalizeServiceScopes([]string{"secrets:read"}); valid {
		t.Fatal("unsupported scope was accepted")
	}
	if _, valid := normalizeServiceScopes(nil); valid {
		t.Fatal("empty scopes were accepted")
	}
}

func TestNewServiceAccountToken(t *testing.T) {
	token, hash, prefix, err := newServiceAccountToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "wgsa_") || len(token) < 40 {
		t.Fatalf("unexpected token shape: %q", token)
	}
	if prefix != token[:13] {
		t.Fatalf("prefix = %q, want %q", prefix, token[:13])
	}
	if hash != sha256.Sum256([]byte(token)) {
		t.Fatal("stored hash does not match token")
	}
}
