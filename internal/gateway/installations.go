package gateway

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Installation struct {
	ID                  string         `json:"id"`
	Provider            string         `json:"provider"`
	InstallationKey     string         `json:"installationKey"`
	DisplayName         string         `json:"displayName"`
	BaseURL             string         `json:"baseUrl"`
	AuthType            string         `json:"authType"`
	Status              string         `json:"status"`
	Configuration       map[string]any `json:"configuration"`
	HasCredentials      bool           `json:"hasCredentials"`
	CredentialVersion   int            `json:"credentialVersion"`
	CredentialUpdatedAt *time.Time     `json:"credentialUpdatedAt"`
	LastError           string         `json:"lastError"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	SyncEnabled         bool           `json:"syncEnabled"`
	NextSyncAt          *time.Time     `json:"nextSyncAt"`
	LastSyncAt          *time.Time     `json:"lastSyncAt"`
	SyncFailures        int            `json:"syncFailures"`
	SyncError           string         `json:"syncError"`
	SyncIntervalMinutes int            `json:"syncIntervalMinutes"`
	RecentRuns          []syncRun      `json:"recentRuns"`
}

const installationColumns = `id,provider,installation_key,display_name,base_url,auth_type,installation_status,configuration,(credential_ciphertext IS NOT NULL),credential_version,credential_updated_at,last_error,created_at,updated_at`

func (h *Handler) credentialCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h.config.MasterKey))
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("gateway master key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (h *Handler) encryptCredentials(installationID string, value []byte) ([]byte, []byte, error) {
	aead, err := h.credentialCipher()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return aead.Seal(nil, nonce, value, []byte(installationID)), nonce, nil
}

func (h *Handler) decryptCredentials(installationID string, ciphertext, nonce []byte) ([]byte, error) {
	aead, err := h.credentialCipher()
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, []byte(installationID))
}

func scanInstallation(row scanner) (Installation, error) {
	var item Installation
	var configuration []byte
	err := row.Scan(&item.ID, &item.Provider, &item.InstallationKey, &item.DisplayName, &item.BaseURL, &item.AuthType, &item.Status, &configuration, &item.HasCredentials, &item.CredentialVersion, &item.CredentialUpdatedAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(configuration, &item.Configuration)
	}
	return item, err
}

func (h *Handler) installations(r *http.Request) ([]Installation, error) {
	rows, err := h.db.Query(r.Context(), `SELECT `+installationColumns+` FROM gateway_installations ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Installation{}
	for rows.Next() {
		item, scanErr := scanInstallation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	schedules, err := h.db.Query(r.Context(), `SELECT installation_id::text,enabled,next_run_at,last_completed_at,consecutive_failures,last_error,interval_seconds/60 FROM gateway_sync_schedules`)
	if err != nil {
		return nil, err
	}
	defer schedules.Close()
	byID := map[string]*Installation{}
	for index := range items {
		byID[items[index].ID] = &items[index]
	}
	for schedules.Next() {
		var id string
		var enabled bool
		var next, completed *time.Time
		var failures int
		var syncError string
		var intervalMinutes int
		if err = schedules.Scan(&id, &enabled, &next, &completed, &failures, &syncError, &intervalMinutes); err != nil {
			return nil, err
		}
		if item := byID[id]; item != nil {
			item.SyncEnabled, item.NextSyncAt, item.LastSyncAt, item.SyncFailures, item.SyncError = enabled, next, completed, failures, syncError
			item.SyncIntervalMinutes = intervalMinutes
		}
	}
	if err = schedules.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		items[index].RecentRuns, err = h.syncRuns(r.Context(), items[index].ID, 5)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (h *Handler) listInstallations(w http.ResponseWriter, r *http.Request) {
	items, err := h.installations(r)
	if err != nil {
		h.internal(w, "list gateway installations", err)
		return
	}
	writeGatewayJSON(w, http.StatusOK, items)
}

func (h *Handler) createInstallation(w http.ResponseWriter, r *http.Request) {
	input := struct {
		Provider, InstallationKey, DisplayName, BaseURL, AuthType string
		Configuration                                             map[string]any
		Credentials                                               map[string]string
	}{}
	form := strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	if form {
		if err := r.ParseForm(); err != nil {
			writeGatewayError(w, http.StatusBadRequest, "invalid form")
			return
		}
		input.Provider = "jira_cloud"
		input.DisplayName = r.FormValue("displayName")
		input.BaseURL = r.FormValue("baseUrl")
		input.AuthType = "oauth2"
		input.Configuration = map[string]any{"jql": strings.TrimSpace(r.FormValue("jql")), "maxIssues": 25, "syncMode": "read_only"}
		input.Credentials = map[string]string{"clientId": r.FormValue("clientId"), "clientSecret": r.FormValue("clientSecret")}
	} else if !decodeGatewayJSON(w, r, &input) {
		return
	}
	input.Provider = strings.TrimSpace(input.Provider)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.AuthType = strings.TrimSpace(input.AuthType)
	if input.InstallationKey == "" {
		input.InstallationKey = input.BaseURL
	}
	credentialsValid := len(input.Credentials) > 0
	for _, value := range input.Credentials {
		if strings.TrimSpace(value) == "" {
			credentialsValid = false
		}
	}
	parsed, err := url.Parse(input.BaseURL)
	if input.Provider == "" || input.DisplayName == "" || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || !map[string]bool{"oauth2": true, "api_token": true, "basic": true, "none": true}[input.AuthType] {
		writeGatewayError(w, http.StatusBadRequest, "provider, name, HTTPS base URL, and valid authentication type are required")
		return
	}
	if input.AuthType != "none" && !credentialsValid {
		writeGatewayError(w, http.StatusBadRequest, "credentials are required for this authentication type")
		return
	}
	if containsSensitiveConfiguration(input.Configuration) {
		writeGatewayError(w, http.StatusBadRequest, "secret, token, password, and credential values belong in credentials, not configuration")
		return
	}
	var id string
	if err = h.db.QueryRow(r.Context(), `SELECT uuidv7()::text`).Scan(&id); err != nil {
		h.internal(w, "allocate installation id", err)
		return
	}
	var ciphertext, nonce []byte
	if input.AuthType != "none" {
		encoded, _ := json.Marshal(input.Credentials)
		ciphertext, nonce, err = h.encryptCredentials(id, encoded)
		clear(encoded)
		input.Credentials = nil
		if err != nil {
			writeGatewayError(w, http.StatusServiceUnavailable, "gateway credential encryption is not configured")
			return
		}
	}
	configuration, _ := json.Marshal(input.Configuration)
	var item Installation
	err = h.db.QueryRow(r.Context(), `INSERT INTO gateway_installations(id,provider,installation_key,display_name,base_url,auth_type,configuration,credential_ciphertext,credential_nonce,credential_version,credential_updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,CASE WHEN $8::bytea IS NULL THEN 0 ELSE 1 END,CASE WHEN $8::bytea IS NULL THEN NULL ELSE now() END) RETURNING `+installationColumns, id, input.Provider, input.InstallationKey, input.DisplayName, input.BaseURL, input.AuthType, configuration, ciphertext, nonce).Scan(&item.ID, &item.Provider, &item.InstallationKey, &item.DisplayName, &item.BaseURL, &item.AuthType, &item.Status, &configuration, &item.HasCredentials, &item.CredentialVersion, &item.CredentialUpdatedAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		h.internal(w, "create gateway installation", err)
		return
	}
	_ = json.Unmarshal(configuration, &item.Configuration)
	if form {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeGatewayJSON(w, http.StatusCreated, item)
}

func containsSensitiveConfiguration(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
				return true
			}
			if containsSensitiveConfiguration(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsSensitiveConfiguration(child) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) disableInstallation(w http.ResponseWriter, r *http.Request) {
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		h.internal(w, "disable gateway installation", err)
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), `UPDATE gateway_installations SET installation_status='disabled',credential_ciphertext=NULL,credential_nonce=NULL,credential_version=credential_version+1,credential_updated_at=now(),updated_at=now() WHERE id::text=$1`, r.PathValue("installationID"))
	if err != nil {
		h.internal(w, "disable gateway installation", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeGatewayError(w, http.StatusNotFound, "installation not found")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE gateway_sync_schedules SET enabled=false,lease_until=NULL,updated_at=now() WHERE installation_id::text=$1`, r.PathValue("installationID")); err != nil {
		h.internal(w, "disable gateway synchronization", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.internal(w, "disable gateway installation", err)
		return
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readInstallationCredentials(r *http.Request, installationID string) (map[string]string, error) {
	return h.readInstallationCredentialsFromContext(r.Context(), installationID)
}
