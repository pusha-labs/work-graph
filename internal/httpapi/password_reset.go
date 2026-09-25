package httpapi

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "password_reset_ip", clientIdentifier(r), 10, time.Hour, time.Hour); err != nil {
		s.internalError(w, "limit password reset", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "password_reset_email", input.Email, 3, time.Hour, time.Hour); err != nil {
		s.internalError(w, "limit password reset", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}

	var accountID string
	err := s.db.QueryRow(r.Context(), `SELECT id FROM accounts WHERE lower(email)=lower($1)`, input.Email).Scan(&accountID)
	if err == nil && passwordEmailConfigured() {
		token, tokenHash, tokenErr := newSessionToken()
		if tokenErr != nil {
			s.internalError(w, "create password reset token", tokenErr)
			return
		}
		if _, tokenErr = s.db.Exec(r.Context(), `
			WITH expired AS (UPDATE password_reset_tokens SET used_at=now() WHERE account_id=$1 AND used_at IS NULL)
			INSERT INTO password_reset_tokens(account_id,token_hash,expires_at) VALUES($1,$2,now()+interval '30 minutes')`, accountID, tokenHash[:]); tokenErr != nil {
			s.internalError(w, "save password reset token", tokenErr)
			return
		}
		if tokenErr = sendPasswordResetEmail(input.Email, token); tokenErr != nil {
			s.logger.Error("send password reset email", "error", tokenErr)
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.internalError(w, "find password reset account", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "If an account exists, a reset link will be sent."})
}

func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Password) < 10 {
		writeError(w, http.StatusBadRequest, "password must be at least 10 characters")
		return
	}
	identifier := opaqueIdentifier(r.PathValue("token") + ":" + clientIdentifier(r))
	if retryAfter, err := s.consumeAuthAttempt(r.Context(), "password_reset_token", identifier, 8, 15*time.Minute, 15*time.Minute); err != nil {
		s.internalError(w, "limit password reset", err)
		return
	} else if !enforceAuthLimit(w, retryAfter) {
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.internalError(w, "hash replacement password", err)
		return
	}
	tokenHash := sha256.Sum256([]byte(r.PathValue("token")))
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin password reset", err)
		return
	}
	defer tx.Rollback(r.Context())
	var tokenID, accountID string
	err = tx.QueryRow(r.Context(), `SELECT id,account_id FROM password_reset_tokens WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() FOR UPDATE`, tokenHash[:]).Scan(&tokenID, &accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "password reset link is invalid or expired")
		return
	}
	if err != nil {
		s.internalError(w, "find password reset token", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE accounts SET password_hash=$2,updated_at=now() WHERE id=$1`, accountID, string(passwordHash)); err != nil {
		s.internalError(w, "replace password", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE password_reset_tokens SET used_at=now() WHERE account_id=$1 AND used_at IS NULL`, accountID); err != nil {
		s.internalError(w, "consume password reset tokens", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE account_id=$1`, accountID); err != nil {
		s.internalError(w, "end existing sessions", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit password reset", err)
		return
	}
	s.clearAuthAttempts(r.Context(), "password_reset_token", identifier)
	w.WriteHeader(http.StatusNoContent)
}

func passwordEmailConfigured() bool {
	return strings.TrimSpace(os.Getenv("WORK_GRAPH_SMTP_ADDRESS")) != "" && strings.TrimSpace(os.Getenv("WORK_GRAPH_EMAIL_FROM")) != "" && strings.TrimSpace(os.Getenv("WORK_GRAPH_PUBLIC_URL")) != ""
}

func sendPasswordResetEmail(recipient, token string) error {
	address := strings.TrimSpace(os.Getenv("WORK_GRAPH_SMTP_ADDRESS"))
	from := strings.TrimSpace(os.Getenv("WORK_GRAPH_EMAIL_FROM"))
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("WORK_GRAPH_PUBLIC_URL")), "/")
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("WORK_GRAPH_PUBLIC_URL is invalid")
	}
	resetURL := publicURL + "/?reset=" + url.QueryEscape(token)
	host := address
	if index := strings.LastIndex(address, ":"); index > 0 {
		host = address[:index]
	}
	var auth smtp.Auth
	username := os.Getenv("WORK_GRAPH_SMTP_USERNAME")
	if username != "" {
		auth = smtp.PlainAuth("", username, os.Getenv("WORK_GRAPH_SMTP_PASSWORD"), host)
	}
	message := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Reset your Work Graph password\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nUse this link within 30 minutes to choose a new password:\r\n%s\r\n\r\nIf you did not request this, you can ignore this message.\r\n", from, recipient, resetURL))
	return smtp.SendMail(address, auth, from, []string{recipient}, message)
}
