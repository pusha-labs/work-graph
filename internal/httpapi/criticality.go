package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type criticalitySignal struct {
	ID              string    `json:"id"`
	WorkNodeID      string    `json:"workNodeId"`
	WorkNodeTitle   string    `json:"workNodeTitle"`
	Reason          string    `json:"reason"`
	CriticalUntil   time.Time `json:"criticalUntil"`
	CreatedAt       time.Time `json:"createdAt"`
	AffectedNodeIDs []string  `json:"affectedNodeIds"`
}

func (s *server) listCriticalitySignals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		WITH RECURSIVE active AS (
			SELECT cs.id,cs.work_node_id,n.title,cs.reason,cs.critical_until,cs.created_at
			FROM criticality_signals cs JOIN work_nodes n ON n.id=cs.work_node_id
			WHERE cs.workspace_id=$1 AND cs.revoked_at IS NULL AND cs.critical_until>now()
		), ancestors(signal_id,node_id,parent_id) AS (
			SELECT a.id,n.id,n.parent_id FROM active a JOIN work_nodes n ON n.id=a.work_node_id
			UNION ALL SELECT an.signal_id,p.id,p.parent_id FROM ancestors an JOIN work_nodes p ON p.id=an.parent_id
		)
		SELECT a.id,a.work_node_id,a.title,a.reason,a.critical_until,a.created_at,array_agg(an.node_id::text ORDER BY an.node_id)
		FROM active a JOIN ancestors an ON an.signal_id=a.id
		GROUP BY a.id,a.work_node_id,a.title,a.reason,a.critical_until,a.created_at ORDER BY a.critical_until`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list criticality signals", err)
		return
	}
	defer rows.Close()
	items := []criticalitySignal{}
	for rows.Next() {
		var item criticalitySignal
		if err := rows.Scan(&item.ID, &item.WorkNodeID, &item.WorkNodeTitle, &item.Reason, &item.CriticalUntil, &item.CreatedAt, &item.AffectedNodeIDs); err != nil {
			s.internalError(w, "read criticality signal", err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createCriticalitySignal(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reason        string    `json:"reason"`
		CriticalUntil time.Time `json:"criticalUntil"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || !input.CriticalUntil.After(time.Now()) || input.CriticalUntil.After(time.Now().Add(366*24*time.Hour)) {
		writeError(w, http.StatusBadRequest, "reason and a future deadline within one year are required")
		return
	}
	current, _ := accountFromContext(r.Context())
	var allowed bool
	err := s.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM work_nodes n JOIN work_node_participants p ON p.work_node_id=n.id AND p.participant_role='requester'
			JOIN account_actors aa ON aa.actor_id=p.actor_id
			WHERE n.workspace_id=$1 AND n.id=$2 AND aa.account_id=$3
		) OR EXISTS(
			SELECT 1 FROM account_actors aa JOIN actors a ON a.id=aa.actor_id
			WHERE aa.account_id=$3 AND a.workspace_id=$1 AND aa.workspace_role IN ('owner','admin')
		)`, r.PathValue("workspaceID"), r.PathValue("nodeID"), current.ID).Scan(&allowed)
	if err != nil {
		s.internalError(w, "authorize criticality", err)
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "only the requester or an administrator can mark this branch critical")
		return
	}
	var result criticalitySignal
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO criticality_signals(workspace_id,work_node_id,reason,critical_until,created_by)
		SELECT workspace_id,id,$3,$4,$5 FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision IS NULL
		RETURNING id,work_node_id,reason,critical_until,created_at`, r.PathValue("workspaceID"), r.PathValue("nodeID"), input.Reason, input.CriticalUntil, current.ID).Scan(&result.ID, &result.WorkNodeID, &result.Reason, &result.CriticalUntil, &result.CreatedAt)
	if err != nil {
		s.writeDatabaseError(w, "create criticality signal", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) revokeCriticalitySignal(w http.ResponseWriter, r *http.Request) {
	current, _ := accountFromContext(r.Context())
	command, err := s.db.Exec(r.Context(), `
		UPDATE criticality_signals cs SET revoked_at=now(),revoked_by=$3
		WHERE cs.id=$2 AND cs.workspace_id=$1 AND cs.revoked_at IS NULL
		AND (cs.created_by=$3 OR EXISTS(
			SELECT 1 FROM account_actors aa JOIN actors a ON a.id=aa.actor_id
			WHERE aa.account_id=$3 AND a.workspace_id=$1 AND aa.workspace_role IN ('owner','admin')))
	`, r.PathValue("workspaceID"), r.PathValue("signalID"), current.ID)
	if err != nil {
		s.internalError(w, "revoke criticality signal", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "active criticality signal not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
