package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
)

// updateWorkflowStep changes an unstarted definition while keeping its stable identity and position.
func (s *server) updateWorkflowStep(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name             string          `json:"name"`
		CapabilityID     string          `json:"capabilityId"`
		SubjectID        string          `json:"subjectId"`
		Configuration    json.RawMessage `json:"configuration"`
		DistributionMode string          `json:"distributionMode"`
		ModuleVersion    string          `json:"moduleVersion"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "stage name is required")
		return
	}
	if input.Configuration == nil {
		input.Configuration = json.RawMessage(`{}`)
	}
	if input.DistributionMode != "" && input.DistributionMode != "inherit" && input.DistributionMode != "simple" && input.DistributionMode != "exchange" {
		writeError(w, http.StatusBadRequest, "distribution mode must be inherit, simple, or exchange")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workflow step update", err)
		return
	}
	defer tx.Rollback(r.Context())
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	var rootID, lifecycle string
	if err = tx.QueryRow(r.Context(), `SELECT root_id,lifecycle_status FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision IS NULL FOR UPDATE`, workspaceID, nodeID).Scan(&rootID, &lifecycle); err != nil {
		s.writeDatabaseError(w, "find work node", err)
		return
	}
	if lifecycle != "planned" {
		writeError(w, http.StatusConflict, "workflow can only be edited before work starts")
		return
	}
	var stepType, previousName, previousCapability, previousKnowledge, previousDistribution string
	var moduleID, moduleVersion *string
	var previousConfiguration json.RawMessage
	if err = tx.QueryRow(r.Context(), `SELECT s.step_type,s.module_id,s.module_version,s.name,s.distribution_mode,s.configuration,
		COALESCE((SELECT c.name FROM workflow_step_capabilities r JOIN capabilities c ON c.id=r.capability_id WHERE r.workflow_step_id=s.id ORDER BY c.name LIMIT 1),''),
		COALESCE((SELECT k.name FROM workflow_step_knowledge r JOIN knowledge_subjects k ON k.id=r.subject_id WHERE r.workflow_step_id=s.id ORDER BY k.name LIMIT 1),'')
		FROM workflow_steps s WHERE s.id=$1 AND s.work_node_id=$2 FOR UPDATE`, stepID, nodeID).Scan(&stepType, &moduleID, &moduleVersion, &previousName, &previousDistribution, &previousConfiguration, &previousCapability, &previousKnowledge); err != nil {
		s.writeDatabaseError(w, "find workflow step", err)
		return
	}
	if stepType != "human" {
		targetVersion := input.ModuleVersion
		if targetVersion == "" && moduleVersion != nil {
			targetVersion = *moduleVersion
		}
		var schema json.RawMessage
		if err = tx.QueryRow(r.Context(), `SELECT configuration_schema FROM workflow_modules WHERE module_id=$1 AND module_version=$2 AND enabled`, moduleID, targetVersion).Scan(&schema); err != nil {
			s.writeDatabaseError(w, "find workflow module", err)
			return
		}
		var configuration map[string]any
		if json.Unmarshal(input.Configuration, &configuration) != nil {
			writeError(w, http.StatusBadRequest, "configuration must be a JSON object")
			return
		}
		if message := validateModuleConfiguration(schema, configuration); message != "" {
			writeError(w, http.StatusBadRequest, message)
			return
		}
		if moduleID != nil {
			if message := s.validateModulePolicy(r, workspaceID, *moduleID, targetVersion, configuration); message != "" {
				writeError(w, http.StatusForbidden, message)
				return
			}
		}
		if moduleID != nil && *moduleID == "builtin.http" {
			if message := s.validateSecretReference(r, workspaceID, configuration); message != "" {
				writeError(w, http.StatusBadRequest, message)
				return
			}
		}
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	targetModuleVersion := any(nil)
	if stepType != "human" {
		targetModuleVersion = input.ModuleVersion
		if input.ModuleVersion == "" && moduleVersion != nil {
			targetModuleVersion = *moduleVersion
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET name=$3,configuration=$4,distribution_mode=CASE WHEN $5='' THEN distribution_mode ELSE $5 END,module_version=COALESCE($6,module_version) WHERE id=$1 AND work_node_id=$2`, stepID, nodeID, input.Name, input.Configuration, input.DistributionMode, targetModuleVersion); err != nil {
		s.internalError(w, "update workflow step", err)
		return
	}
	if stepType == "human" {
		if _, err = tx.Exec(r.Context(), `DELETE FROM workflow_step_capabilities WHERE workflow_step_id=$1`, stepID); err != nil {
			s.internalError(w, "reset workflow requirements", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `DELETE FROM workflow_step_knowledge WHERE workflow_step_id=$1`, stepID); err != nil {
			s.internalError(w, "reset workflow knowledge", err)
			return
		}
		if input.CapabilityID != "" {
			if _, err = tx.Exec(r.Context(), `INSERT INTO workflow_step_capabilities SELECT $1,c.id FROM capabilities c WHERE c.workspace_id=$2 AND c.id=$3`, stepID, workspaceID, input.CapabilityID); err != nil {
				s.internalError(w, "update step capability", err)
				return
			}
		}
		if input.SubjectID != "" {
			if _, err = tx.Exec(r.Context(), `INSERT INTO workflow_step_knowledge SELECT $1,k.id FROM knowledge_subjects k WHERE k.workspace_id=$2 AND k.id=$3`, stepID, workspaceID, input.SubjectID); err != nil {
				s.internalError(w, "update step knowledge", err)
				return
			}
		}
	}
	capabilityName, knowledgeName := "", ""
	if input.CapabilityID != "" {
		_ = tx.QueryRow(r.Context(), `SELECT name FROM capabilities WHERE workspace_id=$1 AND id=$2`, workspaceID, input.CapabilityID).Scan(&capabilityName)
	}
	if input.SubjectID != "" {
		_ = tx.QueryRow(r.Context(), `SELECT name FROM knowledge_subjects WHERE workspace_id=$1 AND id=$2`, workspaceID, input.SubjectID).Scan(&knowledgeName)
	}
	distribution := input.DistributionMode
	if distribution == "" {
		distribution = previousDistribution
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workflow author", err)
		return
	}
	var previousConfigurationValue, configurationValue any
	_ = json.Unmarshal(previousConfiguration, &previousConfigurationValue)
	_ = json.Unmarshal(input.Configuration, &configurationValue)
	newModuleVersion := ""
	if targetModuleVersion != nil {
		newModuleVersion = targetModuleVersion.(string)
	}
	previousModuleVersion := ""
	if moduleVersion != nil {
		previousModuleVersion = *moduleVersion
	}
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "stepId": stepID, "name": input.Name, "previousName": previousName, "capability": capabilityName, "previousCapability": previousCapability, "knowledge": knowledgeName, "previousKnowledge": previousKnowledge, "distributionMode": distribution, "previousDistributionMode": previousDistribution, "configurationChanged": !reflect.DeepEqual(previousConfigurationValue, configurationValue), "moduleVersion": newModuleVersion, "previousModuleVersion": previousModuleVersion})
	if _, err = tx.Exec(r.Context(), `WITH event AS (INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step',$4,'workflow_step.updated',$5,$6) RETURNING correlation_id) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id) SELECT $1,'workflow_step.updated','work_node',$7,$5,$3,$6,correlation_id FROM event`, workspaceID, rootID, revision, stepID, json.RawMessage(after), actorID, nodeID); err != nil {
		s.internalError(w, "record workflow update", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow update", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *server) moveWorkflowStep(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Direction string `json:"direction"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Direction != "earlier" && input.Direction != "later" {
		writeError(w, http.StatusBadRequest, "direction must be earlier or later")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workflow move", err)
		return
	}
	defer tx.Rollback(r.Context())
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	var rootID, lifecycle string
	if err = tx.QueryRow(r.Context(), `SELECT root_id,lifecycle_status FROM work_nodes WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspaceID, nodeID).Scan(&rootID, &lifecycle); err != nil {
		s.writeDatabaseError(w, "find work node", err)
		return
	}
	if lifecycle != "planned" {
		writeError(w, http.StatusConflict, "workflow can only be reordered before work starts")
		return
	}
	var position int
	var stepName string
	if err = tx.QueryRow(r.Context(), `SELECT position,name FROM workflow_steps WHERE id=$1 AND work_node_id=$2`, stepID, nodeID).Scan(&position, &stepName); err != nil {
		s.writeDatabaseError(w, "find workflow step", err)
		return
	}
	delta := -1
	if input.Direction == "later" {
		delta = 1
	}
	var otherID string
	if err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 AND position=$2`, nodeID, position+delta).Scan(&otherID); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]any{"status": "unchanged"})
			return
		}
		s.writeDatabaseError(w, "find adjacent step", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET position=CASE WHEN id=$1 THEN $3 ELSE $4 END WHERE id IN ($1,$2)`, stepID, otherID, position+delta, position); err != nil {
		s.internalError(w, "move workflow step", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	actorID, _ := s.currentActorID(r.Context(), workspaceID)
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "stepId": stepID, "name": stepName, "direction": input.Direction, "fromPosition": position, "toPosition": position + delta})
	if _, err = tx.Exec(r.Context(), `INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step',$4,'workflow_step.moved',$5,$6)`, workspaceID, rootID, revision, stepID, json.RawMessage(after), actorID); err != nil {
		s.internalError(w, "record workflow move", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow move", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "moved"})
}

func (s *server) deleteWorkflowStep(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workflow deletion", err)
		return
	}
	defer tx.Rollback(r.Context())
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	var rootID, lifecycle string
	if err = tx.QueryRow(r.Context(), `SELECT root_id,lifecycle_status FROM work_nodes WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspaceID, nodeID).Scan(&rootID, &lifecycle); err != nil {
		s.writeDatabaseError(w, "find work node", err)
		return
	}
	if lifecycle != "planned" {
		writeError(w, http.StatusConflict, "workflow can only be edited before work starts")
		return
	}
	var position int
	var stepName string
	if err = tx.QueryRow(r.Context(), `SELECT position,name FROM workflow_steps WHERE id=$1 AND work_node_id=$2`, stepID, nodeID).Scan(&position, &stepName); err != nil {
		s.writeDatabaseError(w, "find workflow step", err)
		return
	}
	if tag, queryErr := tx.Exec(r.Context(), `DELETE FROM workflow_steps WHERE id=$1 AND work_node_id=$2`, stepID, nodeID); queryErr != nil || tag.RowsAffected() != 1 {
		if queryErr != nil {
			s.internalError(w, "delete workflow step", queryErr)
		} else {
			writeError(w, http.StatusNotFound, "route stage not found")
		}
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET position=position-1 WHERE work_node_id=$1 AND position>$2`, nodeID, position); err != nil {
		s.internalError(w, "compact workflow positions", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	actorID, _ := s.currentActorID(r.Context(), workspaceID)
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "stepId": stepID, "name": stepName, "position": position})
	if _, err = tx.Exec(r.Context(), `INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step',$4,'workflow_step.deleted',$5,$6)`, workspaceID, rootID, revision, stepID, json.RawMessage(after), actorID); err != nil {
		s.internalError(w, "record workflow deletion", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow deletion", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}
