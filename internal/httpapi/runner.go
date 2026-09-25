package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

func runnerAuthorized(r *http.Request) bool {
	token := os.Getenv("RUNNER_TOKEN")
	return token != "" && r.Header.Get("Authorization") == "Bearer "+token
}

func (s *server) leaseHTTPExecution(w http.ResponseWriter, r *http.Request) {
	if !runnerAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "runner authentication required")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin execution lease", err)
		return
	}
	defer tx.Rollback(r.Context())
	var stepID, nodeID, workspaceID, name, moduleID, moduleVersion string
	var configuration json.RawMessage
	var before workNode
	err = tx.QueryRow(r.Context(), `SELECT s.id,n.id,n.workspace_id,s.name,s.module_id,s.module_version,s.configuration FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspace_module_installations i ON i.workspace_id=n.workspace_id AND i.module_id=s.module_id AND i.module_version=s.module_version AND i.enabled AND i.publisher_trusted WHERE s.step_status='ready' AND s.step_type='api' AND n.lifecycle_status='planned' ORDER BY n.created_at,s.position FOR UPDATE OF s,n SKIP LOCKED LIMIT 1`).Scan(&stepID, &nodeID, &workspaceID, &name, &moduleID, &moduleVersion, &configuration)
	if err == pgx.ErrNoRows {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.internalError(w, "lease HTTP execution", err)
		return
	}
	if err = scanNode(tx.QueryRow(r.Context(), nodeByIDSQL, workspaceID, nodeID), &before); err != nil {
		s.internalError(w, "read leased node", err)
		return
	}
	var resolvedConfiguration map[string]any
	if err := json.Unmarshal(configuration, &resolvedConfiguration); err != nil {
		s.internalError(w, "read HTTP configuration", err)
		return
	}
	if message := s.validateModulePolicy(r, workspaceID, moduleID, moduleVersion, resolvedConfiguration); message != "" {
		writeError(w, http.StatusForbidden, message)
		return
	}
	if secretID := strings.TrimSpace(stringValue(resolvedConfiguration["secretId"])); secretID != "" {
		var ciphertext, nonce []byte
		if err := tx.QueryRow(r.Context(), `SELECT ciphertext,nonce FROM workspace_secrets WHERE workspace_id=$1 AND id=$2 AND enabled`, workspaceID, secretID).Scan(&ciphertext, &nonce); err != nil {
			s.writeDatabaseError(w, "resolve HTTP secret", err)
			return
		}
		secretValue, err := decryptSecret(workspaceID, ciphertext, nonce)
		if err != nil {
			s.internalError(w, "decrypt HTTP secret", err)
			return
		}
		resolvedConfiguration["secretValue"] = secretValue
	}
	resolvedJSON, err := json.Marshal(resolvedConfiguration)
	if err != nil {
		s.internalError(w, "prepare HTTP configuration", err)
		return
	}
	var actorID string
	if err = tx.QueryRow(r.Context(), `INSERT INTO actors(workspace_id,display_name,actor_type) VALUES($1,'HTTP runner','automation') ON CONFLICT(workspace_id,display_name) DO UPDATE SET updated_at=now() RETURNING id`, workspaceID).Scan(&actorID); err != nil {
		s.internalError(w, "ensure runner actor", err)
		return
	}
	var executionID string
	err = tx.QueryRow(r.Context(), `INSERT INTO workflow_step_executions(workflow_step_id,attempt_number,execution_status,started_by,definition_snapshot) SELECT s.id,COALESCE((SELECT MAX(attempt_number) FROM workflow_step_executions WHERE workflow_step_id=s.id),0)+1,'running',$2,jsonb_build_object('name',s.name,'stepType',s.step_type,'moduleId',s.module_id,'moduleVersion',s.module_version,'configuration',s.configuration) FROM workflow_steps s WHERE s.id=$1 RETURNING id`, stepID, actorID).Scan(&executionID)
	if err != nil {
		s.internalError(w, "create HTTP execution", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='active',claimed_by=$2,started_at=now() WHERE id=$1`, stepID, actorID); err != nil {
		s.internalError(w, "activate HTTP step", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.internalError(w, "advance execution revision", err)
		return
	}
	var updated workNode
	if err = scanNode(tx.QueryRow(r.Context(), `UPDATE work_nodes SET lifecycle_status='active',updated_revision=$3,updated_at=now() WHERE workspace_id=$1 AND id=$2 RETURNING id,workspace_id,root_id,parent_id,title,desired_outcome,lifecycle_status,created_revision,updated_revision,created_at,updated_at`, workspaceID, nodeID, revision), &updated); err != nil {
		s.internalError(w, "activate execution node", err)
		return
	}
	if err = recordNodeChange(r.Context(), tx, updated, "node.updated", revision, before, actorID); err != nil {
		s.internalError(w, "record execution start", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit execution lease", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executionId": executionID, "stepId": stepID, "nodeId": nodeID, "name": name, "moduleId": moduleID, "moduleVersion": moduleVersion, "configuration": json.RawMessage(resolvedJSON)})
}

func (s *server) completeHTTPExecution(w http.ResponseWriter, r *http.Request) {
	if !runnerAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "runner authentication required")
		return
	}
	var input struct {
		ExecutionID string          `json:"executionId"`
		Status      string          `json:"status"`
		Result      json.RawMessage `json:"result"`
		Error       string          `json:"error"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Status != "succeeded" && input.Status != "failed" && input.Status != "timed_out" {
		writeError(w, http.StatusBadRequest, "unsupported execution result")
		return
	}
	if input.Result == nil {
		input.Result = json.RawMessage(`{}`)
	}
	input.Error = strings.TrimSpace(input.Error)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin execution completion", err)
		return
	}
	defer tx.Rollback(r.Context())
	var stepID, nodeID, workspaceID, actorID string
	var before workNode
	err = tx.QueryRow(r.Context(), `SELECT e.workflow_step_id,s.work_node_id,n.workspace_id,e.started_by FROM workflow_step_executions e JOIN workflow_steps s ON s.id=e.workflow_step_id JOIN work_nodes n ON n.id=s.work_node_id WHERE e.id=$1 AND e.execution_status='running' FOR UPDATE OF e,s,n`, input.ExecutionID).Scan(&stepID, &nodeID, &workspaceID, &actorID)
	if err != nil {
		s.writeDatabaseError(w, "find running execution", err)
		return
	}
	if err = scanNode(tx.QueryRow(r.Context(), nodeByIDSQL, workspaceID, nodeID), &before); err != nil {
		s.internalError(w, "read execution node", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_step_executions SET execution_status=$2,finished_at=now(),result=$3,error_message=NULLIF($4,'') WHERE id=$1`, input.ExecutionID, input.Status, input.Result, input.Error); err != nil {
		s.internalError(w, "finish HTTP execution", err)
		return
	}
	nextStatus := "blocked"
	if input.Status == "succeeded" {
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='completed',completed_at=now() WHERE id=$1`, stepID); err != nil {
			s.internalError(w, "complete HTTP step", err)
			return
		}
		var nextID string
		err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 AND step_status='pending' ORDER BY position LIMIT 1`, nodeID).Scan(&nextID)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='ready' WHERE id=$1`, nextID)
			nextStatus = "planned"
		} else if err == pgx.ErrNoRows {
			nextStatus = "review"
		}
		if err != nil && err != pgx.ErrNoRows {
			s.internalError(w, "activate next step", err)
			return
		}
	} else {
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='failed',completed_at=now() WHERE id=$1`, stepID); err != nil {
			s.internalError(w, "fail HTTP step", err)
			return
		}
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.internalError(w, "advance completion revision", err)
		return
	}
	var updated workNode
	if err = scanNode(tx.QueryRow(r.Context(), `UPDATE work_nodes SET lifecycle_status=$3,updated_revision=$4,updated_at=now() WHERE workspace_id=$1 AND id=$2 RETURNING id,workspace_id,root_id,parent_id,title,desired_outcome,lifecycle_status,created_revision,updated_revision,created_at,updated_at`, workspaceID, nodeID, nextStatus, revision), &updated); err != nil {
		s.internalError(w, "update completed execution node", err)
		return
	}
	if err = recordNodeChange(r.Context(), tx, updated, "node.updated", revision, before, actorID); err != nil {
		s.internalError(w, "record execution completion", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit execution completion", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": input.Status})
}

func (s *server) getHTTPExecutionStatus(w http.ResponseWriter, r *http.Request) {
	if !runnerAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "runner authentication required")
		return
	}
	var status string
	if err := s.db.QueryRow(r.Context(), `SELECT execution_status FROM workflow_step_executions WHERE id=$1`, r.PathValue("executionID")).Scan(&status); err != nil {
		s.writeDatabaseError(w, "find HTTP execution", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
