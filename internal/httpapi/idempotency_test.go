package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestServiceIdempotencyKey(t *testing.T) {
	request := httptest.NewRequest("POST", "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), servicePrincipalContextKey{}, servicePrincipal{ID: "service"}))
	recorder := httptest.NewRecorder()
	if _, _, valid := serviceIdempotencyKey(recorder, request); valid || recorder.Code != 400 {
		t.Fatalf("missing key: valid=%t status=%d", valid, recorder.Code)
	}

	request = httptest.NewRequest("POST", "/", nil)
	request.Header.Set(idempotencyKeyHeader, "jira:ABC-123:create")
	request = request.WithContext(context.WithValue(request.Context(), servicePrincipalContextKey{}, servicePrincipal{ID: "service"}))
	recorder = httptest.NewRecorder()
	principal, key, valid := serviceIdempotencyKey(recorder, request)
	if !valid || principal.ID != "service" || key != "jira:ABC-123:create" {
		t.Fatalf("unexpected key result: %#v %q %t", principal, key, valid)
	}
}

func TestRequestFingerprintIsStableAndContentSensitive(t *testing.T) {
	left, err := requestFingerprint(struct {
		Title string `json:"title"`
	}{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	repeated, _ := requestFingerprint(struct {
		Title string `json:"title"`
	}{Title: "One"})
	right, _ := requestFingerprint(struct {
		Title string `json:"title"`
	}{Title: "Two"})
	if left != repeated {
		t.Fatal("equal requests produced different fingerprints")
	}
	if left == right {
		t.Fatal("different requests produced the same fingerprint")
	}
}
