package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
)

type nativeEvent struct {
	ID                string          `json:"id"`
	EventType         string          `json:"eventType"`
	AggregateType     string          `json:"aggregateType"`
	AggregateID       string          `json:"aggregateId"`
	WorkspaceRevision *int64          `json:"workspaceRevision"`
	ActorName         *string         `json:"actorName"`
	CorrelationID     *string         `json:"correlationId"`
	Payload           json.RawMessage `json:"payload"`
	OccurredAt        time.Time       `json:"occurredAt"`
}

type nativeEventPage struct {
	Events     []nativeEvent `json:"events"`
	NextCursor string        `json:"nextCursor"`
	HasMore    bool          `json:"hasMore"`
}

const nativeEventSettleDelay = 2 * time.Second

type OutboundEvent struct {
	SourceEventID     string    `json:"sourceEventId"`
	Provider          string    `json:"provider"`
	ExternalID        string    `json:"externalId"`
	WorkGraphNodeID   string    `json:"workGraphNodeId"`
	EventType         string    `json:"eventType"`
	WorkspaceRevision *int64    `json:"workspaceRevision"`
	ActorName         *string   `json:"actorName"`
	ProjectionStatus  string    `json:"projectionStatus"`
	SuppressionReason string    `json:"suppressionReason"`
	OccurredAt        time.Time `json:"occurredAt"`
}

func (h *Handler) runEventConsumer(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		for {
			hasMore, err := h.consumeNativeEventPage(ctx)
			if err != nil {
				if ctx.Err() == nil {
					h.logger.Error("consume Work Graph events", "error", err)
				}
				break
			}
			if !hasMore {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) consumeNativeEventPage(ctx context.Context) (bool, error) {
	workspaceID, _, err := h.workspace(ctx)
	if err != nil {
		return false, err
	}
	consumerKey := "work-graph:" + workspaceID
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var claimed bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, consumerKey).Scan(&claimed); err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}
	var cursor string
	err = tx.QueryRow(ctx, `SELECT cursor::text FROM gateway_event_cursors WHERE consumer_key=$1`, consumerKey).Scan(&cursor)
	if err != nil && err != pgx.ErrNoRows {
		return false, err
	}
	path := "/api/v1/workspaces/" + url.PathEscape(workspaceID) + "/events?limit=100"
	if cursor != "" {
		path += "&after=" + url.QueryEscape(cursor)
	}
	var page nativeEventPage
	if err = h.coreJSON(ctx, http.MethodGet, path, nil, nil, &page); err != nil {
		return false, err
	}
	stable := page.Events[:0]
	cutoff := time.Now().UTC().Add(-nativeEventSettleDelay)
	for _, event := range page.Events {
		if event.OccurredAt.After(cutoff) {
			break
		}
		stable = append(stable, event)
	}
	if len(stable) == 0 {
		return false, nil
	}
	page.Events = stable
	page.NextCursor = stable[len(stable)-1].ID
	for _, event := range page.Events {
		if event.AggregateType != "work_node" {
			continue
		}
		_, err = tx.Exec(ctx, `INSERT INTO gateway_outbound_events(source_event_id,workspace_id,provider,installation_key,external_id,work_graph_node_id,event_type,workspace_revision,correlation_id,actor_name,payload,projection_status,suppression_reason,occurred_at)
			SELECT $1::uuid,$2::uuid,m.provider,m.installation_key,m.external_id,m.work_graph_node_id,$4::text,$5::bigint,$6::uuid,$7::text,$8::jsonb,
			CASE WHEN $5::bigint IS NOT NULL AND $5::bigint<=m.last_native_revision THEN 'suppressed' ELSE 'pending' END,
			CASE WHEN $5::bigint IS NOT NULL AND $5::bigint<=m.last_native_revision THEN 'gateway-originated revision' ELSE '' END,$9::timestamptz
			FROM gateway_mappings m WHERE m.workspace_id=$2::uuid AND m.work_graph_node_id=$3::uuid
			ON CONFLICT(source_event_id) DO NOTHING`, event.ID, workspaceID, event.AggregateID, event.EventType, event.WorkspaceRevision, event.CorrelationID, event.ActorName, event.Payload, event.OccurredAt)
		if err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO gateway_event_cursors(consumer_key,workspace_id,cursor) VALUES($1,$2,$3) ON CONFLICT(consumer_key) DO UPDATE SET cursor=EXCLUDED.cursor,updated_at=now()`, consumerKey, workspaceID, page.NextCursor)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return page.HasMore && len(stable) == 100, nil
}

func (h *Handler) listOutboundEvents(w http.ResponseWriter, r *http.Request) {
	items, err := h.outboundEvents(r.Context(), 100)
	if err != nil {
		h.internal(w, "list outbound events", err)
		return
	}
	writeGatewayJSON(w, http.StatusOK, items)
}

func (h *Handler) outboundEvents(ctx context.Context, limit int) ([]OutboundEvent, error) {
	rows, err := h.db.Query(ctx, `SELECT source_event_id,provider,external_id,work_graph_node_id,event_type,workspace_revision,actor_name,projection_status,suppression_reason,occurred_at FROM gateway_outbound_events ORDER BY occurred_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (OutboundEvent, error) {
		var item OutboundEvent
		err := row.Scan(&item.SourceEventID, &item.Provider, &item.ExternalID, &item.WorkGraphNodeID, &item.EventType, &item.WorkspaceRevision, &item.ActorName, &item.ProjectionStatus, &item.SuppressionReason, &item.OccurredAt)
		return item, err
	})
	return items, err
}
