package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type jiraIssue struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Fields map[string]any `json:"fields"`
}

type jiraSearchPage struct {
	Issues        []jiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
	IsLast        bool        `json:"isLast"`
}

type jiraSyncResult struct {
	Imported int `json:"imported"`
	Read     int `json:"read"`
	Limit    int `json:"limit"`
}

var jqlOrderBy = regexp.MustCompile(`(?i)\s+ORDER\s+BY\s+.+$`)

func (h *Handler) syncJiraInstallation(w http.ResponseWriter, r *http.Request) {
	runID := h.startSyncRun(r.Context(), r.PathValue("installationID"), "manual")
	result, err := h.syncJira(r.Context(), r.PathValue("installationID"))
	h.finishSyncRun(r.Context(), runID, result, err)
	if err != nil {
		h.logger.Error("synchronize Jira installation", "installation_id", r.PathValue("installationID"), "error", err)
		_, _ = h.db.Exec(r.Context(), `UPDATE gateway_installations SET last_error='Jira synchronization failed; try again or wait for the automatic retry',updated_at=now() WHERE id::text=$1`, r.PathValue("installationID"))
		writeGatewayError(w, http.StatusBadGateway, "Jira synchronization could not be completed")
		return
	}
	_, _ = h.db.Exec(r.Context(), `UPDATE gateway_sync_schedules SET next_run_at=CASE WHEN continuation_token<>'' THEN now() ELSE now()+(interval_seconds*interval '1 second') END,lease_until=NULL,last_completed_at=now(),last_error='',consecutive_failures=0,updated_at=now() WHERE installation_id::text=$1`, r.PathValue("installationID"))
	if strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeGatewayJSON(w, http.StatusOK, result)
}

func (h *Handler) syncJira(ctx context.Context, installationID string) (jiraSyncResult, error) {
	lock, err := h.db.Begin(ctx)
	if err != nil {
		return jiraSyncResult{}, fmt.Errorf("start synchronization lock: %w", err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, installationID); err != nil {
		return jiraSyncResult{}, fmt.Errorf("lock Jira synchronization: %w", err)
	}
	var provider, installationKey, status string
	var configurationJSON []byte
	err = h.db.QueryRow(ctx, `SELECT provider,installation_key,installation_status,configuration FROM gateway_installations WHERE id::text=$1`, installationID).Scan(&provider, &installationKey, &status, &configurationJSON)
	if err != nil {
		return jiraSyncResult{}, fmt.Errorf("read installation: %w", err)
	}
	if provider != "jira_cloud" || status != "connected" {
		return jiraSyncResult{}, fmt.Errorf("installation is not connected Jira Cloud")
	}
	configuration := map[string]any{}
	_ = json.Unmarshal(configurationJSON, &configuration)
	cloudID, _ := configuration["cloudId"].(string)
	if cloudID == "" {
		return jiraSyncResult{}, fmt.Errorf("verified Jira cloud ID is unavailable")
	}
	accessToken, err := h.jiraAccessToken(ctx, installationID)
	if err != nil {
		return jiraSyncResult{}, fmt.Errorf("get Jira access token: %w", err)
	}
	jql := ""
	if configured, ok := configuration["jql"].(string); ok && strings.TrimSpace(configured) != "" {
		jql = strings.TrimSpace(configured)
	}
	var cursorAt, cycleUpper *time.Time
	var continuation, cycleQuery string
	_ = h.db.QueryRow(ctx, `SELECT cursor_at,continuation_token,cycle_query,cycle_upper_bound FROM gateway_sync_schedules WHERE installation_id::text=$1`, installationID).Scan(&cursorAt, &continuation, &cycleQuery, &cycleUpper)
	if continuation == "" || cycleQuery == "" || cycleUpper == nil {
		upper := time.Now().UTC().Truncate(time.Minute)
		cycleUpper = &upper
		filter := strings.TrimSpace(jqlOrderBy.ReplaceAllString(jql, ""))
		clauses := []string{}
		if filter != "" {
			clauses = append(clauses, "("+filter+")")
		}
		if cursorAt != nil {
			clauses = append(clauses, `updated >= "`+cursorAt.Add(-2*time.Minute).UTC().Format("2006-01-02 15:04")+`"`)
		}
		clauses = append(clauses, `updated <= "`+upper.Format("2006-01-02 15:04")+`"`)
		cycleQuery = strings.Join(clauses, " AND ") + " ORDER BY updated ASC"
		continuation = ""
		_, _ = h.db.Exec(ctx, `UPDATE gateway_sync_schedules SET cycle_query=$2,cycle_upper_bound=$3,continuation_token='' WHERE installation_id::text=$1`, installationID, cycleQuery, upper)
	}
	limit := jiraImportLimit(configuration)
	issues, nextToken, isLast, err := h.searchJiraIssues(ctx, cloudID, accessToken, cycleQuery, continuation, limit)
	if err != nil {
		return jiraSyncResult{}, fmt.Errorf("read Jira issues: %w", err)
	}
	imported := 0
	for _, issue := range issues {
		title, _ := issue.Fields["summary"].(string)
		if issue.Key == "" || strings.TrimSpace(title) == "" {
			continue
		}
		version, _ := issue.Fields["updated"].(string)
		if version == "" {
			version = issue.ID
		}
		description := jiraPlainText(issue.Fields["description"])
		if _, err = h.upsertCandidate(ctx, "jira_cloud", installationKey, issue.Key, version, title, description, issue); err != nil {
			return jiraSyncResult{}, fmt.Errorf("save Jira candidate %s: %w", issue.Key, err)
		}
		imported++
	}
	if _, err = h.db.Exec(ctx, `UPDATE gateway_installations SET installation_status='connected',last_error='',configuration=jsonb_set(configuration,'{lastSyncAt}',to_jsonb(now()::text),true),updated_at=now() WHERE id::text=$1`, installationID); err != nil {
		return jiraSyncResult{}, fmt.Errorf("record Jira synchronization: %w", err)
	}
	if isLast || nextToken == "" {
		_, err = h.db.Exec(ctx, `UPDATE gateway_sync_schedules SET cursor_at=$2,continuation_token='',cycle_query='',cycle_upper_bound=NULL WHERE installation_id::text=$1`, installationID, cycleUpper)
	} else {
		_, err = h.db.Exec(ctx, `UPDATE gateway_sync_schedules SET continuation_token=$2 WHERE installation_id::text=$1`, installationID, nextToken)
	}
	if err != nil {
		return jiraSyncResult{}, fmt.Errorf("advance Jira synchronization cursor: %w", err)
	}
	if err = lock.Commit(ctx); err != nil {
		return jiraSyncResult{}, fmt.Errorf("release synchronization lock: %w", err)
	}
	return jiraSyncResult{Imported: imported, Read: len(issues), Limit: limit}, nil
}

func jiraImportLimit(configuration map[string]any) int {
	limit := 25
	switch value := configuration["maxIssues"].(type) {
	case float64:
		limit = int(value)
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (h *Handler) jiraAccessToken(ctx context.Context, installationID string) (string, error) {
	credentials, err := h.readInstallationCredentialsFromContext(ctx, installationID)
	if err != nil {
		return "", err
	}
	expiresAt, _ := time.Parse(time.RFC3339, credentials["expiresAt"])
	if credentials["accessToken"] != "" && expiresAt.After(time.Now().Add(time.Minute)) {
		return credentials["accessToken"], nil
	}
	if credentials["refreshToken"] == "" {
		return "", fmt.Errorf("Jira refresh token is unavailable")
	}
	token, err := h.refreshJiraToken(ctx, credentials)
	if err != nil {
		return "", err
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
		return "", err
	}
	result, err := h.db.Exec(ctx, `UPDATE gateway_installations SET credential_ciphertext=$2,credential_nonce=$3,credential_version=credential_version+1,credential_updated_at=now(),updated_at=now() WHERE id::text=$1 AND installation_status='connected'`, installationID, ciphertext, nonce)
	if err != nil {
		return "", err
	}
	if result.RowsAffected() != 1 {
		return "", fmt.Errorf("Jira installation is no longer connected")
	}
	return token.AccessToken, nil
}

func (h *Handler) readInstallationCredentialsFromContext(ctx context.Context, installationID string) (map[string]string, error) {
	var ciphertext, nonce []byte
	if err := h.db.QueryRow(ctx, `SELECT credential_ciphertext,credential_nonce FROM gateway_installations WHERE id::text=$1`, installationID).Scan(&ciphertext, &nonce); err != nil {
		return nil, err
	}
	plaintext, err := h.decryptCredentials(installationID, ciphertext, nonce)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	result := map[string]string{}
	if err = json.Unmarshal(plaintext, &result); err != nil {
		return nil, fmt.Errorf("decode installation credentials: %w", err)
	}
	return result, nil
}

func (h *Handler) refreshJiraToken(ctx context.Context, credentials map[string]string) (jiraTokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "refresh_token", "client_id": credentials["clientId"], "client_secret": credentials["clientSecret"], "refresh_token": credentials["refreshToken"]})
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
		return token, fmt.Errorf("incomplete refresh response")
	}
	return token, nil
}

