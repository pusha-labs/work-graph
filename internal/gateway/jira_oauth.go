package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const jiraOAuthScopes = "read:jira-work offline_access"

type jiraTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

type atlassianResource struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Scopes []string `json:"scopes"`
}

func (h *Handler) jiraCallbackURL() (string, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(h.config.PublicURL), "/"))
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "https" && !(base.Scheme == "http" && (base.Hostname() == "localhost" || base.Hostname() == "127.0.0.1"))) {
		return "", fmt.Errorf("GATEWAY_PUBLIC_URL must be HTTPS (HTTP is allowed for localhost)")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/oauth/jira/callback"
	base.RawQuery, base.Fragment = "", ""
	return base.String(), nil
}

func (h *Handler) startJiraOAuth(w http.ResponseWriter, r *http.Request) {
	installationID := r.PathValue("installationID")
	var provider, status string
	if err := h.db.QueryRow(r.Context(), `SELECT provider,installation_status FROM gateway_installations WHERE id::text=$1`, installationID).Scan(&provider, &status); err != nil {
		writeGatewayError(w, http.StatusNotFound, "installation not found")
		return
	}
	if provider != "jira_cloud" || status == "disabled" {
		writeGatewayError(w, http.StatusConflict, "installation cannot start Jira OAuth")
		return
	}
	credentials, err := h.readInstallationCredentials(r, installationID)
	if err != nil || strings.TrimSpace(credentials["clientId"]) == "" || strings.TrimSpace(credentials["clientSecret"]) == "" {
		writeGatewayError(w, http.StatusConflict, "Jira OAuth client credentials are unavailable")
		return
	}
	redirectURI, err := h.jiraCallbackURL()
	if err != nil {
		writeGatewayError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	rawState := make([]byte, 32)
	if _, err = rand.Read(rawState); err != nil {
		h.internal(w, "generate OAuth state", err)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(rawState)
	stateHash := sha256.Sum256([]byte(state))
	if _, err = h.db.Exec(r.Context(), `DELETE FROM gateway_oauth_states WHERE expires_at<=now()`); err == nil {
		_, err = h.db.Exec(r.Context(), `INSERT INTO gateway_oauth_states(state_hash,installation_id,redirect_uri,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')`, stateHash[:], installationID, redirectURI)
	}
	if err != nil {
		h.internal(w, "store OAuth state", err)
		return
	}
	authorize, err := url.Parse(strings.TrimRight(h.config.AtlassianAuthURL, "/") + "/authorize")
	if err != nil {
		h.internal(w, "build Atlassian authorization URL", err)
		return
	}
	query := authorize.Query()
	query.Set("audience", "api.atlassian.com")
	query.Set("client_id", credentials["clientId"])
	query.Set("scope", jiraOAuthScopes)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("response_type", "code")
	query.Set("prompt", "consent")
	authorize.RawQuery = query.Encode()
	http.Redirect(w, r, authorize.String(), http.StatusFound)
}

func (h *Handler) finishJiraOAuth(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		writeGatewayError(w, http.StatusBadRequest, "missing OAuth state")
		return
	}
	stateHash := sha256.Sum256([]byte(state))
	var installationID, redirectURI string
	err := h.db.QueryRow(r.Context(), `DELETE FROM gateway_oauth_states WHERE state_hash=$1 AND expires_at>now() RETURNING installation_id::text,redirect_uri`, stateHash[:]).Scan(&installationID, &redirectURI)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, "OAuth state is invalid, expired, or already used")
		return
	}
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		h.markInstallationError(r.Context(), installationID, "authorization was denied")
		writeGatewayError(w, http.StatusBadRequest, "Jira authorization was not completed")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeGatewayError(w, http.StatusBadRequest, "missing authorization code")
		return
	}
	credentials, err := h.readInstallationCredentials(r, installationID)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "read Jira OAuth credentials", err)
		return
	}
	token, err := h.exchangeJiraCode(r.Context(), credentials, code, redirectURI)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "exchange Jira authorization code", err)
		return
	}
	resources, err := h.jiraResources(r.Context(), token.AccessToken)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "load accessible Jira sites", err)
		return
	}
	var baseURL string
	var existingConfiguration []byte
	if err = h.db.QueryRow(r.Context(), `SELECT base_url,configuration FROM gateway_installations WHERE id::text=$1`, installationID).Scan(&baseURL, &existingConfiguration); err != nil {
		h.oauthFailure(w, r.Context(), installationID, "load Jira installation", err)
		return
	}
	resource, ok := matchingAtlassianResource(resources, baseURL)
	if !ok {
		h.markInstallationError(r.Context(), installationID, "authorized account cannot access the configured Jira site")
		writeGatewayError(w, http.StatusBadRequest, "authorized account cannot access the configured Jira site")
		return
	}
	credentials["accessToken"] = token.AccessToken
	credentials["refreshToken"] = token.RefreshToken
	credentials["tokenType"] = token.TokenType
	credentials["scope"] = token.Scope
	credentials["expiresAt"] = time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339)
	encoded, _ := json.Marshal(credentials)
	ciphertext, nonce, err := h.encryptCredentials(installationID, encoded)
	clear(encoded)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "encrypt Jira OAuth tokens", err)
		return
	}
	publicConfiguration := map[string]any{}
	_ = json.Unmarshal(existingConfiguration, &publicConfiguration)
	publicConfiguration["cloudId"] = resource.ID
	publicConfiguration["siteName"] = resource.Name
	publicConfiguration["grantedScopes"] = resource.Scopes
	publicConfiguration["syncMode"] = "read_only"
	configuration, _ := json.Marshal(publicConfiguration)
	_, err = h.db.Exec(r.Context(), `UPDATE gateway_installations SET installation_status='connected',configuration=$2,credential_ciphertext=$3,credential_nonce=$4,credential_version=credential_version+1,credential_updated_at=now(),last_error='',updated_at=now() WHERE id::text=$1`, installationID, configuration, ciphertext, nonce)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "store Jira OAuth tokens", err)
		return
	}
	_, err = h.db.Exec(r.Context(), `INSERT INTO gateway_sync_schedules(installation_id,next_run_at) VALUES($1,now()) ON CONFLICT(installation_id) DO UPDATE SET enabled=true,next_run_at=now(),lease_until=NULL,last_error='',updated_at=now()`, installationID)
	if err != nil {
		h.oauthFailure(w, r.Context(), installationID, "schedule Jira synchronization", err)
		return
	}
	http.Redirect(w, r, "/?connected=jira", http.StatusSeeOther)
}

