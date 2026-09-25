package gateway

import (
	"context"
	"net/http"
	"time"
)

type syncRun struct {
	ID             string     `json:"id"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"startedAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	IssuesRead     int        `json:"issuesRead"`
	IssuesImported int        `json:"issuesImported"`
	Error          string     `json:"error"`
}

func (h *Handler) startSyncRun(ctx context.Context, installationID, trigger string) string {
	var id string
	_ = h.db.QueryRow(ctx, `INSERT INTO gateway_sync_runs(installation_id,trigger_type) VALUES($1,$2) RETURNING id::text`, installationID, trigger).Scan(&id)
	return id
}

func (h *Handler) finishSyncRun(ctx context.Context, id string, result jiraSyncResult, syncErr error) {
	if id == "" {
		return
	}
	if syncErr != nil {
		_, _ = h.db.Exec(ctx, `UPDATE gateway_sync_runs SET completed_at=now(),run_status='failed',error_message='Jira synchronization failed' WHERE id::text=$1`, id)
		return
	}
	_, _ = h.db.Exec(ctx, `UPDATE gateway_sync_runs SET completed_at=now(),run_status='succeeded',issues_read=$2,issues_imported=$3 WHERE id::text=$1`, id, result.Read, result.Imported)
}

func (h *Handler) listSyncRuns(w http.ResponseWriter, r *http.Request) {
	items, err := h.syncRuns(r.Context(), r.PathValue("installationID"), 50)
	if err != nil {
		h.internal(w, "list synchronization runs", err)
		return
	}
	writeGatewayJSON(w, http.StatusOK, items)
}

func (h *Handler) syncRuns(ctx context.Context, installationID string, limit int) ([]syncRun, error) {
	rows, err := h.db.Query(ctx, `SELECT id::text,trigger_type,run_status,started_at,completed_at,issues_read,issues_imported,error_message FROM gateway_sync_runs WHERE installation_id::text=$1 ORDER BY started_at DESC LIMIT $2`, installationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []syncRun{}
	for rows.Next() {
		var item syncRun
		if err = rows.Scan(&item.ID, &item.Trigger, &item.Status, &item.StartedAt, &item.CompletedAt, &item.IssuesRead, &item.IssuesImported, &item.Error); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
