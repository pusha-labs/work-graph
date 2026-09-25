package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "work_graph_session"

type account struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type accountContextKey struct{}

func accountFromContext(ctx context.Context) (account, bool) {
	value, ok := ctx.Value(accountContextKey{}).(account)
	return value, ok
}

func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	var hasAccounts bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM accounts)`).Scan(&hasAccounts); err != nil {
		s.internalError(w, "check authentication setup", err)
		return
	}
	current, ok := accountFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"setupRequired": !hasAccounts, "registrationEnabled": registrationEnabled(), "passwordResetEnabled": passwordEmailConfigured(), "authenticated": ok, "account": func() any {
		if ok {
			return current
		}
		return nil
	}()})
}

func (s *server) registerAccount(w http.ResponseWriter, r *http.Request) {
	var input struct{ DisplayName, Email, Password string }
	if !decodeJSON(w, r, &input) {
		return
	}
	input.DisplayName, input.Email = strings.TrimSpace(input.DisplayName), strings.ToLower(strings.TrimSpace(input.Email))
	if input.DisplayName == "" || !strings.Contains(input.Email, "@") || len(input.Password) < 10 {
		writeError(w, http.StatusBadRequest, "name, valid email, and a password of at least 10 characters are required")
		return
	}
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "register_ip", clientIdentifier(r), 10, time.Hour, time.Hour); err != nil {
		s.internalError(w, "limit account registration", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "register_email", input.Email, 5, time.Hour, time.Hour); err != nil {
		s.internalError(w, "limit account registration", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.internalError(w, "hash password", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin account registration", err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(9137301)`); err != nil {
		s.internalError(w, "lock account setup", err)
		return
	}
	var count int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM accounts`).Scan(&count); err != nil {
		s.internalError(w, "check account setup", err)
		return
	}
	if count > 0 && !registrationEnabled() {
		writeError(w, http.StatusForbidden, "account registration is disabled; ask a workspace administrator for an invitation")
		return
	}
	var result account
	if err := tx.QueryRow(r.Context(), `INSERT INTO accounts(email, display_name, password_hash) VALUES ($1,$2,$3) RETURNING id,email,display_name`, input.Email, input.DisplayName, string(hash)).Scan(&result.ID, &result.Email, &result.DisplayName); err != nil {
		if strings.Contains(err.Error(), "accounts_email_idx") {
			writeError(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		s.internalError(w, "create account", err)
		return
	}
	// The first account claims workspaces that existed before authentication was
	// configured. Later accounts intentionally start without workspace access;
	// they can create their own workspace or join another one by invitation.
	if count == 0 {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_actors(account_id, actor_id, workspace_role)
			SELECT $1, id, 'owner' FROM actors WHERE has_account
			ON CONFLICT (actor_id) DO NOTHING`, result.ID); err != nil {
			s.internalError(w, "link existing owner actors", err)
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE actors SET display_name=$2 WHERE id IN (SELECT actor_id FROM account_actors WHERE account_id=$1)`, result.ID, result.DisplayName); err != nil {
			s.internalError(w, "update owner profile", err)
			return
		}
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		s.internalError(w, "create session token", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO sessions(account_id,token_hash,expires_at) VALUES ($1,$2,$3)`, result.ID, tokenHash[:], time.Now().Add(30*24*time.Hour)); err != nil {
		s.internalError(w, "save owner session", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit account registration", err)
		return
	}
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusCreated, result)
}

func registrationEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("WORK_GRAPH_REGISTRATION_MODE")), "open")
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var input struct{ Email, Password string }
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	clientID := clientIdentifier(r)
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "login_ip", clientID, 30, 15*time.Minute, 15*time.Minute); err != nil {
		s.internalError(w, "limit login", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "login_email", input.Email, 8, 15*time.Minute, 15*time.Minute); err != nil {
		s.internalError(w, "limit login", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	var result account
	var passwordHash string
	err := s.db.QueryRow(r.Context(), `SELECT id,email,display_name,password_hash FROM accounts WHERE lower(email)=lower($1)`, strings.TrimSpace(input.Email)).Scan(&result.ID, &result.Email, &result.DisplayName, &passwordHash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	s.clearAuthAttempts(r.Context(), "login_ip", clientID)
	s.clearAuthAttempts(r.Context(), "login_email", input.Email)
	token, tokenHash, err := newSessionToken()
	if err != nil {
		s.internalError(w, "create session token", err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO sessions(account_id,token_hash,expires_at) VALUES ($1,$2,$3)`, result.ID, tokenHash[:], time.Now().Add(30*24*time.Hour)); err != nil {
		s.internalError(w, "save login session", err)
		return
	}
	setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, result)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		hash := sha256.Sum256([]byte(cookie.Value))
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, hash[:])
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) withAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(authorization, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			if !strings.HasPrefix(token, "wgsa_") {
				writeError(w, http.StatusUnauthorized, "invalid service token")
				return
			}
			hash := sha256.Sum256([]byte(token))
			var principal servicePrincipal
			err := s.db.QueryRow(r.Context(), `
				UPDATE service_accounts sa SET last_used_at=now()
				FROM actors a
				WHERE sa.actor_id=a.id AND sa.token_hash=$1 AND sa.revoked_at IS NULL
				RETURNING sa.id,sa.workspace_id,sa.actor_id,a.display_name,sa.scopes`, hash[:]).Scan(
				&principal.ID, &principal.WorkspaceID, &principal.ActorID, &principal.DisplayName, &principal.Scopes,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid service token")
				return
			}
			if err != nil {
				s.internalError(w, "validate service token", err)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), servicePrincipalContextKey{}, principal)))
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		hash := sha256.Sum256([]byte(cookie.Value))
		var current account
		err = s.db.QueryRow(r.Context(), `
			SELECT a.id,a.email,a.display_name FROM sessions s JOIN accounts a ON a.id=s.account_id
			WHERE s.token_hash=$1 AND s.expires_at>now()`, hash[:]).Scan(&current.ID, &current.Email, &current.DisplayName)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if err != nil {
			s.internalError(w, "validate session", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountContextKey{}, current)))
	})
}

func (s *server) withOptionalAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		hash := sha256.Sum256([]byte(cookie.Value))
		var current account
		err = s.db.QueryRow(r.Context(), `SELECT a.id,a.email,a.display_name FROM sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>now()`, hash[:]).Scan(&current.ID, &current.Email, &current.DisplayName)
		if err == nil {
			r = r.WithContext(context.WithValue(r.Context(), accountContextKey{}, current))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) authorizeWorkspace(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, service := servicePrincipalFromContext(r); service {
			writeError(w, http.StatusForbidden, "service token is not permitted for this endpoint")
			return
		}
		current, _ := accountFromContext(r.Context())
		workspaceID := r.PathValue("workspaceID")
		var allowed bool
		err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM account_actors aa JOIN actors a ON a.id=aa.actor_id WHERE aa.account_id=$1 AND a.workspace_id=$2)`, current.ID, workspaceID).Scan(&allowed)
		if err != nil {
			s.internalError(w, "authorize workspace", err)
			return
		}
		if !allowed {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		next(w, r)
	}
}

func (s *server) authorizeWorkspaceServiceScope(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if principal, service := servicePrincipalFromContext(r); service {
			if principal.WorkspaceID != r.PathValue("workspaceID") {
				writeError(w, http.StatusNotFound, "resource not found")
				return
			}
			if !principal.hasScope(scope) {
				writeError(w, http.StatusForbidden, "service token lacks required scope: "+scope)
				return
			}
			next(w, r)
			return
		}
		s.authorizeWorkspace(next)(w, r)
	}
}

func (s *server) authorizeWorkspaceAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.authorizeWorkspace(func(w http.ResponseWriter, r *http.Request) {
		current, _ := accountFromContext(r.Context())
		var role string
		err := s.db.QueryRow(r.Context(), `SELECT aa.workspace_role FROM account_actors aa JOIN actors a ON a.id=aa.actor_id WHERE aa.account_id=$1 AND a.workspace_id=$2`, current.ID, r.PathValue("workspaceID")).Scan(&role)
		if err != nil {
			s.internalError(w, "read workspace role", err)
			return
		}
		if role != "owner" && role != "admin" {
			writeError(w, http.StatusForbidden, "administrator access required")
			return
		}
		next(w, r)
	})
}

func newSessionToken() (string, [32]byte, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, sha256.Sum256([]byte(token)), nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 60 * 60})
}

func (s *server) currentActorID(ctx context.Context, workspaceID string) (string, error) {
	if principal, ok := ctx.Value(servicePrincipalContextKey{}).(servicePrincipal); ok {
		if principal.WorkspaceID != workspaceID {
			return "", pgx.ErrNoRows
		}
		return principal.ActorID, nil
	}
	current, ok := accountFromContext(ctx)
	if !ok {
		return "", pgx.ErrNoRows
	}
	var actorID string
	err := s.db.QueryRow(ctx, `SELECT aa.actor_id FROM account_actors aa JOIN actors a ON a.id=aa.actor_id WHERE aa.account_id=$1 AND a.workspace_id=$2`, current.ID, workspaceID).Scan(&actorID)
	return actorID, err
}
