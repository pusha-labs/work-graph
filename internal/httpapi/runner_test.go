package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestRunnerAuthorization(t *testing.T) {
	t.Setenv("RUNNER_TOKEN", "test-token")
	request := httptest.NewRequest("POST", "/api/v1/runner/http/lease", nil)
	if runnerAuthorized(request) {
		t.Fatal("missing bearer token was accepted")
	}
	request.Header.Set("Authorization", "Bearer wrong")
	if runnerAuthorized(request) {
		t.Fatal("wrong bearer token was accepted")
	}
	request.Header.Set("Authorization", "Bearer test-token")
	if !runnerAuthorized(request) {
		t.Fatal("valid bearer token was rejected")
	}
}
func TestRunnerDisabledWithoutToken(t *testing.T) {
	t.Setenv("RUNNER_TOKEN", "")
	request := httptest.NewRequest("POST", "/api/v1/runner/http/lease", nil)
	request.Header.Set("Authorization", "Bearer ")
	if runnerAuthorized(request) {
		t.Fatal("runner endpoint must stay disabled without a configured token")
	}
}
