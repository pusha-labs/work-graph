package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type actorSummary struct {
	ID          string `json:"id" db:"id"`
	DisplayName string `json:"displayName" db:"display_name"`
	ActorType   string `json:"actorType" db:"actor_type"`
}

type performerMatch struct {
	actorSummary
	EvidenceScore         int    `json:"-"`
	ConfirmedRequirements int    `json:"confirmedRequirements"`
	RequirementCount      int    `json:"requirementCount"`
	MatchQuality          string `json:"matchQuality"`
}

type workKnowledgeRequirement struct {
	ID          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	SubjectType string `json:"subjectType" db:"subject_type"`
}

type descendantBlocker struct {
	ID              string `json:"id" db:"id"`
	Title           string `json:"title" db:"title"`
	LifecycleStatus string `json:"lifecycleStatus" db:"lifecycle_status"`
	Depth           int    `json:"depth" db:"depth"`
}

type workflowStep struct {
	ID                    string                     `json:"id"`
	Position              int                        `json:"position"`
	Name                  string                     `json:"name"`
	StepType              string                     `json:"stepType"`
	StepStatus            string                     `json:"stepStatus"`
	ModuleID              *string                    `json:"moduleId"`
	ModuleVersion         *string                    `json:"moduleVersion"`
	Configuration         json.RawMessage            `json:"configuration"`
	DistributionMode      string                     `json:"distributionMode"`
	EffectiveDistribution string                     `json:"effectiveDistributionMode"`
	ClaimedBy             *actorSummary              `json:"claimedBy"`
	Requirements          []capability               `json:"requirements"`
	KnowledgeRequirements []workKnowledgeRequirement `json:"knowledgeRequirements"`
	Executions            []workflowExecution        `json:"executions"`
	Bids                  []workflowStepBid          `json:"bids"`
}

