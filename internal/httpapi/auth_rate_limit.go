package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) consumeAuthAttempt(ctx context.Context, scope, identifier string, limit int, window, blockFor time.Duration) (time.Duration, error) {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(identifier))))
	var blockedUntil *time.Time
	err := s.db.QueryRow(ctx, `
		INSERT INTO auth_rate_limits(scope,identifier_hash,window_started_at,attempt_count,blocked_until)
		VALUES($1,$2,now(),1,NULL)
		ON CONFLICT(scope,identifier_hash) DO UPDATE SET
		  window_started_at=CASE WHEN auth_rate_limits.window_started_at <= now()-$3::interval THEN now() ELSE auth_rate_limits.window_started_at END,
		  attempt_count=CASE WHEN auth_rate_limits.window_started_at <= now()-$3::interval THEN 1 ELSE auth_rate_limits.attempt_count+1 END,
		  blocked_until=CASE
		    WHEN auth_rate_limits.blocked_until>now() THEN auth_rate_limits.blocked_until
		    WHEN (CASE WHEN auth_rate_limits.window_started_at <= now()-$3::interval THEN 1 ELSE auth_rate_limits.attempt_count+1 END)>$4 THEN now()+$5::interval
		    ELSE NULL END,
		  updated_at=now()
		RETURNING blocked_until`, scope, hash[:], durationInterval(window), limit, durationInterval(blockFor)).Scan(&blockedUntil)
	if err != nil || blockedUntil == nil {
		return 0, err
	}
	retryAfter := time.Until(*blockedUntil)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return retryAfter, nil
}

func (s *server) clearAuthAttempts(ctx context.Context, scope, identifier string) {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(identifier))))
	_, _ = s.db.Exec(ctx, `DELETE FROM auth_rate_limits WHERE scope=$1 AND identifier_hash=$2`, scope, hash[:])
}

func enforceAuthLimit(w http.ResponseWriter, retryAfter time.Duration) bool {
	if retryAfter <= 0 {
		return true
	}
	seconds := int(retryAfter.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, http.StatusTooManyRequests, "too many attempts; please try again later")
	return false
}

func clientIdentifier(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if first, _, ok := strings.Cut(forwarded, ","); ok {
			return strings.TrimSpace(first)
		}
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func durationInterval(value time.Duration) string {
	return strconv.FormatInt(int64(value/time.Second), 10) + " seconds"
}

func opaqueIdentifier(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