func (h *Handler) searchJiraIssues(ctx context.Context, cloudID, accessToken, jql, continuation string, limit int) ([]jiraIssue, string, bool, error) {
	endpoint := strings.TrimRight(h.config.AtlassianAPIURL, "/") + "/ex/jira/" + url.PathEscape(cloudID) + "/rest/api/3/search/jql"
	issues := make([]jiraIssue, 0, limit)
	nextPageToken := continuation
	last := false
	for len(issues) < limit {
		pageSize := limit - len(issues)
		if pageSize > 50 {
			pageSize = 50
		}
		payload := map[string]any{"jql": jql, "maxResults": pageSize, "fields": []string{"summary", "description", "updated", "status", "project", "assignee"}}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, _ := json.Marshal(payload)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, "", false, err
		}
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Content-Type", "application/json")
		var page jiraSearchPage
		if err = h.doAtlassianJSON(request, &page); err != nil {
			return nil, "", false, err
		}
		issues = append(issues, page.Issues...)
		last = page.IsLast
		if page.IsLast || page.NextPageToken == "" || len(page.Issues) == 0 {
			nextPageToken = page.NextPageToken
			break
		}
		nextPageToken = page.NextPageToken
	}
	if len(issues) > limit {
		issues = issues[:limit]
	}
	return issues, nextPageToken, last, nil
}

func jiraPlainText(value any) string {
	parts := []string{}
	var visit func(any)
	visit = func(node any) {
		switch typed := node.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				parts = append(parts, strings.TrimSpace(typed))
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		case map[string]any:
			if text, ok := typed["text"].(string); ok {
				visit(text)
				return
			}
			if content, ok := typed["content"]; ok {
				visit(content)
			}
		}
	}
	visit(value)
	return strings.Join(parts, "\n")
}
