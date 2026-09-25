package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func stringPointer(value string) *string { return &value }

func TestGatewayAdminAuthentication(t *testing.T) {
	h := &Handler{logger: slog.Default(), config: Config{AdminToken: "secret"}}
	next := h.withAdminAuthentication(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	unauthorized := httptest.NewRecorder()
	next.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer secret")
	authorized := httptest.NewRecorder()
	next.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestBuildTreeOptionsKeepsHierarchyAndLinks(t *testing.T) {
	nodes := []CoreNode{
		{ID: "child-b", ParentID: stringPointer("root"), Title: "Build walls", Status: "planned"},
		{ID: "root", Title: "Build a house", Status: "active"},
		{ID: "child-a", ParentID: stringPointer("root"), Title: "Buy land", Status: "closed"},
	}
	options := buildTreeOptions(nodes, "https://work.example/", "workspace 1")
	if len(options) != 3 || options[0].ID != "root" || options[1].ID != "child-b" || options[2].ID != "child-a" {
		t.Fatalf("unexpected tree order: %#v", options)
	}
	if options[1].Prefix != "├─ " || options[1].Path != "Build a house / Build walls" {
		t.Fatalf("unexpected child presentation: %#v", options[1])
	}
	want := "https://work.example/?view=structure&workspace=workspace+1&node=child-b"
	if options[1].TreeURL != want {
		t.Fatalf("tree URL = %q, want %q", options[1].TreeURL, want)
	}
}

func TestRetryDelayUsesBoundedExponentialBackoff(t *testing.T) {
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute}
	for index, expected := range want {
		if actual := retryDelay(index + 1); actual != expected {
			t.Fatalf("retry %d delay = %s, want %s", index+1, actual, expected)
		}
	}
	if actual := retryDelay(20); actual != 15*time.Minute {
		t.Fatalf("bounded delay = %s", actual)
	}
}

func TestInstallationCredentialsAreBoundToInstallation(t *testing.T) {
	h := &Handler{config: Config{MasterKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))}}
	ciphertext, nonce, err := h.encryptCredentials("installation-a", []byte(`{"clientSecret":"very-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := h.decryptCredentials("installation-a", ciphertext, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != `{"clientSecret":"very-secret"}` {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if _, err = h.decryptCredentials("installation-b", ciphertext, nonce); err == nil {
		t.Fatal("credentials decrypted under another installation")
	}
}

func TestInstallationEncryptionRequiresValidMasterKey(t *testing.T) {
	h := &Handler{config: Config{MasterKey: "not-base64"}}
	if _, _, err := h.encryptCredentials("installation", []byte("secret")); err == nil {
		t.Fatal("encryption succeeded without a valid master key")
	}
}

func TestSensitiveValuesCannotBePublicInstallationConfiguration(t *testing.T) {
	if !containsSensitiveConfiguration(map[string]any{"nested": map[string]any{"accessToken": "secret"}}) {
		t.Fatal("nested token was accepted as public configuration")
	}
	if containsSensitiveConfiguration(map[string]any{"projectKeys": []any{"DEMO"}, "syncMode": "read_only"}) {
		t.Fatal("non-secret adapter configuration was rejected")
	}
}

func TestJiraCallbackURLRequiresSafePublicURL(t *testing.T) {
	h := &Handler{config: Config{PublicURL: "https://gateway.example/integrations/"}}
	callback, err := h.jiraCallbackURL()
	if err != nil || callback != "https://gateway.example/integrations/api/v1/oauth/jira/callback" {
		t.Fatalf("callback = %q, err = %v", callback, err)
	}
	h.config.PublicURL = "http://gateway.example"
	if _, err = h.jiraCallbackURL(); err == nil {
		t.Fatal("insecure non-local callback URL was accepted")
	}
}

func TestMatchingAtlassianResourceUsesConfiguredSite(t *testing.T) {
	resources := []atlassianResource{{ID: "other", URL: "https://other.atlassian.net"}, {ID: "wanted", URL: "https://TEAM.atlassian.net/", Name: "Team"}}
	resource, ok := matchingAtlassianResource(resources, "https://team.atlassian.net")
	if !ok || resource.ID != "wanted" {
		t.Fatalf("resource = %#v, found = %v", resource, ok)
	}
	if _, ok = matchingAtlassianResource(resources, "https://missing.atlassian.net"); ok {
		t.Fatal("unconfigured Jira site was selected")
	}
}

func TestJiraOAuthCallsUseFixedEndpointsAndBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			var body map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || body["client_secret"] != "secret" || body["code"] != "code" {
				t.Error("invalid token exchange request")
			}
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","scope":"read:jira-work","expires_in":3600}`))
		case "/oauth/token/accessible-resources":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`[{"id":"cloud","name":"Team","url":"https://team.atlassian.net","scopes":["read:jira-work"]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := &Handler{client: server.Client(), config: Config{AtlassianAuthURL: server.URL, AtlassianAPIURL: server.URL}}
	token, err := h.exchangeJiraCode(context.Background(), map[string]string{"clientId": "client", "clientSecret": "secret"}, "code", "https://gateway.example/callback")
	if err != nil || token.AccessToken != "access" || token.RefreshToken != "refresh" {
		t.Fatalf("token = %#v, err = %v", token, err)
	}
	resources, err := h.jiraResources(context.Background(), token.AccessToken)
	if err != nil || len(resources) != 1 || resources[0].ID != "cloud" {
		t.Fatalf("resources = %#v, err = %v", resources, err)
	}
}

func TestAtlassianErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("provider-secret-response"))
	}))
	defer server.Close()
	h := &Handler{client: server.Client(), config: Config{AtlassianAPIURL: server.URL}}
	_, err := h.jiraResources(context.Background(), "access")
	if err == nil || strings.Contains(err.Error(), "provider-secret-response") {
		t.Fatalf("unsafe error = %v", err)
	}
}

func TestJiraImportLimitIsBounded(t *testing.T) {
	if got := jiraImportLimit(map[string]any{}); got != 25 {
		t.Fatalf("default limit = %d", got)
	}
	if got := jiraImportLimit(map[string]any{"maxIssues": float64(1000)}); got != 100 {
		t.Fatalf("upper limit = %d", got)
	}
	if got := jiraImportLimit(map[string]any{"maxIssues": "0"}); got != 1 {
		t.Fatalf("lower limit = %d", got)
	}
}

func TestJiraPlainTextExtractsADFWithoutMetadata(t *testing.T) {
	description := map[string]any{"type": "doc", "version": float64(1), "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "First line"}}},
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Second line"}}},
	}}
	if got := jiraPlainText(description); got != "First line\nSecond line" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestJiraEnhancedSearchUsesPaginationAndLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/ex/jira/cloud-id/rest/api/3/search/jql" || r.Header.Get("Authorization") != "Bearer access" {
			t.Errorf("unexpected Jira request %s, authorization %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			JQL           string `json:"jql"`
			MaxResults    int    `json:"maxResults"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JQL != "project = DEMO" {
			t.Errorf("invalid search body: %#v, %v", body, err)
		}
		if requests == 1 {
			_, _ = w.Write([]byte(`{"issues":[{"id":"1","key":"DEMO-1","fields":{"summary":"One"}}],"nextPageToken":"next","isLast":false}`))
			return
		}
		if body.NextPageToken != "next" || body.MaxResults != 1 {
			t.Errorf("second page = %#v", body)
		}
		_, _ = w.Write([]byte(`{"issues":[{"id":"2","key":"DEMO-2","fields":{"summary":"Two"}}],"isLast":true}`))
	}))
	defer server.Close()
	h := &Handler{client: server.Client(), config: Config{AtlassianAPIURL: server.URL}}
	issues, _, last, err := h.searchJiraIssues(context.Background(), "cloud-id", "access", "project = DEMO", "", 2)
	if err != nil || len(issues) != 2 || requests != 2 || !last {
		t.Fatalf("issues = %#v, requests = %d, err = %v", issues, requests, err)
	}
}

func TestRefreshJiraTokenRequiresRotatedRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "old-refresh" {
			t.Errorf("refresh body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()
	h := &Handler{client: server.Client(), config: Config{AtlassianAuthURL: server.URL}}
	token, err := h.refreshJiraToken(context.Background(), map[string]string{"clientId": "client", "clientSecret": "secret", "refreshToken": "old-refresh"})
	if err != nil || token.AccessToken != "new-access" || token.RefreshToken != "new-refresh" {
		t.Fatalf("token = %#v, err = %v", token, err)
	}
}

func TestSyncRetryDelayBacksOffAndCaps(t *testing.T) {
	if got := syncRetryDelay(5*time.Minute, 0); got != 10*time.Minute {
		t.Fatalf("first retry = %s", got)
	}
	if got := syncRetryDelay(5*time.Minute, 20); got != time.Hour {
		t.Fatalf("capped retry = %s", got)
	}
}

func TestGatewayRejectsCrossOriginMutation(t *testing.T) {
	h := &Handler{logger: slog.Default(), config: Config{AdminToken: "secret"}}
	next := h.withAdminAuthentication(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodPost, "http://gateway.local/action", nil)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	next.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", recorder.Code)
	}
}
