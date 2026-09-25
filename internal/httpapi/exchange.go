package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type workflowStepBid struct {
	ID                      string             `json:"id"`
	WorkflowStepID          string             `json:"workflowStepId"`
	Actor                   actorSummary       `json:"actor"`
	PromisedDurationMinutes int                `json:"promisedDurationMinutes"`
	Status                  string             `json:"status"`
	SubmittedAt             time.Time          `json:"submittedAt"`
	UpdatedAt               time.Time          `json:"updatedAt"`
	WithdrawnAt             *time.Time         `json:"withdrawnAt"`
	Estimation              estimationStats    `json:"estimation"`
	RiskAssessment          *bidRiskAssessment `json:"riskAssessment,omitempty"`
}

type bidRiskAssessment struct {
	AdjustedDurationMinutes  int `json:"adjustedDurationMinutes"`
	ExpectedDurationMinutes  int `json:"expectedDurationMinutes"`
	UncertaintyBufferMinutes int `json:"uncertaintyBufferMinutes"`
}

const minimumRiskAssessmentObservations = 5

func assessBidRisk(proposedMinutes int, stats estimationStats) *bidRiskAssessment {
	if stats.ObservationCount < minimumRiskAssessmentObservations {
		return nil
	}
	expectedFactor := math.Max(0.25, 1+stats.MeanRelativeError)
	expected := int(math.Ceil(float64(proposedMinutes) * expectedFactor))
	buffer := int(math.Ceil(float64(proposedMinutes) * 1.645 * stats.Stability / math.Sqrt(float64(stats.ObservationCount))))
	return &bidRiskAssessment{
		ExpectedDurationMinutes:  expected,
		UncertaintyBufferMinutes: buffer,
		AdjustedDurationMinutes:  expected + buffer,
	}
}

type exchangeOffer struct {
	NodeID                string           `json:"nodeId"`
	NodeTitle             string           `json:"nodeTitle"`
	RootID                string           `json:"rootId"`
	RootTitle             string           `json:"rootTitle"`
	StepID                string           `json:"stepId"`
	StepName              string           `json:"stepName"`
	StepPosition          int              `json:"stepPosition"`
	Requester             string           `json:"requester"`
	Requirements          []string         `json:"requirements"`
	KnowledgeRequirements []string         `json:"knowledgeRequirements"`
	ActiveBidCount        int              `json:"activeBidCount"`
	MyBid                 *workflowStepBid `json:"myBid"`
}

func scanWorkflowStepBid(row rowScanner, bid *workflowStepBid) error {
	var m2, severitySum float64
	if err := row.Scan(&bid.ID, &bid.WorkflowStepID, &bid.Actor.ID, &bid.Actor.DisplayName, &bid.Actor.ActorType, &bid.PromisedDurationMinutes, &bid.Status, &bid.SubmittedAt, &bid.UpdatedAt, &bid.WithdrawnAt, &bid.Estimation.ObservationCount, &bid.Estimation.MeanRelativeError, &m2, &bid.Estimation.MeanAbsoluteRelativeError, &bid.Estimation.LongOverrunCount, &severitySum, &bid.Estimation.UpdatedAt); err != nil {
		return err
	}
	if bid.Estimation.ObservationCount > 1 {
		bid.Estimation.Stability = math.Sqrt(m2 / float64(bid.Estimation.ObservationCount-1))
	}
	if bid.Estimation.ObservationCount > 0 {
		bid.Estimation.LongOverrunRate = float64(bid.Estimation.LongOverrunCount) / float64(bid.Estimation.ObservationCount)
	}
	if bid.Estimation.LongOverrunCount > 0 {
		bid.Estimation.MeanLongOverrunSeverity = severitySum / float64(bid.Estimation.LongOverrunCount)
	}
	bid.RiskAssessment = assessBidRisk(bid.PromisedDurationMinutes, bid.Estimation)
	return nil
}

