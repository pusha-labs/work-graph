package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

var errRevisionPrecondition = errors.New("workspace revision precondition failed")

func serviceExpectedRevision(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	if _, service := servicePrincipalFromContext(r); !service {
		return nil, true
	}
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	expected, err := strconv.ParseInt(value, 10, 64)
	if err != nil || expected < 1 {
		writeError(w, http.StatusPreconditionRequired, `service writes require If-Match with the current workspace revision, for example If-Match: "42"`)
		return nil, false
	}
	return &expected, true
}

func nextRevisionExpected(ctx context.Context, tx pgx.Tx, workspaceID string, expected *int64) (int64, error) {
	if expected == nil {
		return nextRevision(ctx, tx, workspaceID)
	}
	var revision int64
	err := tx.QueryRow(ctx, `
		UPDATE workspaces SET revision=revision+1,updated_at=now()
		WHERE id=$1 AND revision=$2
		RETURNING revision`, workspaceID, *expected).Scan(&revision)
	if err == pgx.ErrNoRows {
		return 0, errRevisionPrecondition
	}
	return revision, err
}

func (s *server) writeRevisionError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if !errors.Is(err, errRevisionPrecondition) {
		s.writeDatabaseError(w, operation, err)
		return
	}
	var current int64
	if queryErr := s.db.QueryRow(r.Context(), `SELECT revision FROM workspaces WHERE id=$1`, r.PathValue("workspaceID")).Scan(&current); queryErr != nil {
		s.writeDatabaseError(w, operation, queryErr)
		return
	}
	w.Header().Set("ETag", `"`+strconv.FormatInt(current, 10)+`"`)
	writeJSON(w, http.StatusPreconditionFailed, map[string]any{
		"error":           "workspace changed since it was read",
		"currentRevision": current,
	})
}
