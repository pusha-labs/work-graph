package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIdentifierUsesForwardedClient(t *testing.T) {
	request := httptest.NewRequest("POST", "http://work.example/api/v1/auth/login", nil)
	request.RemoteAddr = "10.0.0.5:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := clientIdentifier(request); got != "203.0.113.9" {
		t.Fatalf("clientIdentifier() = %q", got)
	}
}

func TestDurationInterval(t *testing.T) {
	if got := durationInterval(15 * time.Minute); got != "900 seconds" {
		t.Fatalf("durationInterval() = %q", got)
	}
}
