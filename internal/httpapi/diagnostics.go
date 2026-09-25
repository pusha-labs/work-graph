package httpapi

import (
	"fmt"
	"net/http"
)

type diagnostic struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Severity       string   `json:"severity"`
	Title          string   `json:"title"`
	Explanation    string   `json:"explanation"`
	NodeID         string   `json:"nodeId"`
	NodeTitle      string   `json:"nodeTitle"`
	SubjectID      string   `json:"subjectId"`
	SubjectName    string   `json:"subjectName"`
	Evidence       []string `json:"evidence"`
	RelatedNodeIDs []string `json:"relatedNodeIds"`
}

func (s *server) listDiagnostics(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	items := []diagnostic{}

	requesterRows, err := s.db.Query(r.Context(), `
		SELECT n.id,n.title FROM work_nodes n
		WHERE n.workspace_id=$1 AND n.removed_revision IS NULL AND n.lifecycle_status<>'closed'
		  AND NOT EXISTS (SELECT 1 FROM work_node_participants p WHERE p.work_node_id=n.id AND p.participant_role='requester')
		ORDER BY n.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find work without requesters", err)
		return
	}
	for requesterRows.Next() {
		var nodeID, nodeTitle string
		if err := requesterRows.Scan(&nodeID, &nodeTitle); err != nil {
			requesterRows.Close()
			s.internalError(w, "read requester diagnostic", err)
			return
		}
		items = append(items, diagnostic{
			ID: "missing-requester:" + nodeID, Kind: "missing_requester", Severity: "error",
			Title: "Requester is missing", Explanation: "This task has no person responsible for accepting its result, so its task circle cannot complete.",
			NodeID: nodeID, NodeTitle: nodeTitle, Evidence: []string{"No requester participant is recorded for this task."}, RelatedNodeIDs: []string{},
		})
	}
	if err := requesterRows.Err(); err != nil {
		requesterRows.Close()
		s.internalError(w, "read requester diagnostics", err)
		return
	}
	requesterRows.Close()

	outcomeRows, err := s.db.Query(r.Context(), `
		SELECT id,title FROM work_nodes
		WHERE workspace_id=$1 AND parent_id IS NULL AND removed_revision IS NULL AND lifecycle_status<>'closed'
		  AND btrim(desired_outcome)=''
		ORDER BY created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find goals without outcomes", err)
		return
	}
	for outcomeRows.Next() {
		var nodeID, nodeTitle string
		if err := outcomeRows.Scan(&nodeID, &nodeTitle); err != nil {
			outcomeRows.Close()
			s.internalError(w, "read outcome diagnostic", err)
			return
		}
		items = append(items, diagnostic{
			ID: "missing-outcome:" + nodeID, Kind: "missing_outcome", Severity: "warning",
			Title: "Goal outcome is not described", Explanation: "This root goal names the work, but does not yet explain what successful completion should produce.",
			NodeID: nodeID, NodeTitle: nodeTitle, Evidence: []string{"Desired outcome is empty on this root goal."}, RelatedNodeIDs: []string{},
		})
	}
	if err := outcomeRows.Err(); err != nil {
		outcomeRows.Close()
		s.internalError(w, "read outcome diagnostics", err)
		return
	}
	outcomeRows.Close()

	wideRows, err := s.db.Query(r.Context(), `
		SELECT parent.id,parent.title,count(child.id),array_agg(child.id::text ORDER BY child.created_at),array_agg(child.title ORDER BY child.created_at)
		FROM work_nodes parent JOIN work_nodes child ON child.parent_id=parent.id
		WHERE parent.workspace_id=$1 AND parent.removed_revision IS NULL AND parent.lifecycle_status<>'closed'
		  AND child.removed_revision IS NULL AND child.lifecycle_status<>'closed'
		GROUP BY parent.id,parent.title,parent.created_at HAVING count(child.id)>=8
		ORDER BY parent.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find wide branches", err)
		return
	}
	for wideRows.Next() {
		var nodeID, nodeTitle string
		var childCount int
		var relatedIDs, titles []string
		if err := wideRows.Scan(&nodeID, &nodeTitle, &childCount, &relatedIDs, &titles); err != nil {
			wideRows.Close()
			s.internalError(w, "read wide branch diagnostic", err)
			return
		}
		items = append(items, diagnostic{
			ID: "wide-branch:" + nodeID, Kind: "wide_branch", Severity: "warning",
			Title: "Branch is becoming hard to scan", Explanation: "Many open tasks sit directly under this node. Grouping related work into meaningful phases may make the execution path easier to understand.",
			NodeID: nodeID, NodeTitle: nodeTitle,
			Evidence: []string{fmt.Sprintf("%d direct open child tasks: %s", childCount, joinDiagnosticNames(titles))}, RelatedNodeIDs: relatedIDs,
		})
	}
	if err := wideRows.Err(); err != nil {
		wideRows.Close()
		s.internalError(w, "read wide branch diagnostics", err)
		return
	}
	wideRows.Close()

	matchRows, err := s.db.Query(r.Context(), `
		WITH current_steps AS (
			SELECT DISTINCT ON (s.work_node_id) s.id,s.work_node_id,s.name,s.step_type,s.step_status
			FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id
			WHERE n.workspace_id=$1 AND n.removed_revision IS NULL AND n.lifecycle_status<>'closed' AND s.step_status<>'completed'
			ORDER BY s.work_node_id,s.position
		)
		SELECT n.id,n.title,s.name,
		       COALESCE((SELECT array_agg(c.name ORDER BY c.name) FROM workflow_step_capabilities r JOIN capabilities c ON c.id=r.capability_id WHERE r.workflow_step_id=s.id),'{}'::text[]),
		       COALESCE((SELECT array_agg(k.name ORDER BY k.name) FROM workflow_step_knowledge r JOIN knowledge_subjects k ON k.id=r.subject_id WHERE r.workflow_step_id=s.id),'{}'::text[])
		FROM current_steps s JOIN work_nodes n ON n.id=s.work_node_id
		WHERE s.step_type='human' AND s.step_status='ready'
		  AND (EXISTS(SELECT 1 FROM workflow_step_capabilities WHERE workflow_step_id=s.id) OR EXISTS(SELECT 1 FROM workflow_step_knowledge WHERE workflow_step_id=s.id))
		  AND NOT EXISTS (
			SELECT 1 FROM actors a WHERE a.workspace_id=$1
			  AND NOT EXISTS(SELECT 1 FROM workflow_step_capabilities r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=a.id AND ac.capability_id=r.capability_id))
			  AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge r WHERE r.workflow_step_id=s.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=a.id AND ak.subject_id=r.subject_id))
		  )
		ORDER BY n.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find work without eligible performers", err)
		return
	}
	for matchRows.Next() {
		var nodeID, nodeTitle, stepName string
		var capabilities, subjects []string
		if err := matchRows.Scan(&nodeID, &nodeTitle, &stepName, &capabilities, &subjects); err != nil {
			matchRows.Close()
			s.internalError(w, "read performer diagnostic", err)
			return
		}
		evidence := []string{fmt.Sprintf("Ready step: %s", stepName)}
		if len(capabilities) > 0 {
			evidence = append(evidence, "Required roles or skills: "+joinDiagnosticNames(capabilities))
		}
		if len(subjects) > 0 {
			evidence = append(evidence, "Required entity knowledge: "+joinDiagnosticNames(subjects))
		}
		items = append(items, diagnostic{
			ID: "no-eligible-performer:" + nodeID, Kind: "no_eligible_performer", Severity: "warning",
			Title: "No eligible performer", Explanation: "No actor currently satisfies every requirement of the next human step.",
			NodeID: nodeID, NodeTitle: nodeTitle, Evidence: evidence, RelatedNodeIDs: []string{},
		})
	}
	if err := matchRows.Err(); err != nil {
		matchRows.Close()
		s.internalError(w, "read performer diagnostics", err)
		return
	}
	matchRows.Close()

	blockerRows, err := s.db.Query(r.Context(), `
		SELECT n.id,n.title,array_agg(d.id::text ORDER BY d.depth,d.title),array_agg(d.title ORDER BY d.depth,d.title)
		FROM work_nodes n
		JOIN LATERAL (
			WITH RECURSIVE descendants AS (
				SELECT child.id,child.title,child.lifecycle_status,1 AS depth
				FROM work_nodes child WHERE child.workspace_id=$1 AND child.parent_id=n.id AND child.removed_revision IS NULL
				UNION ALL
				SELECT child.id,child.title,child.lifecycle_status,d.depth+1
				FROM work_nodes child JOIN descendants d ON child.parent_id=d.id
				WHERE child.workspace_id=$1 AND child.removed_revision IS NULL
			)
			SELECT id,title,depth FROM descendants WHERE lifecycle_status<>'closed'
		) d ON true
		WHERE n.workspace_id=$1 AND n.removed_revision IS NULL AND n.lifecycle_status='review'
		GROUP BY n.id,n.title ORDER BY n.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find descendant blockers", err)
		return
	}
	for blockerRows.Next() {
		var nodeID, nodeTitle string
		var relatedIDs, titles []string
		if err := blockerRows.Scan(&nodeID, &nodeTitle, &relatedIDs, &titles); err != nil {
			blockerRows.Close()
			s.internalError(w, "read descendant blocker diagnostic", err)
			return
		}
		items = append(items, diagnostic{
			ID: "open-descendants:" + nodeID, Kind: "open_descendants", Severity: "warning",
			Title: "Open child work blocks acceptance", Explanation: "This task is ready for requester review, but it cannot be closed until every task below it is closed.",
			NodeID: nodeID, NodeTitle: nodeTitle,
			Evidence: []string{fmt.Sprintf("%d open descendant task(s): %s", len(titles), joinDiagnosticNames(titles))}, RelatedNodeIDs: relatedIDs,
		})
	}
	if err := blockerRows.Err(); err != nil {
		blockerRows.Close()
		s.internalError(w, "read descendant blocker diagnostics", err)
		return
	}
	blockerRows.Close()

	knowledgeRows, err := s.db.Query(r.Context(), `
		SELECT k.id,k.name,k.subject_type,
		       COALESCE((SELECT array_agg(DISTINCT a.display_name ORDER BY a.display_name) FROM actor_knowledge ak JOIN actors a ON a.id=ak.actor_id WHERE ak.subject_id=k.id),'{}'::text[]),
		       COALESCE((SELECT array_agg(DISTINCT n.title ORDER BY n.title) FROM work_node_knowledge_requirements r JOIN work_nodes n ON n.id=r.work_node_id WHERE r.subject_id=k.id AND n.removed_revision IS NULL AND n.lifecycle_status<>'closed'),'{}'::text[])
		FROM knowledge_subjects k WHERE k.workspace_id=$1 ORDER BY k.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find knowledge risks", err)
		return
	}
	for knowledgeRows.Next() {
		var subjectID, subjectName, subjectType string
		var holders, affectedWork []string
		if err := knowledgeRows.Scan(&subjectID, &subjectName, &subjectType, &holders, &affectedWork); err != nil {
			knowledgeRows.Close()
			s.internalError(w, "read knowledge diagnostic", err)
			return
		}
		kind, severity, title, explanation, report := classifyKnowledgeRisk(len(holders))
		if !report {
			continue
		}
		evidence := []string{}
		if len(holders) == 1 {
			evidence = append(evidence, "Only known holder: "+holders[0])
		} else {
			evidence = append(evidence, "No knowledgeable actor is recorded.")
		}
		if len(affectedWork) > 0 {
			evidence = append(evidence, fmt.Sprintf("Required by %d open task(s): %s", len(affectedWork), joinDiagnosticNames(affectedWork)))
		} else {
			evidence = append(evidence, "No open task currently requires this subject; the risk is visible for planning and continuity.")
		}
		items = append(items, diagnostic{
			ID: kind + ":" + subjectID, Kind: kind, Severity: severity, Title: title, Explanation: explanation,
			SubjectID: subjectID, SubjectName: subjectName, Evidence: evidence, RelatedNodeIDs: []string{},
		})
	}
	if err := knowledgeRows.Err(); err != nil {
		knowledgeRows.Close()
		s.internalError(w, "read knowledge diagnostics", err)
		return
	}
	knowledgeRows.Close()

	writeJSON(w, http.StatusOK, items)
}

func classifyKnowledgeRisk(holderCount int) (kind, severity, title, explanation string, report bool) {
	if holderCount == 0 {
		return "uncovered_knowledge", "error", "Knowledge is uncovered", "No one is currently recorded as knowing this subject. Work that depends on it has no known source of expertise.", true
	}
	if holderCount == 1 {
		return "concentrated_knowledge", "warning", "Knowledge is concentrated", "Only one person is recorded as knowing this subject. Their unavailability creates a continuity risk for the project, not a negative judgment about that person.", true
	}
	return "", "", "", "", false
}

func joinDiagnosticNames(values []string) string {
	const visibleLimit = 4
	if len(values) <= visibleLimit {
		return joinWithComma(values)
	}
	return fmt.Sprintf("%s, and %d more", joinWithComma(values[:visibleLimit]), len(values)-visibleLimit)
}

func joinWithComma(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += ", "
		}
		result += value
	}
	return result
}
