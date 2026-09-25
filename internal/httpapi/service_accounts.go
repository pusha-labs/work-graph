package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type serviceAccount struct {
	ID          string     `json:"id" db:"id"`
	ActorID     string     `json:"actorId" db:"actor_id"`
	Name        string     `json:"name" db:"name"`
	TokenPrefix string     `json:"tokenPrefix" db:"token_prefix"`
	Scopes      []string   `json:"scopes" db:"scopes"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	LastUsedAt  *time.Time `json:"lastUsedAt" db:"last_used_at"`
	RevokedAt   *time.Time `json:"revokedAt" db:"revoked_at"`
}

type servicePrincipal struct {
	ID          string
	WorkspaceID string
	ActorID     string
	DisplayName string
	Scopes      []string
}

type servicePrincipalContextKey struct{}

func servicePrincipalFromContext(r *http.Request) (servicePrincipal, bool) {
	value, ok := r.Context().Value(servicePrincipalContextKey{}).(servicePrincipal)
	return value, ok
}

func (principal servicePrincipal) hasScope(scope string) bool {
	for _, candidate := range principal.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func (s *server) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id,actor_id,name,token_prefix,scopes,created_at,last_used_at,revoked_at
		FROM service_accounts WHERE workspace_id=$1 ORDER BY created_at`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list service accounts", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[serviceAccount])
	if err != nil {
		s.internalError(w, "read service accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createServiceAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	scopes, valid := normalizeServiceScopes(input.Scopes)
	if input.Name == "" || !valid {
		writeError(w, http.StatusBadRequest, "name and at least one valid scope are required")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM service_accounts WHERE workspace_id=$1 AND lower(name)=lower($2))`, workspaceID, input.Name).Scan(&exists); err != nil {
		s.internalError(w, "check service account name", err)
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "a service account with this name already exists")
		return
	}
	token, tokenHash, tokenPrefix, err := newServiceAccountToken()
	if err != nil {
		s.internalError(w, "create service token", err)
		return
	}
	current, _ := accountFromContext(r.Context())
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin service account creation", err)
		return
	}
	defer tx.Rollback(r.Context())
	var result serviceAccount
	if err := tx.QueryRow(r.Context(), `INSERT INTO actors(workspace_id,display_name,actor_type) VALUES ($1,$2,'automation') RETURNING id`, workspaceID, input.Name).Scan(&result.ActorID); err != nil {
		s.internalError(w, "create service actor", err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO service_accounts(workspace_id,actor_id,name,token_hash,token_prefix,scopes,created_by_account_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id,actor_id,name,token_prefix,scopes,created_at,last_used_at,revoked_at`,
		workspaceID, result.ActorID, input.Name, tokenHash[:], tokenPrefix, scopes, current.ID).Scan(
		&result.ID, &result.ActorID, &result.Name, &result.TokenPrefix, &result.Scopes, &result.CreatedAt, &result.LastUsedAt, &result.RevokedAt,
	); err != nil {
		s.internalError(w, "save service account", err)
		return
	}
	if err := recordAudit(r.Context(), tx, workspaceID, current.ID, "service_account.created", "Service account created", result.Name); err != nil {
		s.internalError(w, "audit service account creation", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit service account creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"serviceAccount": result, "token": token})
}

func (s *server) rotateServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	token, tokenHash, tokenPrefix, err := newServiceAccountToken()
	if err != nil {
		s.internalError(w, "create replacement service token", err)
		return
	}
	workspaceID, serviceAccountID := r.PathValue("workspaceID"), r.PathValue("serviceAccountID")
	current, _ := accountFromContext(r.Context())
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin service token rotation", err)
		return
	}
	defer tx.Rollback(r.Context())
	var result serviceAccount
	err = tx.QueryRow(r.Context(), `
		UPDATE service_accounts SET token_hash=$3,token_prefix=$4,revoked_at=NULL,last_used_at=NULL
		WHERE workspace_id=$1 AND id=$2
		RETURNING id,actor_id,name,token_prefix,scopes,created_at,last_used_at,revoked_at`,
		workspaceID, serviceAccountID, tokenHash[:], tokenPrefix).Scan(
		&result.ID, &result.ActorID, &result.Name, &result.TokenPrefix, &result.Scopes, &result.CreatedAt, &result.LastUsedAt, &result.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	if err != nil {
		s.internalError(w, "rotate service token", err)
		return
	}
	if err := recordAudit(r.Context(), tx, workspaceID, current.ID, "service_account.rotated", "Service token rotated", result.Name); err != nil {
		s.internalError(w, "audit service token rotation", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit service token rotation", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"serviceAccount": result, "token": token})
}

func (s *server) revokeServiceAccount(w http.ResponseWriter, r *http.Request) {
	workspaceID, serviceAccountID := r.PathValue("workspaceID"), r.PathValue("serviceAccountID")
	current, _ := accountFromContext(r.Context())
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin service account revocation", err)
		return
	}
	defer tx.Rollback(r.Context())
	var name string
	err = tx.QueryRow(r.Context(), `UPDATE service_accounts SET revoked_at=COALESCE(revoked_at,now()) WHERE workspace_id=$1 AND id=$2 RETURNING name`, workspaceID, serviceAccountID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	if err != nil {
		s.internalError(w, "revoke service account", err)
		return
	}
	if err := recordAudit(r.Context(), tx, workspaceID, current.ID, "service_account.revoked", "Service account revoked", name); err != nil {
		s.internalError(w, "audit service account revocation", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit service account revocation", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeServiceScopes(input []string) ([]string, bool) {
	allowed := map[string]bool{"work:read": true, "work:write": true}
	unique := map[string]bool{}
	for _, scope := range input {
		if !allowed[scope] {
			return nil, false
		}
		unique[scope] = true
	}
	if len(unique) == 0 {
		return nil, false
	}
	result := make([]string, 0, len(unique))
	for scope := range unique {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result, true
}

func newServiceAccountToken() (string, [32]byte, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", [32]byte{}, "", err
	}
	token := "wgsa_" + base64.RawURLEncoding.EncodeToString(bytes)
	prefix := token[:13]
	return token, sha256.Sum256([]byte(token)), prefix, nil
}