const workflowStepBidColumns = `b.id,b.workflow_step_id,a.id,a.display_name,a.actor_type,b.promised_duration_minutes,b.bid_status,b.submitted_at,b.updated_at,b.withdrawn_at,COALESCE(es.observation_count,0),COALESCE(es.mean_relative_error,0),COALESCE(es.relative_error_m2,0),COALESCE(es.mean_absolute_relative_error,0),COALESCE(es.long_overrun_count,0),COALESCE(es.long_overrun_severity_sum,0),es.updated_at`

func (s *server) listMyExchangeOffers(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find exchange actor", err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT n.id,n.title,root.id,root.title,s.id,s.name,s.position,requester.display_name,
		  COALESCE((SELECT array_agg(c.name ORDER BY c.name) FROM workflow_step_capabilities req JOIN capabilities c ON c.id=req.capability_id WHERE req.workflow_step_id=s.id),'{}'::text[]),
		  COALESCE((SELECT array_agg(k.name ORDER BY k.name) FROM workflow_step_knowledge req JOIN knowledge_subjects k ON k.id=req.subject_id WHERE req.workflow_step_id=s.id),'{}'::text[]),
		  (SELECT COUNT(*) FROM workflow_step_bids active_bid WHERE active_bid.workflow_step_id=s.id AND active_bid.bid_status='active'),
		  mine.id,mine.promised_duration_minutes,mine.bid_status,mine.submitted_at,mine.updated_at,mine.withdrawn_at
		FROM workflow_steps s
		JOIN work_nodes n ON n.id=s.work_node_id
		JOIN workspaces w ON w.id=n.workspace_id
		JOIN work_nodes root ON root.id=n.root_id
		JOIN work_node_participants rp ON rp.work_node_id=n.id AND rp.participant_role='requester'
		JOIN actors requester ON requester.id=rp.actor_id
		LEFT JOIN workflow_step_bids mine ON mine.workflow_step_id=s.id AND mine.actor_id=$2
		WHERE n.workspace_id=$1 AND n.removed_revision IS NULL AND n.lifecycle_status IN ('planned','active')
		  AND s.step_type='human' AND s.step_status='ready' AND s.claimed_by IS NULL
		  AND CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END='exchange'
		  AND NOT EXISTS(SELECT 1 FROM workflow_step_capabilities req WHERE req.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=$2 AND ac.capability_id=req.capability_id))
		  AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge req WHERE req.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=$2 AND ak.subject_id=req.subject_id))
		ORDER BY (mine.bid_status='active') DESC,root.title,n.title,s.position`, workspaceID, actorID)
	if err != nil {
		s.internalError(w, "list exchange offers", err)
		return
	}
	defer rows.Close()
	offers := []exchangeOffer{}
	for rows.Next() {
		var offer exchangeOffer
		var bidID, bidStatus *string
		var bidMinutes *int
		var submittedAt, updatedAt, withdrawnAt *time.Time
		if err := rows.Scan(&offer.NodeID, &offer.NodeTitle, &offer.RootID, &offer.RootTitle, &offer.StepID, &offer.StepName, &offer.StepPosition, &offer.Requester, &offer.Requirements, &offer.KnowledgeRequirements, &offer.ActiveBidCount, &bidID, &bidMinutes, &bidStatus, &submittedAt, &updatedAt, &withdrawnAt); err != nil {
			s.internalError(w, "read exchange offer", err)
			return
		}
		if bidID != nil {
			offer.MyBid = &workflowStepBid{ID: *bidID, WorkflowStepID: offer.StepID, Actor: actorSummary{ID: actorID}, PromisedDurationMinutes: *bidMinutes, Status: *bidStatus, SubmittedAt: *submittedAt, UpdatedAt: *updatedAt, WithdrawnAt: withdrawnAt}
		}
		offers = append(offers, offer)
	}
	writeJSON(w, http.StatusOK, offers)
}

func (s *server) listWorkflowStepBids(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT `+workflowStepBidColumns+` FROM workflow_step_bids b JOIN workflow_steps s ON s.id=b.workflow_step_id JOIN work_nodes n ON n.id=s.work_node_id JOIN actors a ON a.id=b.actor_id LEFT JOIN actor_estimation_stats es ON es.actor_id=b.actor_id WHERE n.workspace_id=$1 AND n.id=$2 AND s.id=$3 ORDER BY CASE b.bid_status WHEN 'active' THEN 0 WHEN 'won' THEN 1 ELSE 2 END,b.promised_duration_minutes,b.submitted_at,b.actor_id`, r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID"))
	if err != nil {
		s.internalError(w, "list workflow step bids", err)
		return
	}
	defer rows.Close()
	bids := []workflowStepBid{}
	for rows.Next() {
		var bid workflowStepBid
		if err := scanWorkflowStepBid(rows, &bid); err != nil {
			s.internalError(w, "read workflow step bid", err)
			return
		}
		bids = append(bids, bid)
	}
	writeJSON(w, http.StatusOK, bids)
}

