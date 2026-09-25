package httpapi

import (
	"net/http"
	"time"
)

type myWorkItem struct {
	NodeID        string     `json:"nodeId"`
	NodeTitle     string     `json:"nodeTitle"`
	RootID        string     `json:"rootId"`
	RootTitle     string     `json:"rootTitle"`
	StepID        string     `json:"stepId"`
	StepName      string     `json:"stepName"`
	StepPosition  int        `json:"stepPosition"`
	StepStatus    string     `json:"stepStatus"`
	Requester     string     `json:"requester"`
	CriticalUntil *time.Time `json:"criticalUntil"`
}

func (s *server) getMyWork(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find current actor", err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT n.id,n.title,root.id,root.title,step.id,step.name,step.position,CASE WHEN n.lifecycle_status='review' THEN 'review' ELSE step.step_status END,requester.display_name,
		       (SELECT MAX(cs.critical_until) FROM criticality_signals cs
		        WHERE cs.workspace_id=n.workspace_id AND cs.revoked_at IS NULL AND cs.critical_until>now()
		          AND cs.work_node_id IN (
		            WITH RECURSIVE branch AS (
		              SELECT id FROM work_nodes WHERE id=n.id
		              UNION ALL SELECT child.id FROM work_nodes child JOIN branch parent ON child.parent_id=parent.id WHERE child.removed_revision IS NULL
		            ) SELECT id FROM branch))
		FROM workflow_steps step
		JOIN work_nodes n ON n.id=step.work_node_id
		JOIN workspaces w ON w.id=n.workspace_id
		JOIN work_nodes root ON root.id=n.root_id
		JOIN work_node_participants rp ON rp.work_node_id=n.id AND rp.participant_role='requester'
		JOIN actors requester ON requester.id=rp.actor_id
		WHERE n.workspace_id=$1 AND n.removed_revision IS NULL AND (
		  (step.step_status IN ('assigned','active') AND step.claimed_by=$2)
		  OR (step.step_status='ready'
		    AND CASE step.distribution_mode WHEN 'inherit' THEN w.work_distribution_mode ELSE step.distribution_mode END='simple'
		    AND (EXISTS(SELECT 1 FROM workflow_step_capabilities WHERE workflow_step_id=step.id) OR EXISTS(SELECT 1 FROM workflow_step_knowledge WHERE workflow_step_id=step.id))
		    AND NOT EXISTS(SELECT 1 FROM workflow_step_capabilities req WHERE req.workflow_step_id=step.id AND NOT EXISTS(SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=$2 AND ac.capability_id=req.capability_id))
		    AND NOT EXISTS(SELECT 1 FROM workflow_step_knowledge req WHERE req.workflow_step_id=step.id AND NOT EXISTS(SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=$2 AND ak.subject_id=req.subject_id))
		  ) OR (n.lifecycle_status='review' AND rp.actor_id=$2 AND step.position=(SELECT MAX(last.position) FROM workflow_steps last WHERE last.work_node_id=n.id))
		)
		ORDER BY (step.step_status='active') DESC,(n.lifecycle_status='review') DESC,n.created_at,step.position`, workspaceID, actorID)
	if err != nil {
		s.internalError(w, "list personal work", err)
		return
	}
	defer rows.Close()
	items := []myWorkItem{}
	for rows.Next() {
		var item myWorkItem
		if err := rows.Scan(&item.NodeID, &item.NodeTitle, &item.RootID, &item.RootTitle, &item.StepID, &item.StepName, &item.StepPosition, &item.StepStatus, &item.Requester, &item.CriticalUntil); err != nil {
			s.internalError(w, "read personal work", err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}
