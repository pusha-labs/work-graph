package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type activityItem struct {
	ID                string    `json:"id"`
	EventType         string    `json:"eventType"`
	Summary           string    `json:"summary"`
	Detail            string    `json:"detail"`
	ActorName         string    `json:"actorName"`
	OccurredAt        time.Time `json:"occurredAt"`
	WorkspaceRevision *int64    `json:"workspaceRevision"`
	NodeID            *string   `json:"nodeId"`
	NodeTitle         string    `json:"nodeTitle"`
	CanPreview        bool      `json:"canPreview"`
}

func (s *server) listActivity(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	items := []activityItem{}
	rows, err := s.db.Query(r.Context(), `
		WITH event_rows AS (
			SELECT ce.*,
			       COALESCE(NULLIF(ce.after_state->>'nodeId',''),
			                CASE WHEN ce.entity_type='work_node' THEN ce.entity_id::text END,
			                (SELECT ws.work_node_id::text FROM workflow_step_bids b JOIN workflow_steps ws ON ws.id=b.workflow_step_id WHERE b.id=ce.entity_id)) AS resolved_node_id
			FROM change_events ce WHERE ce.workspace_id=$1
		)
		SELECT ce.id::text,ce.event_type,ce.entity_type,ce.entity_id::text,ce.workspace_revision,ce.occurred_at,
		       COALESCE(a.display_name,'System'),
		       COALESCE(ce.after_state->>'title',ce.after_state->>'name',ce.after_state->>'direction',ce.after_state->'capability'->>'name',ce.after_state->'knowledgeSubject'->>'name',CASE WHEN ce.event_type='workflow.returned' THEN (ce.after_state->'workflowStep'->>'name') || ' · ' || (ce.after_state->>'reason') ELSE ce.after_state->'workflowStep'->>'name' END,''),
		       ce.resolved_node_id,COALESCE(n.title,''),ce.after_state
		FROM event_rows ce LEFT JOIN actors a ON a.id=ce.actor_id
		LEFT JOIN work_nodes n ON n.id::text=ce.resolved_node_id
		ORDER BY ce.occurred_at DESC LIMIT 200`, workspaceID)
	if err != nil {
		s.internalError(w, "list change activity", err)
		return
	}
	for rows.Next() {
		var item activityItem
		var entityType, entityID string
		var revision int64
		var afterState json.RawMessage
		if err := rows.Scan(&item.ID, &item.EventType, &entityType, &entityID, &revision, &item.OccurredAt, &item.ActorName, &item.Detail, &item.NodeID, &item.NodeTitle, &afterState); err != nil {
			rows.Close()
			s.internalError(w, "read change activity", err)
			return
		}
		item.WorkspaceRevision = &revision
		item.Detail = describeActivity(item.EventType, afterState, item.Detail)
		switch item.EventType {
		case "node.created":
			item.Summary = "Work created"
		case "node.updated":
			item.Summary = "Work updated"
		case "node.moved":
			item.Summary = "Branch moved"
		case "node.removed":
			item.Summary = "Branch removed"
		case "node.restored":
			item.Summary = "Branch restored"
		case "requirement.added":
			item.Summary = "Role or skill requirement added"
		case "knowledge_requirement.added":
			item.Summary = "Entity knowledge requirement added"
		case "workflow_step.added":
			item.Summary = "Workflow step added"
		case "workflow_step.updated":
			item.Summary = "Workflow step updated"
		case "workflow_step.moved":
			item.Summary = "Workflow step moved"
		case "workflow_step.deleted":
			item.Summary = "Workflow step deleted"
		case "workflow_step_bid.submitted":
			item.Summary = "Performer bid submitted"
		case "workflow_step_bid.updated":
			item.Summary = "Performer bid updated"
		case "workflow_step_bid.withdrawn":
			item.Summary = "Performer bid withdrawn"
		case "workflow_step_bid.selected":
			item.Summary = "Performer selected from bids"
		case "workspace.distribution_changed":
			item.Summary = "Work distribution policy changed"
		case "workflow.returned":
			item.Summary = "Work returned for revision"
		default:
			item.Summary = item.EventType
		}
		if item.NodeID == nil && entityType == "work_node" {
			item.NodeID = &entityID
		}
		item.CanPreview = item.NodeID != nil
		items = append(items, item)
	}
	rows.Close()
	priorityRows, err := s.db.Query(r.Context(), `
		SELECT pd.id::text,l.title,r.title,c.title,pd.decision_basis,a.display_name,pd.created_at
		FROM priority_decisions pd JOIN work_nodes l ON l.id=pd.left_root_id JOIN work_nodes r ON r.id=pd.right_root_id
		LEFT JOIN work_nodes c ON c.id=pd.chosen_root_id JOIN accounts a ON a.id=pd.decided_by
		WHERE pd.workspace_id=$1`, workspaceID)
	if err != nil {
		s.internalError(w, "list priority activity", err)
		return
	}
	for priorityRows.Next() {
		var item activityItem
		var left, right, basis string
		var chosen *string
		if err := priorityRows.Scan(&item.ID, &left, &right, &chosen, &basis, &item.ActorName, &item.OccurredAt); err != nil {
			priorityRows.Close()
			s.internalError(w, "read priority activity", err)
			return
		}
		item.EventType = "priority.decided"
		if chosen == nil {
			item.Summary = "Priority left unresolved"
			item.Detail = left + " vs " + right
		} else {
			item.Summary = "Priority decision recorded"
			item.Detail = *chosen + " first · based on " + basis
		}
		items = append(items, item)
	}
	priorityRows.Close()
	criticalRows, err := s.db.Query(r.Context(), `
		SELECT cs.id::text,n.title,cs.reason,cs.critical_until,creator.display_name,cs.created_at,cs.revoked_at,COALESCE(revoker.display_name,'')
		FROM criticality_signals cs JOIN work_nodes n ON n.id=cs.work_node_id JOIN accounts creator ON creator.id=cs.created_by
		LEFT JOIN accounts revoker ON revoker.id=cs.revoked_by WHERE cs.workspace_id=$1`, workspaceID)
	if err != nil {
		s.internalError(w, "list criticality activity", err)
		return
	}
	for criticalRows.Next() {
		var id, title, reason, creator, revoker string
		var until, created time.Time
		var revoked *time.Time
		if err := criticalRows.Scan(&id, &title, &reason, &until, &creator, &created, &revoked, &revoker); err != nil {
			criticalRows.Close()
			s.internalError(w, "read criticality activity", err)
			return
		}
		items = append(items, activityItem{ID: id + ":created", EventType: "criticality.created", Summary: "Branch marked critical", Detail: title + " · " + reason + " · until " + until.Format(time.RFC3339), ActorName: creator, OccurredAt: created})
		if revoked != nil {
			items = append(items, activityItem{ID: id + ":revoked", EventType: "criticality.revoked", Summary: "Branch criticality revoked", Detail: title + " · " + reason, ActorName: revoker, OccurredAt: *revoked})
		}
	}
	criticalRows.Close()
	auditRows, err := s.db.Query(r.Context(), `SELECT wae.id::text,wae.event_type,wae.summary,wae.detail,a.display_name,wae.occurred_at FROM workspace_audit_events wae JOIN accounts a ON a.id=wae.account_id WHERE wae.workspace_id=$1`, workspaceID)
	if err != nil {
		s.internalError(w, "list workspace audit activity", err)
		return
	}
	for auditRows.Next() {
		var item activityItem
		if err := auditRows.Scan(&item.ID, &item.EventType, &item.Summary, &item.Detail, &item.ActorName, &item.OccurredAt); err != nil {
			auditRows.Close()
			s.internalError(w, "read workspace audit activity", err)
			return
		}
		items = append(items, item)
	}
	auditRows.Close()
	sort.Slice(items, func(i, j int) bool { return items[i].OccurredAt.After(items[j].OccurredAt) })
	if len(items) > 200 {
		items = items[:200]
	}
	writeJSON(w, http.StatusOK, items)
}

