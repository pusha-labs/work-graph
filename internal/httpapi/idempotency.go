package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

const idempotencyKeyHeader = "Idempotency-Key"

func serviceIdempotencyKey(w http.ResponseWriter, r *http.Request) (servicePrincipal, string, bool) {
	principal, service := servicePrincipalFromContext(r)
	if !service {
		return servicePrincipal{}, "", true
	}
	key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if key == "" || len(key) > 200 || strings.ContainsAny(key, "\r\n") {
		writeError(w, http.StatusBadRequest, "service writes require an Idempotency-Key header of 1 to 200 characters")
		return servicePrincipal{}, "", false
	}
	return principal, key, true
}

func requestFingerprint(value any) ([32]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func findServiceCommandResult(r *http.Request, tx pgx.Tx, principal servicePrincipal, key string, fingerprint [32]byte) (*workNode, int, bool, error) {
	if key == "" {
		return nil, 0, false, nil
	}
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, principal.ID+":"+key); err != nil {
		return nil, 0, false, err
	}
	var storedHash, body []byte
	var status int
	err := tx.QueryRow(r.Context(), `
		SELECT request_hash,response_status,response_body
		FROM service_command_results
		WHERE service_account_id=$1 AND idempotency_key=$2`, principal.ID, key).Scan(&storedHash, &status, &body)
	if err == pgx.ErrNoRows {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !equalHash(storedHash, fingerprint[:]) {
		return nil, http.StatusConflict, true, nil
	}
	var node workNode
	if err := json.Unmarshal(body, &node); err != nil {
		return nil, 0, false, err
	}
	return &node, status, true, nil
}

func saveServiceCommandResult(r *http.Request, tx pgx.Tx, principal servicePrincipal, key string, fingerprint [32]byte, status int, node workNode) error {
	if key == "" {
		return nil
	}
	body, err := json.Marshal(node)
	if err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO service_command_results(service_account_id,workspace_id,idempotency_key,request_hash,response_status,response_body)
		VALUES ($1,$2,$3,$4,$5,$6)`, principal.ID, principal.WorkspaceID, key, fingerprint[:], status, body)
	return err
}

func equalHash(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}