func (s *server) putMyWorkflowStepBid(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PromisedDurationMinutes int `json:"promisedDurationMinutes"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.PromisedDurationMinutes < 1 || input.PromisedDurationMinutes > 525600 {
		writeError(w, http.StatusBadRequest, "promised duration must be between 1 and 525600 minutes")
		return
	}
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find bidding actor", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin bid submission", err)
		return
	}
	defer tx.Rollback(r.Context())
	var rootID, stepType, stepStatus, distributionMode string
	var claimed bool
	var eligible bool
	err = tx.QueryRow(r.Context(), `SELECT n.root_id,s.step_type,s.step_status,CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END,s.claimed_by IS NOT NULL,
		NOT EXISTS(SELECT 1 FROM workflow_step_capabilities req WHERE req.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=$4 AND ac.capability_id=req.capability_id))
		AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge req WHERE req.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=$4 AND ak.subject_id=req.subject_id))
		FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspaces w ON w.id=n.workspace_id WHERE n.workspace_id=$1 AND n.id=$2 AND s.id=$3 AND n.removed_revision IS NULL FOR UPDATE OF s`, workspaceID, nodeID, stepID, actorID).Scan(&rootID, &stepType, &stepStatus, &distributionMode, &claimed, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workflow step not found")
		return
	}
	if err != nil {
		s.internalError(w, "read workflow step for bid", err)
		return
	}
	if stepType != "human" || stepStatus != "ready" || claimed || distributionMode != "exchange" {
		writeError(w, http.StatusConflict, "only an unclaimed ready human step accepts bids")
		return
	}
	if !eligible {
		writeError(w, http.StatusForbidden, "your capabilities do not satisfy every step requirement")
		return
	}
	var before json.RawMessage
	_ = tx.QueryRow(r.Context(), `SELECT jsonb_build_object('id',id,'workflowStepId',workflow_step_id,'actorId',actor_id,'promisedDurationMinutes',promised_duration_minutes,'status',bid_status,'submittedAt',submitted_at,'updatedAt',updated_at) FROM workflow_step_bids WHERE workflow_step_id=$1 AND actor_id=$2`, stepID, actorID).Scan(&before)
	var bid workflowStepBid
	err = scanWorkflowStepBid(tx.QueryRow(r.Context(), `WITH changed AS (
		INSERT INTO workflow_step_bids(workflow_step_id,actor_id,promised_duration_minutes) VALUES($1,$2,$3)
		ON CONFLICT(workflow_step_id,actor_id) DO UPDATE SET promised_duration_minutes=EXCLUDED.promised_duration_minutes,bid_status='active',updated_at=now(),withdrawn_at=NULL
		RETURNING *) SELECT `+workflowStepBidColumns+` FROM changed b JOIN actors a ON a.id=b.actor_id LEFT JOIN actor_estimation_stats es ON es.actor_id=b.actor_id`, stepID, actorID, input.PromisedDurationMinutes), &bid)
	if err != nil {
		s.internalError(w, "save workflow step bid", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "advance workspace revision for bid", err)
		return
	}
	after, _ := json.Marshal(bid)
	eventType := "workflow_step_bid.submitted"
	if len(before) > 0 {
		eventType = "workflow_step_bid.updated"
	}
	if _, err = tx.Exec(r.Context(), `WITH event AS (INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,before_state,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step_bid',$4,$5,$6,$7,$8) RETURNING correlation_id) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id) SELECT $1,$5,'workflow_step',$9,$7,$3,$8,correlation_id FROM event`, workspaceID, rootID, revision, bid.ID, eventType, before, json.RawMessage(after), actorID, stepID); err != nil {
		s.internalError(w, "record workflow step bid", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow step bid", err)
		return
	}
	writeJSON(w, http.StatusOK, bid)
}

