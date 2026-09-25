package httprunner

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplySecretHeaderAndRedaction(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	applySecretHeader(request, "token-value", "bearer", "X-Ignored")
	if request.Header.Get("Authorization") != "Bearer token-value" {
		t.Fatal("bearer credential was not applied")
	}
	if value := redactSecret("echo Bearer token-value and token-value", "token-value"); value != "echo Bearer [REDACTED] and [REDACTED]" {
		t.Fatalf("secret was not redacted: %s", value)
	}

	custom := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	applySecretHeader(custom, "key-value", "header", "X-API-Key")
	if custom.Header.Get("X-API-Key") != "key-value" {
		t.Fatal("custom header credential was not applied")
	}
}

func TestParseAllowlist(t *testing.T) {
	items := parseAllowlist("api.example.com, WEBHOOK.example.com, ")
	if len(items) != 2 {
		t.Fatalf("got %d hosts", len(items))
	}
	if _, ok := items["webhook.example.com"]; !ok {
		t.Fatal("host was not normalized")
	}
}
func TestPublicIPRejectsLocalNetworks(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.1", "192.168.1.1", "169.254.169.254", "::1", "fc00::1"} {
		if publicIP(net.ParseIP(raw)) {
			t.Fatalf("%s must be blocked", raw)
		}
	}
	if !publicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public address was rejected")
	}
}

func TestMonitorCancellationStopsRunningJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("monitor omitted runner authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"cancelled"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan struct{})
	done := make(chan struct{})
	go monitorCancellation(ctx, server.Client(), server.URL, "test-token", "execution-id", cancel, cancelled, done)

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("cancelled execution was not observed")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("running job context was not cancelled")
	}
	<-done
}