type workflowExecution struct {
	ID                 string          `json:"id"`
	AttemptNumber      int             `json:"attemptNumber"`
	ExecutionStatus    string          `json:"executionStatus"`
	StartedBy          *actorSummary   `json:"startedBy"`
	StartedAt          string          `json:"startedAt"`
	FinishedAt         *string         `json:"finishedAt"`
	Result             json.RawMessage `json:"result"`
	ErrorMessage       *string         `json:"errorMessage"`
	ReturnReason       *string         `json:"returnReason"`
	DefinitionSnapshot json.RawMessage `json:"definitionSnapshot"`
	PromisedMinutes    *int            `json:"promisedDurationMinutes"`
	ActualMinutes      *float64        `json:"actualDurationMinutes"`
	RelativeError      *float64        `json:"relativeError"`
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func validateModuleConfiguration(schemaJSON json.RawMessage, configuration map[string]any) string {
	var schema struct {
		Fields []struct {
			Key      string   `json:"key"`
			Type     string   `json:"type"`
			Required bool     `json:"required"`
			Options  []string `json:"options"`
		} `json:"fields"`
	}
	if json.Unmarshal(schemaJSON, &schema) != nil {
		return "module configuration schema is invalid"
	}
	for _, field := range schema.Fields {
		value, present := configuration[field.Key]
		if field.Required && (!present || value == nil || strings.TrimSpace(stringValue(value)) == "" && field.Type != "number") {
			return field.Key + " is required"
		}
		if !present || value == nil {
			continue
		}
		if field.Type == "number" {
			if _, ok := value.(float64); !ok {
				return field.Key + " must be a number"
			}
		} else if _, ok := value.(string); !ok {
			return field.Key + " must be text"
		}
		if len(field.Options) > 0 {
			matched := false
			for _, option := range field.Options {
				matched = matched || stringValue(value) == option
			}
			if !matched {
				return field.Key + " has an unsupported value"
			}
		}
	}
	return ""
}

func (s *server) validateSecretReference(r *http.Request, workspaceID string, configuration map[string]any) string {
	secretID := strings.TrimSpace(stringValue(configuration["secretId"]))
	if secretID == "" {
		return ""
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_secrets WHERE workspace_id=$1 AND id=$2 AND enabled)`, workspaceID, secretID).Scan(&exists); err != nil || !exists {
		return "selected secret is unavailable"
	}
	placement := stringValue(configuration["secretPlacement"])
	if placement == "header" {
		header := strings.TrimSpace(stringValue(configuration["secretHeader"]))
		if header == "" || !validHTTPHeaderName(header) {
			return "a valid header name is required for header credentials"
		}
	}
	return ""
}

func validHTTPHeaderName(value string) bool {
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
			return false
		}
	}
	return value != ""
}

func (s *server) getWorkCircle(w http.ResponseWriter, r *http.Request) {
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	var currentActor actorSummary
	currentActorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find current actor", err)
		return
	}
	if err := s.db.QueryRow(r.Context(), `SELECT id, display_name, actor_type FROM actors WHERE workspace_id = $1 AND id = $2`, workspaceID, currentActorID).Scan(&currentActor.ID, &currentActor.DisplayName, &currentActor.ActorType); err != nil {
		s.writeDatabaseError(w, "find current actor", err)
		return
	}
	var requester actorSummary
	err = s.db.QueryRow(r.Context(), `
		SELECT a.id, a.display_name, a.actor_type
		FROM work_node_participants p
		JOIN work_nodes n ON n.id = p.work_node_id
		JOIN actors a ON a.id = p.actor_id
		WHERE n.workspace_id = $1 AND n.id = $2 AND p.participant_role = 'requester'
		ORDER BY p.sequence_number LIMIT 1`, workspaceID, nodeID).Scan(&requester.ID, &requester.DisplayName, &requester.ActorType)
	if err != nil {
		s.writeDatabaseError(w, "find requester", err)
		return
	}

	requirementRows, err := s.db.Query(r.Context(), `
		SELECT c.id, c.name, c.capability_type, c.created_at
		FROM work_node_requirements r
		JOIN work_nodes n ON n.id = r.work_node_id
		JOIN capabilities c ON c.id = r.capability_id
		WHERE n.workspace_id = $1 AND n.id = $2
		ORDER BY c.capability_type, c.name`, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "list work requirements", err)
		return
	}
	requirements, err := pgx.CollectRows(requirementRows, pgx.RowToStructByName[capability])
	requirementRows.Close()
	if err != nil {
		s.internalError(w, "read work requirements", err)
		return
	}
	knowledgeRows, err := s.db.Query(r.Context(), `
		SELECT ks.id,ks.name,ks.subject_type
		FROM work_node_knowledge_requirements r
		JOIN work_nodes n ON n.id=r.work_node_id JOIN knowledge_subjects ks ON ks.id=r.subject_id
		WHERE n.workspace_id=$1 AND n.id=$2 ORDER BY ks.subject_type,ks.name`, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "list work knowledge requirements", err)
		return
	}
	knowledgeRequirements, err := pgx.CollectRows(knowledgeRows, pgx.RowToStructByName[workKnowledgeRequirement])
	knowledgeRows.Close()
	if err != nil {
		s.internalError(w, "read work knowledge requirements", err)
		return
	}
	blockerRows, err := s.db.Query(r.Context(), `
		WITH RECURSIVE descendants AS (
			SELECT id,title,lifecycle_status,1 AS depth
			FROM work_nodes WHERE workspace_id=$1 AND parent_id=$2 AND removed_revision IS NULL
			UNION ALL
			SELECT n.id,n.title,n.lifecycle_status,d.depth+1
			FROM work_nodes n JOIN descendants d ON n.parent_id=d.id
			WHERE n.workspace_id=$1 AND n.removed_revision IS NULL
		)
		SELECT id,title,lifecycle_status,depth FROM descendants
		WHERE lifecycle_status <> 'closed' ORDER BY depth,title`, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "list open descendant work", err)
		return
	}
	openDescendants, err := pgx.CollectRows(blockerRows, pgx.RowToStructByName[descendantBlocker])
	blockerRows.Close()
	if err != nil {
		s.internalError(w, "read open descendant work", err)
		return
	}
	stepRows, err := s.db.Query(r.Context(), `
		SELECT s.id,s.position,s.name,s.step_type,s.step_status,s.module_id,s.module_version,s.configuration,s.distribution_mode,CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END,a.id,a.display_name,a.actor_type
		FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspaces w ON w.id=n.workspace_id
		LEFT JOIN actors a ON a.id=s.claimed_by
		WHERE n.workspace_id=$1 AND n.id=$2 ORDER BY s.position`, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "list workflow steps", err)
		return
	}
	steps := []workflowStep{}
	for stepRows.Next() {
		var step workflowStep
		var actorID, actorName, actorType *string
		if err := stepRows.Scan(&step.ID, &step.Position, &step.Name, &step.StepType, &step.StepStatus, &step.ModuleID, &step.ModuleVersion, &step.Configuration, &step.DistributionMode, &step.EffectiveDistribution, &actorID, &actorName, &actorType); err != nil {
			stepRows.Close()
			s.internalError(w, "read workflow step", err)
			return
		}
		if actorID != nil {
			step.ClaimedBy = &actorSummary{ID: *actorID, DisplayName: *actorName, ActorType: *actorType}
		}
		capRows, queryErr := s.db.Query(r.Context(), `SELECT c.id,c.name,c.capability_type,c.created_at FROM workflow_step_capabilities r JOIN capabilities c ON c.id=r.capability_id WHERE r.workflow_step_id=$1 ORDER BY c.capability_type,c.name`, step.ID)
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "list workflow step requirements", queryErr)
			return
		}
		step.Requirements, queryErr = pgx.CollectRows(capRows, pgx.RowToStructByName[capability])
		capRows.Close()
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "read workflow step requirements", queryErr)
			return
		}
		knowledgeStepRows, queryErr := s.db.Query(r.Context(), `SELECT k.id,k.name,k.subject_type FROM workflow_step_knowledge r JOIN knowledge_subjects k ON k.id=r.subject_id WHERE r.workflow_step_id=$1 ORDER BY k.subject_type,k.name`, step.ID)
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "list workflow step knowledge", queryErr)
			return
		}
		step.KnowledgeRequirements, queryErr = pgx.CollectRows(knowledgeStepRows, pgx.RowToStructByName[workKnowledgeRequirement])
		knowledgeStepRows.Close()
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "read workflow step knowledge", queryErr)
			return
		}
		executionRows, queryErr := s.db.Query(r.Context(), `SELECT e.id,e.attempt_number,e.execution_status,a.id,a.display_name,a.actor_type,e.started_at::text,e.finished_at::text,e.result,e.error_message,e.return_reason,e.definition_snapshot,e.promised_duration_minutes,CASE WHEN e.finished_at IS NOT NULL THEN EXTRACT(EPOCH FROM (e.finished_at-e.started_at))/60.0 END,CASE WHEN e.finished_at IS NOT NULL AND e.promised_duration_minutes IS NOT NULL THEN ((EXTRACT(EPOCH FROM (e.finished_at-e.started_at))/60.0)-e.promised_duration_minutes)/e.promised_duration_minutes END FROM workflow_step_executions e LEFT JOIN actors a ON a.id=e.started_by WHERE e.workflow_step_id=$1 ORDER BY e.attempt_number DESC`, step.ID)
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "list workflow executions", queryErr)
			return
		}
		step.Executions = []workflowExecution{}
		for executionRows.Next() {
			var execution workflowExecution
			var actorID, actorName, actorType *string
			if queryErr = executionRows.Scan(&execution.ID, &execution.AttemptNumber, &execution.ExecutionStatus, &actorID, &actorName, &actorType, &execution.StartedAt, &execution.FinishedAt, &execution.Result, &execution.ErrorMessage, &execution.ReturnReason, &execution.DefinitionSnapshot, &execution.PromisedMinutes, &execution.ActualMinutes, &execution.RelativeError); queryErr != nil {
				executionRows.Close()
				stepRows.Close()
				s.internalError(w, "read workflow execution", queryErr)
				return
			}
			if actorID != nil {
				execution.StartedBy = &actorSummary{ID: *actorID, DisplayName: *actorName, ActorType: *actorType}
			}
			step.Executions = append(step.Executions, execution)
		}
		executionRows.Close()
		bidRows, queryErr := s.db.Query(r.Context(), `SELECT `+workflowStepBidColumns+` FROM workflow_step_bids b JOIN actors a ON a.id=b.actor_id LEFT JOIN actor_estimation_stats es ON es.actor_id=b.actor_id WHERE b.workflow_step_id=$1 ORDER BY CASE b.bid_status WHEN 'won' THEN 0 WHEN 'active' THEN 1 ELSE 2 END,b.promised_duration_minutes,b.submitted_at,b.actor_id`, step.ID)
		if queryErr != nil {
			stepRows.Close()
			s.internalError(w, "list workflow step bids", queryErr)
			return
		}
		step.Bids = []workflowStepBid{}
		for bidRows.Next() {
			var bid workflowStepBid
			if queryErr = scanWorkflowStepBid(bidRows, &bid); queryErr != nil {
				bidRows.Close()
				stepRows.Close()
				s.internalError(w, "read workflow step bid", queryErr)
				return
			}
			step.Bids = append(step.Bids, bid)
		}
		bidRows.Close()
		steps = append(steps, step)
	}
	stepRows.Close()

	matches := []performerMatch{}
	if len(requirements) > 0 || len(knowledgeRequirements) > 0 {
		matchRows, err := s.db.Query(r.Context(), `
			SELECT a.id, a.display_name, a.actor_type,
			  COALESCE((SELECT SUM((SELECT MAX(CASE ac.claim_level WHEN 'expert' THEN 40 WHEN 'advanced' THEN 30 WHEN 'working' THEN 20 ELSE 10 END + CASE ac.claim_source WHEN 'admin' THEN 20 WHEN 'imported' THEN 10 WHEN 'inferred' THEN 5 ELSE 0 END) FROM actor_capabilities ac WHERE ac.actor_id=a.id AND ac.capability_id=r.capability_id)) FROM work_node_requirements r WHERE r.work_node_id=$2),0)
			  + COALESCE((SELECT SUM((SELECT MAX(CASE ak.claim_level WHEN 'expert' THEN 40 WHEN 'advanced' THEN 30 WHEN 'working' THEN 20 ELSE 10 END + CASE ak.claim_source WHEN 'admin' THEN 20 WHEN 'imported' THEN 10 WHEN 'inferred' THEN 5 ELSE 0 END) FROM actor_knowledge ak WHERE ak.actor_id=a.id AND ak.subject_id=r.subject_id)) FROM work_node_knowledge_requirements r WHERE r.work_node_id=$2),0) AS evidence_score,
			  (SELECT COUNT(*) FROM work_node_requirements r WHERE r.work_node_id=$2 AND EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=a.id AND ac.capability_id=r.capability_id AND ac.claim_source='admin'))
			  + (SELECT COUNT(*) FROM work_node_knowledge_requirements r WHERE r.work_node_id=$2 AND EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=a.id AND ak.subject_id=r.subject_id AND ak.claim_source='admin')) AS confirmed_requirements,
			  (SELECT COUNT(*) FROM work_node_requirements r WHERE r.work_node_id=$2) + (SELECT COUNT(*) FROM work_node_knowledge_requirements r WHERE r.work_node_id=$2) AS requirement_count
			FROM actors a
			WHERE a.workspace_id = $1
			  AND NOT EXISTS (
				SELECT 1 FROM work_node_requirements r
				WHERE r.work_node_id = $2
				  AND NOT EXISTS (
					SELECT 1 FROM actor_capabilities ac
					WHERE ac.actor_id = a.id AND ac.capability_id = r.capability_id
				  )
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM work_node_knowledge_requirements kr
				WHERE kr.work_node_id=$2
				  AND NOT EXISTS (
					SELECT 1 FROM actor_knowledge ak
					WHERE ak.actor_id=a.id AND ak.subject_id=kr.subject_id
				  )
			  )
			ORDER BY evidence_score DESC, confirmed_requirements DESC, a.display_name`, workspaceID, nodeID)
		if err != nil {
			s.internalError(w, "find matching performers", err)
			return
		}
		for matchRows.Next() {
			var match performerMatch
			if err = matchRows.Scan(&match.ID, &match.DisplayName, &match.ActorType, &match.EvidenceScore, &match.ConfirmedRequirements, &match.RequirementCount); err != nil {
				matchRows.Close()
				s.internalError(w, "read matching performer", err)
				return
			}
			switch {
			case match.ConfirmedRequirements == match.RequirementCount:
				match.MatchQuality = "verified"
			case match.ConfirmedRequirements > 0:
				match.MatchQuality = "partially_verified"
			default:
				match.MatchQuality = "self_reported"
			}
			matches = append(matches, match)
		}
		matchRows.Close()
	}
	var performer *actorSummary
	var performerValue actorSummary
	err = s.db.QueryRow(r.Context(), `
		SELECT a.id, a.display_name, a.actor_type
		FROM workflow_steps s JOIN actors a ON a.id=s.claimed_by
		WHERE s.work_node_id=$1 AND s.step_status='active'
		ORDER BY s.position LIMIT 1`, nodeID).Scan(&performerValue.ID, &performerValue.DisplayName, &performerValue.ActorType)
	if err == nil {
		performer = &performerValue
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.internalError(w, "find performer", err)
		return
	}
	var lifecycleStatus string
	if err := s.db.QueryRow(r.Context(), `SELECT lifecycle_status FROM work_nodes WHERE workspace_id = $1 AND id = $2`, workspaceID, nodeID).Scan(&lifecycleStatus); err != nil {
		s.writeDatabaseError(w, "find work status", err)
		return
	}
	var canClaim bool
	err = s.db.QueryRow(r.Context(), `
		WITH current_step AS (SELECT s.id,s.step_status,s.claimed_by,CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END AS distribution_mode FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspaces w ON w.id=n.workspace_id WHERE s.work_node_id=$1 AND s.step_status<>'completed' ORDER BY s.position LIMIT 1)
		SELECT EXISTS(SELECT 1 FROM current_step s WHERE (s.step_status='assigned' AND s.claimed_by=$2) OR (s.step_status='ready'
		  AND s.distribution_mode='simple'
		  AND (EXISTS(SELECT 1 FROM workflow_step_capabilities WHERE workflow_step_id=s.id) OR EXISTS(SELECT 1 FROM workflow_step_knowledge WHERE workflow_step_id=s.id))
		  AND NOT EXISTS(SELECT 1 FROM workflow_step_capabilities r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=$2 AND ac.capability_id=r.capability_id))
		  AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=$2 AND ak.subject_id=r.subject_id))))`, nodeID, currentActorID).Scan(&canClaim)
	if err != nil {
		s.internalError(w, "check current workflow eligibility", err)
		return
	}
	var hasRetryableStep bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workflow_steps WHERE work_node_id=$1 AND step_type='api' AND step_status='failed')`, nodeID).Scan(&hasRetryableStep); err != nil {
		s.internalError(w, "check retryable workflow step", err)
		return
	}
	var hasCancellableStep bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workflow_steps s JOIN workflow_step_executions e ON e.workflow_step_id=s.id WHERE s.work_node_id=$1 AND s.step_type='api' AND s.step_status='active' AND e.execution_status='running')`, nodeID).Scan(&hasCancellableStep); err != nil {
		s.internalError(w, "check cancellable workflow step", err)
		return
	}
	permissions := map[string]bool{
		"canClaim":  canClaim,
		"canSubmit": performer != nil && performer.ID == currentActor.ID && lifecycleStatus == "active",
		"canClose":  requester.ID == currentActor.ID && lifecycleStatus == "review" && len(openDescendants) == 0,
		"canReturn": requester.ID == currentActor.ID && lifecycleStatus == "review",
		"canReopen": requester.ID == currentActor.ID && lifecycleStatus == "closed",
		"canRetry":  requester.ID == currentActor.ID && lifecycleStatus == "blocked" && hasRetryableStep,
		"canCancel": requester.ID == currentActor.ID && lifecycleStatus == "active" && hasCancellableStep,
	}
	writeJSON(w, http.StatusOK, map[string]any{"requester": requester, "performer": performer, "currentActor": currentActor, "requirements": requirements, "knowledgeRequirements": knowledgeRequirements, "openDescendants": openDescendants, "workflowSteps": steps, "matchingPerformers": matches, "permissions": permissions})
}

func (s *server) addWorkflowStep(w http.ResponseWriter, r *http.Request) {
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	var input struct {
		Name             string          `json:"name"`
		CapabilityID     string          `json:"capabilityId"`
		SubjectID        string          `json:"subjectId"`
		StepType         string          `json:"stepType"`
		ModuleID         string          `json:"moduleId"`
		ModuleVersion    string          `json:"moduleVersion"`
		Configuration    json.RawMessage `json:"configuration"`
		DistributionMode string          `json:"distributionMode"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "step name is required")
		return
	}
	if input.StepType == "" {
		input.StepType = "human"
	}
	if input.Configuration == nil {
		input.Configuration = json.RawMessage(`{}`)
	}
	if input.StepType == "human" {
		input.ModuleID, input.ModuleVersion = "", ""
		if input.DistributionMode == "" {
			input.DistributionMode = "inherit"
		}
		if input.DistributionMode != "inherit" && input.DistributionMode != "simple" && input.DistributionMode != "exchange" {
			writeError(w, http.StatusBadRequest, "distribution mode must be inherit, simple, or exchange")
			return
		}
	} else {
		input.DistributionMode = "inherit"
		var registeredType string
		var configurationSchema json.RawMessage
		if err := s.db.QueryRow(r.Context(), `SELECT step_type,configuration_schema FROM workflow_modules WHERE module_id=$1 AND module_version=$2 AND enabled`, input.ModuleID, input.ModuleVersion).Scan(&registeredType, &configurationSchema); err != nil || registeredType != input.StepType {
			writeError(w, http.StatusBadRequest, "unsupported workflow module binding")
			return
		}
		var configuration map[string]any
		if err := json.Unmarshal(input.Configuration, &configuration); err != nil {
			writeError(w, http.StatusBadRequest, "configuration must be a JSON object")
			return
		}
		if validationError := validateModuleConfiguration(configurationSchema, configuration); validationError != "" {
			writeError(w, http.StatusBadRequest, validationError)
			return
		}
		if validationError := s.validateModulePolicy(r, workspaceID, input.ModuleID, input.ModuleVersion, configuration); validationError != "" {
			writeError(w, http.StatusForbidden, validationError)
			return
		}
		if input.ModuleID == "builtin.http" {
			if validationError := s.validateSecretReference(r, workspaceID, configuration); validationError != "" {
				writeError(w, http.StatusBadRequest, validationError)
				return
			}
		}
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workflow step creation", err)
		return
	}
	defer tx.Rollback(r.Context())
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	var rootID, lifecycle string
	if err := tx.QueryRow(r.Context(), `SELECT root_id,lifecycle_status FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision IS NULL FOR UPDATE`, workspaceID, nodeID).Scan(&rootID, &lifecycle); err != nil {
		s.writeDatabaseError(w, "find work node", err)
		return
	}
	if lifecycle != "planned" {
		writeError(w, http.StatusConflict, "workflow steps can only be added before work starts")
		return
	}
	var step workflowStep
	var moduleID, moduleVersion any
	if input.ModuleID != "" {
		moduleID, moduleVersion = input.ModuleID, input.ModuleVersion
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO workflow_steps(work_node_id,position,name,step_type,step_status,module_id,module_version,configuration,distribution_mode) SELECT $1,COALESCE(MAX(position),0)+1,$2,$3,'pending',$4,$5,$6,$7 FROM workflow_steps WHERE work_node_id=$1 RETURNING id,position,name,step_type,step_status,module_id,module_version,configuration,distribution_mode`, nodeID, input.Name, input.StepType, moduleID, moduleVersion, input.Configuration, input.DistributionMode).Scan(&step.ID, &step.Position, &step.Name, &step.StepType, &step.StepStatus, &step.ModuleID, &step.ModuleVersion, &step.Configuration, &step.DistributionMode)
	if err != nil {
		s.internalError(w, "create workflow step", err)
		return
	}
	step.Requirements = []capability{}
	step.KnowledgeRequirements = []workKnowledgeRequirement{}
	if input.CapabilityID != "" {
		if _, err = tx.Exec(r.Context(), `INSERT INTO workflow_step_capabilities(workflow_step_id,capability_id) SELECT $1,c.id FROM capabilities c WHERE c.workspace_id=$2 AND c.id=$3`, step.ID, workspaceID, input.CapabilityID); err != nil {
			s.internalError(w, "add step capability", err)
			return
		}
	}
	if input.SubjectID != "" {
		if _, err = tx.Exec(r.Context(), `INSERT INTO workflow_step_knowledge(workflow_step_id,subject_id) SELECT $1,k.id FROM knowledge_subjects k WHERE k.workspace_id=$2 AND k.id=$3`, step.ID, workspaceID, input.SubjectID); err != nil {
			s.internalError(w, "add step knowledge", err)
			return
		}
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workflow author", err)
		return
	}
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "workflowStep": step})
	_, err = tx.Exec(r.Context(), `WITH event AS (INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step',$4,'workflow_step.added',$5,$6) RETURNING correlation_id) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id) SELECT $1,'workflow_step.added','work_node',$4,$5,$3,$6,correlation_id FROM event`, workspaceID, rootID, revision, nodeID, json.RawMessage(after), actorID)
	if err != nil {
		s.internalError(w, "record workflow step", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow step", err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (s *server) addWorkRequirement(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CapabilityID string `json:"capabilityId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin requirement creation", err)
		return
	}
	defer tx.Rollback(r.Context())
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	var item capability
	err = tx.QueryRow(r.Context(), `
		WITH inserted AS (
			INSERT INTO work_node_requirements(work_node_id, capability_id)
			SELECT n.id, c.id FROM work_nodes n JOIN capabilities c ON c.workspace_id = n.workspace_id
			WHERE n.workspace_id = $1 AND n.id = $2 AND c.id = $3
			ON CONFLICT DO NOTHING
			RETURNING capability_id
		)
		SELECT c.id, c.name, c.capability_type, c.created_at FROM inserted i JOIN capabilities c ON c.id = i.capability_id`, workspaceID, nodeID, input.CapabilityID).Scan(
		&item.ID, &item.Name, &item.CapabilityType, &item.CreatedAt,
	)
	if err != nil {
		s.writeDatabaseError(w, "add work requirement", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO workflow_step_capabilities(workflow_step_id,capability_id) SELECT s.id,$2 FROM workflow_steps s WHERE s.work_node_id=$1 AND s.step_status<>'completed' ORDER BY s.position LIMIT 1 ON CONFLICT DO NOTHING`, nodeID, input.CapabilityID); err != nil {
		s.internalError(w, "add current step requirement", err)
		return
	}
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "capability": item})
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find requirement author", err)
		return
	}
	var rootID string
	if err := tx.QueryRow(r.Context(), `SELECT root_id FROM work_nodes WHERE workspace_id = $1 AND id = $2`, workspaceID, nodeID).Scan(&rootID); err != nil {
		s.writeDatabaseError(w, "find requirement root", err)
		return
	}
	_, err = tx.Exec(r.Context(), `
		WITH event AS (
			INSERT INTO change_events(workspace_id, root_id, workspace_revision, correlation_id, entity_type, entity_id, event_type, after_state, actor_id)
			VALUES ($1, $2, $3, uuidv7(), 'work_requirement', $4, 'requirement.added', $5, $6)
			RETURNING correlation_id
		)
		INSERT INTO outbox_events(workspace_id, event_type, aggregate_type, aggregate_id, payload, workspace_revision, actor_id, correlation_id)
		SELECT $1, 'requirement.added', 'work_node', $4, $5, $3, $6, correlation_id FROM event`, workspaceID, rootID, revision, nodeID, json.RawMessage(after), actorID)
	if err != nil {
		s.internalError(w, "record work requirement", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit work requirement", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) addWorkKnowledgeRequirement(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SubjectID string `json:"subjectId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin knowledge requirement creation", err)
		return
	}
	defer tx.Rollback(r.Context())
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	var item workKnowledgeRequirement
	err = tx.QueryRow(r.Context(), `
		WITH inserted AS (
			INSERT INTO work_node_knowledge_requirements(work_node_id,subject_id)
			SELECT n.id,ks.id FROM work_nodes n JOIN knowledge_subjects ks ON ks.workspace_id=n.workspace_id
			WHERE n.workspace_id=$1 AND n.id=$2 AND ks.id=$3
			ON CONFLICT DO NOTHING RETURNING subject_id
		)
		SELECT ks.id,ks.name,ks.subject_type FROM inserted i JOIN knowledge_subjects ks ON ks.id=i.subject_id`, workspaceID, nodeID, input.SubjectID).Scan(&item.ID, &item.Name, &item.SubjectType)
	if err != nil {
		s.writeDatabaseError(w, "add work knowledge requirement", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO workflow_step_knowledge(workflow_step_id,subject_id) SELECT s.id,$2 FROM workflow_steps s WHERE s.work_node_id=$1 AND s.step_status<>'completed' ORDER BY s.position LIMIT 1 ON CONFLICT DO NOTHING`, nodeID, input.SubjectID); err != nil {
		s.internalError(w, "add current step knowledge", err)
		return
	}
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "knowledgeSubject": item})
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find knowledge requirement author", err)
		return
	}
	var rootID string
	if err := tx.QueryRow(r.Context(), `SELECT root_id FROM work_nodes WHERE workspace_id=$1 AND id=$2`, workspaceID, nodeID).Scan(&rootID); err != nil {
		s.writeDatabaseError(w, "find knowledge requirement root", err)
		return
	}
	_, err = tx.Exec(r.Context(), `
		WITH event AS (
			INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id)
			VALUES ($1,$2,$3,uuidv7(),'work_knowledge_requirement',$4,'knowledge_requirement.added',$5,$6) RETURNING correlation_id
		)
		INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id)
		SELECT $1,'knowledge_requirement.added','work_node',$4,$5,$3,$6,correlation_id FROM event`, workspaceID, rootID, revision, nodeID, json.RawMessage(after), actorID)
	if err != nil {
		s.internalError(w, "record work knowledge requirement", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit work knowledge requirement", err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) performWorkAction(w http.ResponseWriter, r *http.Request) {
	workspaceID, nodeID, action := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("action")
	if action != "claim" && action != "submit" && action != "close" && action != "reopen" && action != "return" && action != "retry" && action != "cancel" {
		writeError(w, http.StatusBadRequest, "unknown work action")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin work action", err)
		return
	}
	defer tx.Rollback(r.Context())
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	var before workNode
	if err := scanNode(tx.QueryRow(r.Context(), nodeByIDSQL+` FOR UPDATE`, workspaceID, nodeID), &before); err != nil {
		s.writeDatabaseError(w, "find work node", err)
		return
	}
	currentActorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find current actor", err)
		return
	}

	nextStatus := ""
	switch action {
	case "claim":
		var stepID string
		err = tx.QueryRow(r.Context(), `
			WITH current_step AS (SELECT s.id,s.step_status,s.claimed_by,CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END AS distribution_mode FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspaces w ON w.id=n.workspace_id WHERE s.work_node_id=$1 AND s.step_status IN ('ready','assigned') ORDER BY s.position LIMIT 1)
			SELECT id FROM current_step s WHERE (s.step_status='assigned' AND s.claimed_by=$2) OR (s.step_status='ready' AND
			  s.distribution_mode='simple' AND
			  (EXISTS(SELECT 1 FROM workflow_step_capabilities WHERE workflow_step_id=s.id) OR EXISTS(SELECT 1 FROM workflow_step_knowledge WHERE workflow_step_id=s.id))
			  AND NOT EXISTS(SELECT 1 FROM workflow_step_capabilities r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=$2 AND ac.capability_id=r.capability_id))
			  AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=$2 AND ak.subject_id=r.subject_id)))`, nodeID, currentActorID).Scan(&stepID)
		if errors.Is(err, pgx.ErrNoRows) || before.LifecycleStatus != "planned" {
			writeError(w, http.StatusConflict, "current user cannot claim this task")
			return
		}
		if err != nil {
			s.internalError(w, "check performer eligibility", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='active',claimed_by=$2,started_at=now() WHERE id=$1`, stepID, currentActorID); err != nil {
			s.internalError(w, "claim workflow step", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO workflow_step_executions(workflow_step_id,attempt_number,execution_status,started_by,definition_snapshot,selected_bid_id,promised_duration_minutes) SELECT s.id,COALESCE((SELECT MAX(e.attempt_number) FROM workflow_step_executions e WHERE e.workflow_step_id=s.id),0)+1,'running',$2,jsonb_build_object('name',s.name,'stepType',s.step_type,'moduleId',s.module_id,'moduleVersion',s.module_version,'configuration',s.configuration,'capabilityIds',COALESCE((SELECT jsonb_agg(capability_id) FROM workflow_step_capabilities WHERE workflow_step_id=s.id),'[]'::jsonb),'subjectIds',COALESCE((SELECT jsonb_agg(subject_id) FROM workflow_step_knowledge WHERE workflow_step_id=s.id),'[]'::jsonb)),s.selected_bid_id,b.promised_duration_minutes FROM workflow_steps s LEFT JOIN workflow_step_bids b ON b.id=s.selected_bid_id WHERE s.id=$1`, stepID, currentActorID); err != nil {
			s.internalError(w, "start workflow execution", err)
			return
		}
		nextStatus = "active"
	case "submit":
		var stepID string
		err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 AND step_status='active' AND claimed_by=$2 ORDER BY position LIMIT 1`, nodeID, currentActorID).Scan(&stepID)
		if errors.Is(err, pgx.ErrNoRows) || before.LifecycleStatus != "active" {
			writeError(w, http.StatusConflict, "only the active performer can submit this task")
			return
		}
		if err != nil {
			s.internalError(w, "find active workflow step", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='completed',completed_at=now() WHERE id=$1`, stepID); err != nil {
			s.internalError(w, "complete workflow step", err)
			return
		}
		if tag, queryErr := tx.Exec(r.Context(), `UPDATE workflow_step_executions SET execution_status='succeeded',finished_at=now() WHERE workflow_step_id=$1 AND execution_status='running'`, stepID); queryErr != nil || tag.RowsAffected() != 1 {
			if queryErr != nil {
				s.internalError(w, "complete workflow execution", queryErr)
			} else {
				writeError(w, http.StatusConflict, "running execution was not found")
			}
			return
		}
		if _, err = tx.Exec(r.Context(), `
			WITH observation AS (
			  SELECT started_by AS actor_id,((EXTRACT(EPOCH FROM (finished_at-started_at))/60.0)-promised_duration_minutes)/promised_duration_minutes AS relative_error,
			         ABS(((EXTRACT(EPOCH FROM (finished_at-started_at))/60.0)-promised_duration_minutes)/promised_duration_minutes) AS absolute_error,
			         EXTRACT(EPOCH FROM (finished_at-started_at))/60.0/promised_duration_minutes AS duration_ratio
			  FROM workflow_step_executions WHERE workflow_step_id=$1 AND execution_status='succeeded' AND promised_duration_minutes IS NOT NULL ORDER BY attempt_number DESC LIMIT 1
			)
			INSERT INTO actor_estimation_stats(actor_id,observation_count,mean_relative_error,relative_error_m2,mean_absolute_relative_error,long_overrun_count,long_overrun_severity_sum)
			SELECT actor_id,1,relative_error,0,absolute_error,CASE WHEN duration_ratio>2 THEN 1 ELSE 0 END,CASE WHEN duration_ratio>2 THEN duration_ratio-2 ELSE 0 END FROM observation
			ON CONFLICT(actor_id) DO UPDATE SET
			  relative_error_m2=actor_estimation_stats.relative_error_m2+(EXCLUDED.mean_relative_error-actor_estimation_stats.mean_relative_error)*(EXCLUDED.mean_relative_error-(actor_estimation_stats.mean_relative_error+(EXCLUDED.mean_relative_error-actor_estimation_stats.mean_relative_error)/(actor_estimation_stats.observation_count+1))),
			  mean_relative_error=actor_estimation_stats.mean_relative_error+(EXCLUDED.mean_relative_error-actor_estimation_stats.mean_relative_error)/(actor_estimation_stats.observation_count+1),
			  mean_absolute_relative_error=actor_estimation_stats.mean_absolute_relative_error+(EXCLUDED.mean_absolute_relative_error-actor_estimation_stats.mean_absolute_relative_error)/(actor_estimation_stats.observation_count+1),
			  observation_count=actor_estimation_stats.observation_count+1,
			  long_overrun_count=actor_estimation_stats.long_overrun_count+EXCLUDED.long_overrun_count,
			  long_overrun_severity_sum=actor_estimation_stats.long_overrun_severity_sum+EXCLUDED.long_overrun_severity_sum,
			  updated_at=now()`, stepID); err != nil {
			s.internalError(w, "update performer estimation statistics", err)
			return
		}
		var nextStepID string
		err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 AND step_status='pending' ORDER BY position LIMIT 1`, nodeID).Scan(&nextStepID)
		if err == nil {
			if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='ready' WHERE id=$1`, nextStepID); err != nil {
				s.internalError(w, "activate next workflow step", err)
				return
			}
			nextStatus = "planned"
		} else if errors.Is(err, pgx.ErrNoRows) {
			nextStatus = "review"
		} else {
			s.internalError(w, "find next workflow step", err)
			return
		}
	case "close":
		var allowed bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_node_participants WHERE work_node_id = $1 AND actor_id = $2 AND participant_role = 'requester')`, nodeID, currentActorID).Scan(&allowed)
		if err != nil || !allowed || before.LifecycleStatus != "review" {
			writeError(w, http.StatusConflict, "only the requester can close a task awaiting review")
			return
		}
		var hasOpenDescendants bool
		err = tx.QueryRow(r.Context(), `
			WITH RECURSIVE descendants AS (
				SELECT id,lifecycle_status FROM work_nodes WHERE workspace_id=$1 AND parent_id=$2 AND removed_revision IS NULL
				UNION ALL
				SELECT n.id,n.lifecycle_status FROM work_nodes n JOIN descendants d ON n.parent_id=d.id
				WHERE n.workspace_id=$1 AND n.removed_revision IS NULL
			)
			SELECT EXISTS(SELECT 1 FROM descendants WHERE lifecycle_status <> 'closed')`, workspaceID, nodeID).Scan(&hasOpenDescendants)
		if err != nil {
			s.internalError(w, "check descendant work", err)
			return
		}
		if hasOpenDescendants {
			writeError(w, http.StatusConflict, "all child work must be closed before this task can be closed")
			return
		}
		nextStatus = "closed"
	case "return":
		var input struct {
			StepID string `json:"stepId"`
			Reason string `json:"reason"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		input.Reason = strings.TrimSpace(input.Reason)
		if input.StepID == "" || input.Reason == "" {
			writeError(w, http.StatusBadRequest, "step and return reason are required")
			return
		}
		var allowed bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_node_participants WHERE work_node_id=$1 AND actor_id=$2 AND participant_role='requester')`, nodeID, currentActorID).Scan(&allowed)
		if err != nil {
			s.internalError(w, "check requester", err)
			return
		}
		if !allowed || before.LifecycleStatus != "review" {
			writeError(w, http.StatusConflict, "only the requester can return work awaiting review")
			return
		}
		var returnedPosition int
		var returnedName string
		err = tx.QueryRow(r.Context(), `SELECT position,name FROM workflow_steps WHERE id=$1 AND work_node_id=$2 AND step_status='completed'`, input.StepID, nodeID).Scan(&returnedPosition, &returnedName)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "only a completed step can be returned")
			return
		}
		if err != nil {
			s.internalError(w, "find returned workflow step", err)
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status=CASE WHEN position=$2 THEN 'ready' ELSE 'pending' END,claimed_by=NULL,started_at=NULL,completed_at=NULL WHERE work_node_id=$1 AND position >= $2`, nodeID, returnedPosition)
		if err != nil {
			s.internalError(w, "return workflow steps", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_step_executions e SET execution_status=CASE WHEN e.workflow_step_id=$2 THEN 'returned' ELSE 'superseded' END,return_reason=CASE WHEN e.workflow_step_id=$2 THEN $3 ELSE 'An earlier workflow step was returned for revision' END,finished_at=COALESCE(finished_at,now()) FROM workflow_steps s WHERE e.workflow_step_id=s.id AND s.work_node_id=$1 AND s.position >= $4 AND e.execution_status='succeeded'`, nodeID, input.StepID, input.Reason, returnedPosition); err != nil {
			s.internalError(w, "record returned executions", err)
			return
		}
		after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "workflowStep": map[string]any{"id": input.StepID, "name": returnedName}, "reason": input.Reason})
		_, err = tx.Exec(r.Context(), `WITH event AS (
			INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id)
			VALUES($1,$2,$3,uuidv7(),'workflow_step',$4,'workflow.returned',$5,$6) RETURNING correlation_id
		) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id)
		  SELECT $1,'workflow.returned','work_node',$4,$5,$3,$6,correlation_id FROM event`, workspaceID, before.RootID, revision, nodeID, json.RawMessage(after), currentActorID)
		if err != nil {
			s.internalError(w, "record workflow return", err)
			return
		}
		nextStatus = "planned"
	case "reopen":
		var allowed bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_node_participants WHERE work_node_id = $1 AND actor_id = $2 AND participant_role = 'requester')`, nodeID, currentActorID).Scan(&allowed)
		if err != nil || !allowed || before.LifecycleStatus != "closed" {
			writeError(w, http.StatusConflict, "only the requester can reopen a closed task")
			return
		}
		var lastStepID string
		err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 ORDER BY position DESC LIMIT 1`, nodeID).Scan(&lastStepID)
		if err != nil {
			s.internalError(w, "find last workflow step", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='ready',claimed_by=NULL,started_at=NULL,completed_at=NULL WHERE id=$1`, lastStepID); err != nil {
			s.internalError(w, "reopen workflow step", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_step_executions SET execution_status='returned',return_reason='Task was reopened after acceptance',finished_at=COALESCE(finished_at,now()) WHERE workflow_step_id=$1 AND execution_status='succeeded'`, lastStepID); err != nil {
			s.internalError(w, "record reopened execution", err)
			return
		}
		nextStatus = "planned"
	case "retry":
		var allowed bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_node_participants WHERE work_node_id=$1 AND actor_id=$2 AND participant_role='requester')`, nodeID, currentActorID).Scan(&allowed)
		if err != nil {
			s.internalError(w, "check requester", err)
			return
		}
		if !allowed || before.LifecycleStatus != "blocked" {
			writeError(w, http.StatusConflict, "only the requester can retry a blocked task")
			return
		}
		var failedStepID string
		err = tx.QueryRow(r.Context(), `SELECT id FROM workflow_steps WHERE work_node_id=$1 AND step_type='api' AND step_status='failed' ORDER BY position LIMIT 1 FOR UPDATE`, nodeID).Scan(&failedStepID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "no failed HTTP step is available to retry")
			return
		}
		if err != nil {
			s.internalError(w, "find failed HTTP step", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='ready',claimed_by=NULL,started_at=NULL,completed_at=NULL WHERE id=$1`, failedStepID); err != nil {
			s.internalError(w, "prepare HTTP step retry", err)
			return
		}
		nextStatus = "planned"
	case "cancel":
		var allowed bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM work_node_participants WHERE work_node_id=$1 AND actor_id=$2 AND participant_role='requester')`, nodeID, currentActorID).Scan(&allowed)
		if err != nil {
			s.internalError(w, "check requester", err)
			return
		}
		if !allowed || before.LifecycleStatus != "active" {
			writeError(w, http.StatusConflict, "only the requester can cancel an active automated step")
			return
		}
		var stepID, executionID string
		err = tx.QueryRow(r.Context(), `SELECT s.id,e.id FROM workflow_steps s JOIN workflow_step_executions e ON e.workflow_step_id=s.id WHERE s.work_node_id=$1 AND s.step_type='api' AND s.step_status='active' AND e.execution_status='running' ORDER BY s.position LIMIT 1 FOR UPDATE OF s,e`, nodeID).Scan(&stepID, &executionID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "no active HTTP execution is available to cancel")
			return
		}
		if err != nil {
			s.internalError(w, "find active HTTP execution", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_step_executions SET execution_status='cancelled',finished_at=now(),error_message='Cancelled by requester' WHERE id=$1 AND execution_status='running'`, executionID); err != nil {
			s.internalError(w, "cancel HTTP execution", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='failed',completed_at=now() WHERE id=$1`, stepID); err != nil {
			s.internalError(w, "mark cancelled HTTP step", err)
			return
		}
		nextStatus = "blocked"
	}

	var updated workNode
	err = scanNode(tx.QueryRow(r.Context(), `
		UPDATE work_nodes SET lifecycle_status = $3, updated_revision = $4, updated_at = now()
		WHERE workspace_id = $1 AND id = $2
		RETURNING id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
		          created_revision, updated_revision, created_at, updated_at`, workspaceID, nodeID, nextStatus, revision), &updated)
	if err != nil {
		s.writeDatabaseError(w, "update work action", err)
		return
	}
	if err := recordNodeChange(r.Context(), tx, updated, "node.updated", revision, before, currentActorID); err != nil {
		s.internalError(w, "record work action", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit work action", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