func (s *server) withdrawMyWorkflowStepBid(w http.ResponseWriter, r *http.Request) {
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find bidding actor", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin bid withdrawal", err)
		return
	}
	defer tx.Rollback(r.Context())
	var rootID string
	var before json.RawMessage
	err = tx.QueryRow(r.Context(), `SELECT n.root_id,jsonb_build_object('id',b.id,'workflowStepId',b.workflow_step_id,'actorId',b.actor_id,'promisedDurationMinutes',b.promised_duration_minutes,'status',b.bid_status,'submittedAt',b.submitted_at,'updatedAt',b.updated_at) FROM workflow_step_bids b JOIN workflow_steps s ON s.id=b.workflow_step_id JOIN work_nodes n ON n.id=s.work_node_id WHERE n.workspace_id=$1 AND n.id=$2 AND s.id=$3 AND b.actor_id=$4 AND b.bid_status='active' FOR UPDATE OF b`, workspaceID, nodeID, stepID, actorID).Scan(&rootID, &before)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "active bid not found")
		return
	}
	if err != nil {
		s.internalError(w, "read workflow step bid", err)
		return
	}
	var bid workflowStepBid
	err = scanWorkflowStepBid(tx.QueryRow(r.Context(), `WITH changed AS (UPDATE workflow_step_bids SET bid_status='withdrawn',withdrawn_at=now(),updated_at=now() WHERE workflow_step_id=$1 AND actor_id=$2 RETURNING *) SELECT `+workflowStepBidColumns+` FROM changed b JOIN actors a ON a.id=b.actor_id LEFT JOIN actor_estimation_stats es ON es.actor_id=b.actor_id`, stepID, actorID), &bid)
	if err != nil {
		s.internalError(w, "withdraw workflow step bid", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "advance workspace revision for bid withdrawal", err)
		return
	}
	after, _ := json.Marshal(bid)
	if _, err = tx.Exec(r.Context(), `WITH event AS (INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,before_state,after_state,actor_id) VALUES($1,$2,$3,uuidv7(),'workflow_step_bid',$4,'workflow_step_bid.withdrawn',$5,$6,$7) RETURNING correlation_id) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id) SELECT $1,'workflow_step_bid.withdrawn','workflow_step',$8,$6,$3,$7,correlation_id FROM event`, workspaceID, rootID, revision, bid.ID, before, json.RawMessage(after), actorID, stepID); err != nil {
		s.internalError(w, "record workflow step bid withdrawal", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow step bid withdrawal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) selectWorkflowStepBid(w http.ResponseWriter, r *http.Request) {
	workspaceID, nodeID, stepID := r.PathValue("workspaceID"), r.PathValue("nodeID"), r.PathValue("stepID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find bid selector", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin bid selection", err)
		return
	}
	defer tx.Rollback(r.Context())
	var rootID, effectiveMode string
	var requester bool
	err = tx.QueryRow(r.Context(), `SELECT n.root_id,CASE s.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE s.distribution_mode END,EXISTS(SELECT 1 FROM work_node_participants p WHERE p.work_node_id=n.id AND p.actor_id=$4 AND p.participant_role='requester') FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workspaces w ON w.id=n.workspace_id WHERE n.workspace_id=$1 AND n.id=$2 AND s.id=$3 AND s.step_type='human' AND s.step_status='ready' AND s.claimed_by IS NULL FOR UPDATE OF s`, workspaceID, nodeID, stepID, actorID).Scan(&rootID, &effectiveMode, &requester)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "only a ready unassigned human step can close bidding")
		return
	}
	if err != nil {
		s.internalError(w, "read step for bid selection", err)
		return
	}
	if !requester {
		writeError(w, http.StatusForbidden, "only the requester can select a performer")
		return
	}
	if effectiveMode != "exchange" {
		writeError(w, http.StatusConflict, "this step does not use exchange distribution")
		return
	}
	var bidID, winnerID string
	var promisedMinutes int
	err = tx.QueryRow(r.Context(), `SELECT id,actor_id,promised_duration_minutes FROM workflow_step_bids WHERE workflow_step_id=$1 AND bid_status='active' ORDER BY promised_duration_minutes,submitted_at,actor_id FOR UPDATE LIMIT 1`, stepID).Scan(&bidID, &winnerID, &promisedMinutes)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "no active bids are available")
		return
	}
	if err != nil {
		s.internalError(w, "select shortest workflow step bid", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_step_bids SET bid_status=CASE WHEN id=$2 THEN 'won' ELSE 'lost' END,updated_at=now() WHERE workflow_step_id=$1 AND bid_status='active'`, stepID, bidID); err != nil {
		s.internalError(w, "close workflow step bids", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE workflow_steps SET step_status='assigned',claimed_by=$2,selected_bid_id=$3,selected_at=now() WHERE id=$1`, stepID, winnerID, bidID); err != nil {
		s.internalError(w, "assign bid winner", err)
		return
	}
	revision, err := nextRevision(r.Context(), tx, workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "advance workspace revision for bid selection", err)
		return
	}
	after, _ := json.Marshal(map[string]any{"nodeId": nodeID, "workflowStepId": stepID, "winningBidId": bidID, "winnerActorId": winnerID, "promisedDurationMinutes": promisedMinutes, "policy": "shortest_promise_v1"})
	if _, err = tx.Exec(r.Context(), `WITH event AS (INSERT INTO change_events(workspace_id,root_id,workspace_revision,correlation_id,entity_type,entity_id,event_type,after_state,actor_id,reason) VALUES($1,$2,$3,uuidv7(),'workflow_step_bid',$4,'workflow_step_bid.selected',$5,$6,'shortest_promise_v1') RETURNING correlation_id) INSERT INTO outbox_events(workspace_id,event_type,aggregate_type,aggregate_id,payload,workspace_revision,actor_id,correlation_id) SELECT $1,'workflow_step_bid.selected','workflow_step',$7,$5,$3,$6,correlation_id FROM event`, workspaceID, rootID, revision, bidID, json.RawMessage(after), actorID, stepID); err != nil {
		s.internalError(w, "record workflow step bid selection", err)
		return
	}
	var winner workflowStepBid
	if err = scanWorkflowStepBid(tx.QueryRow(r.Context(), `SELECT `+workflowStepBidColumns+` FROM workflow_step_bids b JOIN actors a ON a.id=b.actor_id LEFT JOIN actor_estimation_stats es ON es.actor_id=b.actor_id WHERE b.id=$1`, bidID), &winner); err != nil {
		s.internalError(w, "read selected workflow step bid", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workflow step bid selection", err)
		return
	}
	writeJSON(w, http.StatusOK, winner)
}
