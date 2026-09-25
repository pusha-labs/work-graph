package gateway

import (
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) updateJiraSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeGatewayError(w, 400, "invalid form")
		return
	}
	id, jql := r.PathValue("installationID"), strings.TrimSpace(r.FormValue("jql"))
	minutes, err1 := strconv.Atoi(r.FormValue("intervalMinutes"))
	batch, err2 := strconv.Atoi(r.FormValue("maxIssues"))
	if err1 != nil || err2 != nil || minutes < 1 || minutes > 1440 || batch < 1 || batch > 100 {
		writeGatewayError(w, 400, "interval must be 1-1440 minutes and batch size 1-100")
		return
	}
	var cloudID, status string
	if err := h.db.QueryRow(r.Context(), `SELECT configuration->>'cloudId',installation_status FROM gateway_installations WHERE id::text=$1 AND provider='jira_cloud'`, id).Scan(&cloudID, &status); err != nil || status != "connected" {
		writeGatewayError(w, 409, "connected Jira installation required")
		return
	}
	token, err := h.jiraAccessToken(r.Context(), id)
	if err != nil {
		writeGatewayError(w, 502, "Jira settings could not be validated")
		return
	}
	probe := jql
	if probe == "" {
		probe = "ORDER BY updated DESC"
	}
	if _, _, _, err = h.searchJiraIssues(r.Context(), cloudID, token, probe, "", 1); err != nil {
		writeGatewayError(w, 400, "JQL was rejected by Jira")
		return
	}
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		h.internal(w, "update Jira settings", err)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `UPDATE gateway_installations SET configuration=jsonb_set(jsonb_set(configuration,'{jql}',to_jsonb($2::text),true),'{maxIssues}',to_jsonb($3::int),true),updated_at=now() WHERE id::text=$1`, id, jql, batch)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE gateway_sync_schedules SET interval_seconds=$2*60,cursor_at=NULL,continuation_token='',cycle_query='',cycle_upper_bound=NULL,next_run_at=now(),updated_at=now() WHERE installation_id::text=$1`, id, minutes)
	}
	if err != nil {
		h.internal(w, "update Jira settings", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		h.internal(w, "update Jira settings", err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) pauseJiraSync(w http.ResponseWriter, r *http.Request) {
	h.setSyncEnabled(w, r, false)
}
func (h *Handler) resumeJiraSync(w http.ResponseWriter, r *http.Request) {
	h.setSyncEnabled(w, r, true)
}
func (h *Handler) setSyncEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	result, err := h.db.Exec(r.Context(), `UPDATE gateway_sync_schedules SET enabled=$2,next_run_at=CASE WHEN $2 THEN now() ELSE next_run_at END,lease_until=NULL,updated_at=now() WHERE installation_id::text=$1`, r.PathValue("installationID"), enabled)
	if err != nil {
		h.internal(w, "change Jira schedule", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeGatewayError(w, 404, "synchronization schedule not found")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) resetJiraCursor(w http.ResponseWriter, r *http.Request) {
	result, err := h.db.Exec(r.Context(), `UPDATE gateway_sync_schedules SET cursor_at=NULL,continuation_token='',cycle_query='',cycle_upper_bound=NULL,next_run_at=now(),lease_until=NULL,updated_at=now() WHERE installation_id::text=$1`, r.PathValue("installationID"))
	if err != nil {
		h.internal(w, "reset Jira cursor", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeGatewayError(w, 404, "synchronization schedule not found")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
