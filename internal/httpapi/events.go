package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type committedEvent struct {
	ID                string          `json:"id" db:"id"`
	EventType         string          `json:"eventType" db:"event_type"`
	AggregateType     string          `json:"aggregateType" db:"aggregate_type"`
	AggregateID       string          `json:"aggregateId" db:"aggregate_id"`
	WorkspaceRevision *int64          `json:"workspaceRevision" db:"workspace_revision"`
	ActorID           *string         `json:"actorId" db:"actor_id"`
	ActorName         *string         `json:"actorName" db:"actor_name"`
	CorrelationID     *string         `json:"correlationId" db:"correlation_id"`
	Payload           json.RawMessage `json:"payload" db:"payload"`
	OccurredAt        time.Time       `json:"occurredAt" db:"occurred_at"`
}

func (s *server) listCommittedEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	cursor := strings.TrimSpace(r.URL.Query().Get("after"))
	limit, valid := eventPageLimit(r.URL.Query().Get("limit"))
	if !valid {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
		return
	}
	query := `
		SELECT o.id,o.event_type,o.aggregate_type,o.aggregate_id,o.workspace_revision,
		       o.actor_id,a.display_name AS actor_name,o.correlation_id,o.payload,o.occurred_at
		FROM outbox_events o LEFT JOIN actors a ON a.id=o.actor_id
		WHERE o.workspace_id=$1`
	arguments := []any{workspaceID}
	if cursor != "" {
		var cursorTime time.Time
		var cursorID string
		if err := s.db.QueryRow(r.Context(), `SELECT occurred_at,id FROM outbox_events WHERE workspace_id=$1 AND id::text=$2`, workspaceID, cursor).Scan(&cursorTime, &cursorID); err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusBadRequest, "event cursor is not valid for this workspace")
			} else {
				s.internalError(w, "resolve event cursor", err)
			}
			return
		}
		query += ` AND (o.occurred_at,o.id) > ($2,$3::uuid)`
		arguments = append(arguments, cursorTime, cursorID)
	}
	query += ` ORDER BY o.occurred_at,o.id LIMIT $` + strconv.Itoa(len(arguments)+1)
	arguments = append(arguments, limit+1)
	rows, err := s.db.Query(r.Context(), query, arguments...)
	if err != nil {
		s.internalError(w, "list committed events", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[committedEvent])
	if err != nil {
		s.internalError(w, "read committed events", err)
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	nextCursor := cursor
	if len(items) > 0 {
		nextCursor = items[len(items)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items, "nextCursor": nextCursor, "hasMore": hasMore})
}

func eventPageLimit(value string) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return 100, true
	}
	limit, err := strconv.Atoi(value)
	return limit, err == nil && limit >= 1 && limit <= 500
}