func describeActivity(eventType string, raw json.RawMessage, fallback string) string {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	text := func(item map[string]any, key string) string {
		result, _ := item[key].(string)
		return result
	}
	number := func(item map[string]any, key string) int {
		result, _ := item[key].(float64)
		return int(result)
	}
	step, _ := value["workflowStep"].(map[string]any)
	actor, _ := value["actor"].(map[string]any)
	switch eventType {
	case "workflow_step.added":
		name, kind, position := text(step, "name"), text(step, "stepType"), number(step, "position")
		if name != "" && position > 0 {
			return fmt.Sprintf("%s · step %d · %s", name, position, kind)
		}
	case "workflow_step.updated":
		name, previousName := text(value, "name"), text(value, "previousName")
		changes := []string{}
		if name != "" && previousName != "" && name != previousName {
			changes = append(changes, "Name: "+previousName+" → "+name)
		}
		for _, field := range []struct{ label, before, after string }{
			{"Capability", text(value, "previousCapability"), text(value, "capability")},
			{"Knowledge", text(value, "previousKnowledge"), text(value, "knowledge")},
			{"Distribution", text(value, "previousDistributionMode"), text(value, "distributionMode")},
		} {
			if field.before != field.after {
				before, after := field.before, field.after
				if before == "" {
					before = "none"
				}
				if after == "" {
					after = "none"
				}
				changes = append(changes, field.label+": "+before+" → "+after)
			}
		}
		if changed, _ := value["configurationChanged"].(bool); changed {
			changes = append(changes, "Module settings updated")
		}
		if len(changes) > 0 {
			return strings.Join(changes, " · ")
		}
		if name != "" {
			return name + " · route definition updated"
		}
	case "workflow_step.moved":
		name, from, to := text(value, "name"), number(value, "fromPosition"), number(value, "toPosition")
		if name != "" && from > 0 && to > 0 {
			return fmt.Sprintf("%s · step %d → %d", name, from, to)
		}
		if direction := text(value, "direction"); direction != "" {
			return "Moved " + direction + " in the route"
		}
	case "workflow_step.deleted":
		if name, position := text(value, "name"), number(value, "position"); name != "" && position > 0 {
			return fmt.Sprintf("%s · removed step %d", name, position)
		}
		return "Route step removed"
	case "workflow_step_bid.submitted", "workflow_step_bid.updated", "workflow_step_bid.withdrawn":
		name, minutes := text(actor, "displayName"), number(value, "promisedDurationMinutes")
		if minutes > 0 {
			return fmt.Sprintf("%s · %s", name, formatActivityDuration(minutes))
		}
	case "workflow_step_bid.selected":
		if minutes := number(value, "promisedDurationMinutes"); minutes > 0 {
			return "Shortest estimate selected · " + formatActivityDuration(minutes)
		}
	case "workflow.returned":
		name, reason := text(step, "name"), text(value, "reason")
		if name != "" && reason != "" {
			return name + " · " + reason
		}
	}
	return fallback
}

func formatActivityDuration(minutes int) string {
	if minutes%1440 == 0 {
		days := minutes / 1440
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	if minutes%60 == 0 {
		hours := minutes / 60
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d minutes", minutes)
}