func (h *Handler) exchangeJiraCode(ctx context.Context, credentials map[string]string, code, redirectURI string) (jiraTokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "client_id": credentials["clientId"], "client_secret": credentials["clientSecret"], "code": code, "redirect_uri": redirectURI})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(h.config.AtlassianAuthURL, "/")+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return jiraTokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	var token jiraTokenResponse
	if err = h.doAtlassianJSON(request, &token); err != nil {
		return token, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return token, fmt.Errorf("incomplete token response")
	}
	return token, nil
}

func (h *Handler) jiraResources(ctx context.Context, accessToken string) ([]atlassianResource, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(h.config.AtlassianAPIURL, "/")+"/oauth/token/accessible-resources", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	var resources []atlassianResource
	if err = h.doAtlassianJSON(request, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (h *Handler) doAtlassianJSON(request *http.Request, destination any) error {
	response, err := h.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Atlassian returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(destination)
}

func matchingAtlassianResource(resources []atlassianResource, configured string) (atlassianResource, bool) {
	normalize := func(value string) string {
		parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
		if err != nil {
			return ""
		}
		return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + strings.TrimRight(parsed.EscapedPath(), "/")
	}
	want := normalize(configured)
	for _, resource := range resources {
		if resource.ID != "" && normalize(resource.URL) == want {
			return resource, true
		}
	}
	return atlassianResource{}, false
}

func (h *Handler) markInstallationError(ctx context.Context, installationID, message string) {
	_, _ = h.db.Exec(ctx, `UPDATE gateway_installations SET installation_status='error',last_error=$2,updated_at=now() WHERE id::text=$1`, installationID, message)
}

func (h *Handler) oauthFailure(w http.ResponseWriter, ctx context.Context, installationID, operation string, err error) {
	h.logger.Error(operation, "installation_id", installationID, "error", err)
	h.markInstallationError(ctx, installationID, "Jira authorization could not be completed")
	writeGatewayError(w, http.StatusBadGateway, "Jira authorization could not be completed")
}
