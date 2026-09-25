package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type workspaceSecret struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	SecretType string    `json:"secretType" db:"secret_type"`
	Version    int       `json:"version" db:"version"`
	Enabled    bool      `json:"enabled" db:"enabled"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time `json:"updatedAt" db:"updated_at"`
}

func secretCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("WORK_GRAPH_MASTER_KEY")))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func encryptSecret(workspaceID, value string) ([]byte, []byte, error) {
	aead, err := secretCipher()
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return aead.Seal(nil, nonce, []byte(value), []byte(workspaceID)), nonce, nil
}

func decryptSecret(workspaceID string, ciphertext, nonce []byte) (string, error) {
	aead, err := secretCipher()
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(workspaceID))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *server) listWorkspaceSecrets(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id,name,secret_type,version,enabled,created_at,updated_at FROM workspace_secrets WHERE workspace_id=$1 ORDER BY name`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list workspace secrets", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[workspaceSecret])
	if err != nil {
		s.internalError(w, "read workspace secrets", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createWorkspaceSecret(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string `json:"name"`
		SecretType string `json:"secretType"`
		Value      string `json:"value"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.SecretType == "" {
		input.SecretType = "api_token"
	}
	if input.Name == "" || input.Value == "" || !map[string]bool{"api_token": true, "password": true, "signing_key": true, "other": true}[input.SecretType] {
		writeError(w, http.StatusBadRequest, "name, valid type, and value are required")
		return
	}
	ciphertext, nonce, err := encryptSecret(r.PathValue("workspaceID"), input.Value)
	input.Value = ""
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "secret encryption is not configured")
		return
	}
	current, _ := accountFromContext(r.Context())
	var item workspaceSecret
	err = s.db.QueryRow(r.Context(), `INSERT INTO workspace_secrets(workspace_id,name,secret_type,ciphertext,nonce,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,secret_type,version,enabled,created_at,updated_at`, r.PathValue("workspaceID"), input.Name, input.SecretType, ciphertext, nonce, current.ID).Scan(&item.ID, &item.Name, &item.SecretType, &item.Version, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		s.writeDatabaseError(w, "create workspace secret", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) rotateWorkspaceSecret(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Value string `json:"value"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Value == "" {
		writeError(w, http.StatusBadRequest, "new secret value is required")
		return
	}
	ciphertext, nonce, err := encryptSecret(r.PathValue("workspaceID"), input.Value)
	input.Value = ""
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "secret encryption is not configured")
		return
	}
	var item workspaceSecret
	err = s.db.QueryRow(r.Context(), `UPDATE workspace_secrets SET ciphertext=$3,nonce=$4,version=version+1,enabled=true,updated_at=now() WHERE workspace_id=$1 AND id=$2 RETURNING id,name,secret_type,version,enabled,created_at,updated_at`, r.PathValue("workspaceID"), r.PathValue("secretID"), ciphertext, nonce).Scan(&item.ID, &item.Name, &item.SecretType, &item.Version, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		s.writeDatabaseError(w, "rotate workspace secret", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) disableWorkspaceSecret(w http.ResponseWriter, r *http.Request) {
	var referenced bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id WHERE n.workspace_id=$1 AND s.configuration->>'secretId'=$2 AND s.step_status NOT IN ('completed','failed'))`, r.PathValue("workspaceID"), r.PathValue("secretID")).Scan(&referenced); err != nil {
		s.internalError(w, "check secret references", err)
		return
	}
	if referenced {
		writeError(w, http.StatusConflict, "secret is referenced by unfinished workflow steps")
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE workspace_secrets SET enabled=false,updated_at=now() WHERE workspace_id=$1 AND id=$2`, r.PathValue("workspaceID"), r.PathValue("secretID"))
	if err != nil {
		s.internalError(w, "disable workspace secret", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
