package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

type priorityCandidate struct {
	RootID    string       `json:"rootId"`
	RootTitle string       `json:"rootTitle"`
	TaskID    string       `json:"taskId"`
	TaskTitle string       `json:"taskTitle"`
	Requester actorSummary `json:"requester"`
}

type priorityConflict struct {
	Left           priorityCandidate `json:"left"`
	Right          priorityCandidate `json:"right"`
	AffectedActors []actorSummary    `json:"affectedActors"`
	ChosenRootID   *string           `json:"chosenRootId"`
	DecisionBasis  *string           `json:"decisionBasis"`
	DecidedAt      *time.Time        `json:"decidedAt"`
}

func (s *server) getPriorityInbox(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	rows, err := s.db.Query(r.Context(), `
		SELECT eligible.id,eligible.display_name,eligible.actor_type,n.root_id,root.title,n.id,n.title,requester.id,requester.display_name,requester.actor_type
		FROM actors eligible CROSS JOIN work_nodes n JOIN work_nodes root ON root.id=n.root_id
		JOIN LATERAL (SELECT id FROM workflow_steps WHERE work_node_id=n.id AND step_status='ready' ORDER BY position LIMIT 1) ws ON true
		JOIN work_node_participants rp ON rp.work_node_id=n.id AND rp.participant_role='requester'
		JOIN actors requester ON requester.id=rp.actor_id
		WHERE n.workspace_id=$1 AND eligible.workspace_id=$1 AND n.parent_id IS NOT NULL AND n.removed_revision IS NULL
		  AND n.lifecycle_status IN ('planned','active')
		  AND (EXISTS(SELECT 1 FROM workflow_step_capabilities WHERE workflow_step_id=ws.id)
		       OR EXISTS(SELECT 1 FROM workflow_step_knowledge WHERE workflow_step_id=ws.id))
		  AND NOT EXISTS (
			SELECT 1 FROM workflow_step_capabilities wr WHERE wr.workflow_step_id=ws.id
			  AND NOT EXISTS (SELECT 1 FROM actor_capabilities ac WHERE ac.actor_id=eligible.id AND ac.capability_id=wr.capability_id))
		  AND NOT EXISTS (
			SELECT 1 FROM workflow_step_knowledge kr WHERE kr.workflow_step_id=ws.id
			  AND NOT EXISTS (SELECT 1 FROM actor_knowledge ak WHERE ak.actor_id=eligible.id AND ak.subject_id=kr.subject_id))
		ORDER BY eligible.id,n.created_at`, workspaceID)
	if err != nil {
		s.internalError(w, "find priority candidates", err)
		return
	}
	defer rows.Close()
	byActor := map[string]map[string]priorityCandidate{}
	actors := map[string]actorSummary{}
	for rows.Next() {
		var affected actorSummary
		var item priorityCandidate
		if err := rows.Scan(&affected.ID, &affected.DisplayName, &affected.ActorType, &item.RootID, &item.RootTitle, &item.TaskID, &item.TaskTitle, &item.Requester.ID, &item.Requester.DisplayName, &item.Requester.ActorType); err != nil {
			s.internalError(w, "read priority candidate", err)
			return
		}
		actors[affected.ID] = affected
		if byActor[affected.ID] == nil {
			byActor[affected.ID] = map[string]priorityCandidate{}
		}
		if _, exists := byActor[affected.ID][item.RootID]; !exists {
			byActor[affected.ID][item.RootID] = item
		}
	}
	pairs := map[string]*priorityConflict{}
	for actorID, byRoot := range byActor {
		rootIDs := make([]string, 0, len(byRoot))
		for id := range byRoot {
			rootIDs = append(rootIDs, id)
		}
		sort.Strings(rootIDs)
		for i := 0; i < len(rootIDs); i++ {
			for j := i + 1; j < len(rootIDs); j++ {
				key := rootIDs[i] + ":" + rootIDs[j]
				if pairs[key] == nil {
					pairs[key] = &priorityConflict{Left: byRoot[rootIDs[i]], Right: byRoot[rootIDs[j]], AffectedActors: []actorSummary{}}
				}
				pairs[key].AffectedActors = append(pairs[key].AffectedActors, actors[actorID])
			}
		}
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	conflicts := make([]priorityConflict, 0, len(keys))
	scores := map[string]int{}
	for _, key := range keys {
		conflict := *pairs[key]
		leftID, rightID := conflict.Left.RootID, conflict.Right.RootID
		var chosen *string
		var basis *string
		var decidedAt *time.Time
		err := s.db.QueryRow(r.Context(), `SELECT chosen_root_id::text,decision_basis,created_at FROM priority_decisions WHERE workspace_id=$1 AND left_root_id=$2 AND right_root_id=$3 ORDER BY created_at DESC LIMIT 1`, workspaceID, leftID, rightID).Scan(&chosen, &basis, &decidedAt)
		if err == nil {
			conflict.ChosenRootID, conflict.DecisionBasis, conflict.DecidedAt = chosen, basis, decidedAt
			if chosen != nil {
				scores[*chosen]++
			}
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.internalError(w, "read priority decision", err)
			return
		}
		conflicts = append(conflicts, conflict)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": conflicts, "rootScores": scores})
}

func (s *server) recordPriorityDecision(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LeftRootID    string  `json:"leftRootId"`
		RightRootID   string  `json:"rightRootId"`
		ChosenRootID  *string `json:"chosenRootId"`
		DecisionBasis string  `json:"decisionBasis"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.LeftRootID == input.RightRootID || (input.DecisionBasis != "goal" && input.DecisionBasis != "requester" && input.DecisionBasis != "unknown") {
		writeError(w, http.StatusBadRequest, "invalid priority decision")
		return
	}
	if input.DecisionBasis == "unknown" {
		input.ChosenRootID = nil
	}
	if input.ChosenRootID != nil && *input.ChosenRootID != input.LeftRootID && *input.ChosenRootID != input.RightRootID {
		writeError(w, http.StatusBadRequest, "chosen root must be one of the compared goals")
		return
	}
	left, right := input.LeftRootID, input.RightRootID
	if right < left {
		left, right = right, left
	}
	var valid bool
	if err := s.db.QueryRow(r.Context(), `SELECT (SELECT count(*)=2 FROM work_nodes WHERE workspace_id=$1 AND id IN ($2,$3) AND parent_id IS NULL AND removed_revision IS NULL)`, r.PathValue("workspaceID"), left, right).Scan(&valid); err != nil {
		s.internalError(w, "validate priority roots", err)
		return
	}
	if !valid {
		writeError(w, http.StatusNotFound, "priority goals not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	var id string
	var createdAt time.Time
	err := s.db.QueryRow(r.Context(), `INSERT INTO priority_decisions(workspace_id,left_root_id,right_root_id,chosen_root_id,decision_basis,decided_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, r.PathValue("workspaceID"), left, right, input.ChosenRootID, input.DecisionBasis, current.ID).Scan(&id, &createdAt)
	if err != nil {
		s.internalError(w, "record priority decision", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "createdAt": createdAt})
}
