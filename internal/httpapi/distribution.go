package httpapi

import (
	"encoding/json"
	"net/http"
)

func (s *server) updateWorkspaceDistribution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Mode string `json:"mode"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Mode != "simple" && input.Mode != "exchange" {
		writeError(w, http.StatusBadRequest, "distribution mode must be simple or exchange")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find distribution policy author", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin distribution policy update", err)
		return
	}
	defer tx.Rollback(r.Context())
	var before string
	if err = tx.QueryRow(r.Context(), `SELECT work_distribution_mode FROM workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&before); err != nil {
		s.writeDatabaseError(w, "find workspace distribution policy", err)
		return
	}
	var result workspace
	err = tx.QueryRow(r.Context(), `UPDATE workspaces SET work_distribution_mode=$2,revision=revision+1,updated_at=now() WHERE id=$1 RETURNING id,name,revision,work_distribution_mode,created_at`, workspaceID, input.Mode).Scan(&result.ID, &result.Name, &result.Revision, &result.WorkDistributionMode, &result.CreatedAt)
	if err != nil {
		s.internalError(w, "update workspace distribution policy", err)
		return
	}
	beforeJSON, _ := json.Marshal(map[string]string{"mode": before})
	afterJSON, _ := json.Marshal(map[string]string{"mode": input.Mode})
	if _, err = tx.Exec(r.Context(), `INSERT INTO change_events(workspace_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,before_state,after_state,actor_id) VALUES($1,$2,uuidv7(),'workspace',$1,'workspace.distribution_changed',$3,$4,$5)`, workspaceID, result.Revision, json.RawMessage(beforeJSON), json.RawMessage(afterJSON), actorID); err != nil {
		s.internalError(w, "record workspace distribution policy", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workspace distribution policy", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
