package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceExpectedRevision(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), servicePrincipalContextKey{}, servicePrincipal{ID: "service"}))
	recorder := httptest.NewRecorder()
	if _, valid := serviceExpectedRevision(recorder, request); valid || recorder.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing precondition: valid=%t status=%d", valid, recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("If-Match", `"42"`)
	request = request.WithContext(context.WithValue(request.Context(), servicePrincipalContextKey{}, servicePrincipal{ID: "service"}))
	recorder = httptest.NewRecorder()
	expected, valid := serviceExpectedRevision(recorder, request)
	if !valid || expected == nil || *expected != 42 {
		t.Fatalf("quoted revision = %v, valid=%t", expected, valid)
	}
}

func TestHumanWriteDoesNotRequireRevision(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	expected, valid := serviceExpectedRevision(httptest.NewRecorder(), request)
	if !valid || expected != nil {
		t.Fatalf("human precondition = %v, valid=%t", expected, valid)
	}
}
