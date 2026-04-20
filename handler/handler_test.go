package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- realIP ---

func TestRealIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	if got := realIP(r); got != "1.2.3.4" {
		t.Errorf("got %q want 1.2.3.4", got)
	}
}

func TestRealIP_XForwardedFor_Whitespace(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "  1.2.3.4  ")

	if got := realIP(r); got != "1.2.3.4" {
		t.Errorf("got %q want 1.2.3.4", got)
	}
}

func TestRealIP_RemoteAddr_StripPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:54321"

	if got := realIP(r); got != "10.0.0.1" {
		t.Errorf("got %q want 10.0.0.1", got)
	}
}

func TestRealIP_IPv6_StripPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::1]:8080"

	if got := realIP(r); got != "[::1]" {
		t.Errorf("got %q want [::1]", got)
	}
}

func TestRealIP_FallsBackToRemoteAddr_WhenXFFEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:1234"
	// No X-Forwarded-For header

	if got := realIP(r); got != "192.168.1.1" {
		t.Errorf("got %q want 192.168.1.1", got)
	}
}

// --- hashIP ---

func TestHashIP_Deterministic(t *testing.T) {
	a := hashIP("1.2.3.4", "salt")
	b := hashIP("1.2.3.4", "salt")
	if a != b {
		t.Errorf("hashIP not deterministic: %q != %q", a, b)
	}
}

func TestHashIP_SaltChangesHash(t *testing.T) {
	a := hashIP("1.2.3.4", "salt1")
	b := hashIP("1.2.3.4", "salt2")
	if a == b {
		t.Error("different salts should produce different hashes")
	}
}

func TestHashIP_Length(t *testing.T) {
	h := hashIP("1.2.3.4", "salt")
	if len(h) != 16 {
		t.Errorf("hash length: got %d want 16", len(h))
	}
}

func TestHashIP_IPChangesHash(t *testing.T) {
	a := hashIP("1.2.3.4", "salt")
	b := hashIP("1.2.3.5", "salt")
	if a == b {
		t.Error("different IPs should produce different hashes")
	}
}
