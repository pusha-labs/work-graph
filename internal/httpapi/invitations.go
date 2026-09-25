package httpapi

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type invitation struct {
	ID            string     `json:"id"`
	WorkspaceID   string     `json:"workspaceId"`
	WorkspaceName string     `json:"workspaceName,omitempty"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"displayName"`
	WorkspaceRole string     `json:"workspaceRole"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	AcceptedAt    *time.Time `json:"acceptedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func (s *server) createInvitation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email, DisplayName, WorkspaceRole string
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.WorkspaceRole == "" {
		input.WorkspaceRole = "member"
	}
	if !strings.Contains(input.Email, "@") || input.DisplayName == "" || (input.WorkspaceRole != "member" && input.WorkspaceRole != "admin") {
		writeError(w, http.StatusBadRequest, "valid name, email, and role are required")
		return
	}
	var alreadyMember bool
	if err := s.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM accounts account
			JOIN account_actors membership ON membership.account_id=account.id
			JOIN actors actor ON actor.id=membership.actor_id
			WHERE lower(account.email)=lower($1) AND actor.workspace_id=$2
		)`, input.Email, r.PathValue("workspaceID")).Scan(&alreadyMember); err != nil {
		s.internalError(w, "check workspace membership", err)
		return
	}
	if alreadyMember {
		writeError(w, http.StatusConflict, "this account is already a workspace member")
		return
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		s.internalError(w, "create invitation token", err)
		return
	}
	current, _ := accountFromContext(r.Context())
	var result invitation
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO workspace_invitations(workspace_id,email,display_name,workspace_role,token_hash,invited_by,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,now()+interval '7 days')
		RETURNING id,workspace_id,email,display_name,workspace_role,expires_at,accepted_at,created_at`,
		r.PathValue("workspaceID"), input.Email, input.DisplayName, input.WorkspaceRole, tokenHash[:], current.ID).Scan(
		&result.ID, &result.WorkspaceID, &result.Email, &result.DisplayName, &result.WorkspaceRole, &result.ExpiresAt, &result.AcceptedAt, &result.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "workspace_invitations_pending_email_idx") {
			writeError(w, http.StatusConflict, "a pending invitation already exists for this email")
			return
		}
		s.internalError(w, "create invitation", err)
		return
	}
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "invitation.created", "Invitation created", result.DisplayName+" · "+result.Email+" · "+result.WorkspaceRole); err != nil {
		s.internalError(w, "audit invitation creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"invitation": result, "token": token})
}

func (s *server) listInvitations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id,workspace_id,email,display_name,workspace_role,expires_at,accepted_at,created_at FROM workspace_invitations WHERE workspace_id=$1 ORDER BY created_at DESC`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list invitations", err)
		return
	}
	defer rows.Close()
	items := []invitation{}
	for rows.Next() {
		var item invitation
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Email, &item.DisplayName, &item.WorkspaceRole, &item.ExpiresAt, &item.AcceptedAt, &item.CreatedAt); err != nil {
			s.internalError(w, "read invitation", err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) invitationDetails(w http.ResponseWriter, r *http.Request) {
	hash := sha256.Sum256([]byte(r.PathValue("token")))
	var result invitation
	err := s.db.QueryRow(r.Context(), `SELECT i.id,i.workspace_id,w.name,i.email,i.display_name,i.workspace_role,i.expires_at,i.accepted_at,i.created_at FROM workspace_invitations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.token_hash=$1`, hash[:]).Scan(
		&result.ID, &result.WorkspaceID, &result.WorkspaceName, &result.Email, &result.DisplayName, &result.WorkspaceRole, &result.ExpiresAt, &result.AcceptedAt, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || result.AcceptedAt != nil || time.Now().After(result.ExpiresAt) {
		writeError(w, http.StatusNotFound, "invitation is invalid or expired")
		return
	}
	if err != nil {
		s.internalError(w, "find invitation", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	tokenHash := sha256.Sum256([]byte(r.PathValue("token")))
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin invitation acceptance", err)
		return
	}
	defer tx.Rollback(r.Context())
	var invite invitation
	err = tx.QueryRow(r.Context(), `SELECT id,workspace_id,email,display_name,workspace_role,expires_at,accepted_at,created_at FROM workspace_invitations WHERE token_hash=$1 FOR UPDATE`, tokenHash[:]).Scan(
		&invite.ID, &invite.WorkspaceID, &invite.Email, &invite.DisplayName, &invite.WorkspaceRole, &invite.ExpiresAt, &invite.AcceptedAt, &invite.CreatedAt)
	if err != nil || invite.AcceptedAt != nil || time.Now().After(invite.ExpiresAt) {
		writeError(w, http.StatusNotFound, "invitation is invalid or expired")
		return
	}
	current, signedIn := accountFromContext(r.Context())
	var result account
	var storedPasswordHash string
	err = tx.QueryRow(r.Context(), `SELECT id,email,display_name,password_hash FROM accounts WHERE lower(email)=lower($1)`, invite.Email).Scan(&result.ID, &result.Email, &result.DisplayName, &storedPasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		if signedIn {
			writeError(w, http.StatusForbidden, "this invitation belongs to a different account; sign out to accept it")
			return
		}
		if len(input.Password) < 10 {
			writeError(w, http.StatusBadRequest, "password must be at least 10 characters")
			return
		}
		passwordHash, hashErr := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			s.internalError(w, "hash invited password", hashErr)
			return
		}
		err = tx.QueryRow(r.Context(), `INSERT INTO accounts(email,display_name,password_hash) VALUES ($1,$2,$3) RETURNING id,email,display_name`, invite.Email, invite.DisplayName, string(passwordHash)).Scan(&result.ID, &result.Email, &result.DisplayName)
		if err != nil {
			s.internalError(w, "create invited account", err)
			return
		}
	} else if err != nil {
		s.internalError(w, "find invited account", err)
		return
	} else if signedIn {
		if current.ID != result.ID {
			writeError(w, http.StatusForbidden, "this invitation belongs to a different account; sign out to accept it")
			return
		}
	} else {
		retryAfter, limitErr := s.consumeAuthAttempt(r.Context(), "invitation_password", opaqueIdentifier(invite.Email+":"+clientIdentifier(r)), 8, 15*time.Minute, 15*time.Minute)
		if limitErr != nil {
			s.internalError(w, "limit invitation acceptance", limitErr)
			return
		}
		if !enforceAuthLimit(w, retryAfter) {
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(storedPasswordHash), []byte(input.Password)) != nil {
			writeError(w, http.StatusUnauthorized, "sign in with the invited account password")
			return
		}
		s.clearAuthAttempts(r.Context(), "invitation_password", opaqueIdentifier(invite.Email+":"+clientIdentifier(r)))
	}
	var alreadyMember bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM account_actors membership JOIN actors actor ON actor.id=membership.actor_id WHERE membership.account_id=$1 AND actor.workspace_id=$2)`, result.ID, invite.WorkspaceID).Scan(&alreadyMember); err != nil {
		s.internalError(w, "check invited membership", err)
		return
	}
	if alreadyMember {
		writeError(w, http.StatusConflict, "this account is already a workspace member")
		return
	}
	var actorID string
	err = tx.QueryRow(r.Context(), `
		WITH restored AS (
			UPDATE actors SET has_account=true
			WHERE workspace_id=$1 AND lower(display_name)=lower($2) AND has_account=false
			RETURNING id
		), created AS (
			INSERT INTO actors(workspace_id,display_name,has_account)
			SELECT $1,$2,true WHERE NOT EXISTS(SELECT 1 FROM restored)
			RETURNING id
		)
		SELECT id FROM restored UNION ALL SELECT id FROM created LIMIT 1`, invite.WorkspaceID, result.DisplayName).Scan(&actorID)
	if err != nil {
		s.internalError(w, "create invited actor", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO account_actors(account_id,actor_id,workspace_role) VALUES ($1,$2,$3)`, result.ID, actorID, invite.WorkspaceRole); err != nil {
		s.internalError(w, "link invited actor", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE workspace_invitations SET accepted_at=now() WHERE id=$1`, invite.ID); err != nil {
		s.internalError(w, "accept invitation", err)
		return
	}
	if err := recordAudit(r.Context(), tx, invite.WorkspaceID, result.ID, "invitation.accepted", "Invitation accepted", result.DisplayName+" · "+result.Email); err != nil {
		s.internalError(w, "audit invitation acceptance", err)
		return
	}
	var token string
	if !signedIn {
		var sessionHash [32]byte
		token, sessionHash, err = newSessionToken()
		if err != nil {
			s.internalError(w, "create invited session", err)
			return
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO sessions(account_id,token_hash,expires_at) VALUES ($1,$2,now()+interval '30 days')`, result.ID, sessionHash[:]); err != nil {
			s.internalError(w, "save invited session", err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit invitation acceptance", err)
		return
	}
	if !signedIn {
		setSessionCookie(w, r, token)
	}
	writeJSON(w, http.StatusCreated, result)
}
