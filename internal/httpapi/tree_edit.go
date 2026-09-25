package httpapi

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type removedBranch struct {
	ID              string    `json:"id" db:"id"`
	Title           string    `json:"title" db:"title"`
	ParentID        string    `json:"parentId" db:"parent_id"`
	RemovedRevision int64     `json:"removedRevision" db:"removed_revision"`
	RemovedAt       time.Time `json:"removedAt" db:"removed_at"`
	NodeCount       int       `json:"nodeCount" db:"node_count"`
}

func (s *server) listRemovedBranches(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT n.id,n.title,n.parent_id,n.removed_revision,n.updated_at,(SELECT count(*) FROM work_nodes d WHERE d.workspace_id=n.workspace_id AND d.removed_revision=n.removed_revision)::int FROM work_nodes n LEFT JOIN work_nodes p ON p.id=n.parent_id WHERE n.workspace_id=$1 AND n.removed_revision IS NOT NULL AND n.parent_id IS NOT NULL AND p.removed_revision IS DISTINCT FROM n.removed_revision ORDER BY n.updated_at DESC`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list removed branches", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[removedBranch])
	if err != nil {
		s.internalError(w, "read removed branches", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) restoreNode(w http.ResponseWriter, r *http.Request) {
	expectedRevision, valid := serviceExpectedRevision(w, r)
	if !valid {
		return
	}
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin branch restoration", err)
		return
	}
	defer tx.Rollback(r.Context())
	var parentID string
	var removedRevision int64
	err = tx.QueryRow(r.Context(), `SELECT n.parent_id,n.removed_revision FROM work_nodes n LEFT JOIN work_nodes p ON p.id=n.parent_id WHERE n.workspace_id=$1 AND n.id=$2 AND n.removed_revision IS NOT NULL AND p.removed_revision IS DISTINCT FROM n.removed_revision FOR UPDATE OF n`, workspaceID, nodeID).Scan(&parentID, &removedRevision)
	if err != nil {
		s.writeDatabaseError(w, "find removed branch", err)
		return
	}
	var parent workNode
	var partitionKey string
	err = tx.QueryRow(r.Context(), nodeByIDSQL+` FOR UPDATE`, workspaceID, parentID).Scan(&parent.ID, &parent.WorkspaceID, &parent.RootID, &parent.ParentID, &parent.Title, &parent.DesiredOutcome, &parent.LifecycleStatus, &parent.CreatedRevision, &parent.UpdatedRevision, &parent.CreatedAt, &parent.UpdatedAt)
	if err != nil {
		s.writeDatabaseError(w, "find original parent", err)
		return
	}
	if parent.LifecycleStatus != "planned" {
		writeError(w, http.StatusConflict, "the original parent must be planned before this branch can be restored")
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT partition_key FROM work_nodes WHERE id=$1`, parentID).Scan(&partitionKey); err != nil {
		s.internalError(w, "find parent partition", err)
		return
	}
	rows, err := tx.Query(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision=$3 UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision=$3) SELECT n.id,n.workspace_id,n.root_id,n.parent_id,n.title,n.desired_outcome,n.lifecycle_status,n.created_revision,n.updated_revision,n.created_at,n.updated_at FROM work_nodes n JOIN branch b ON b.id=n.id ORDER BY n.created_at FOR UPDATE OF n`, workspaceID, nodeID, removedRevision)
	if err != nil {
		s.internalError(w, "lock removed branch", err)
		return
	}
	beforeNodes, err := pgx.CollectRows(rows, pgx.RowToStructByName[workNode])
	if err != nil {
		s.writeDatabaseError(w, "read removed branch", err)
		return
	}
	if len(beforeNodes) == 0 {
		writeError(w, http.StatusNotFound, "removed branch not found")
		return
	}
	revision, err := nextRevisionExpected(r.Context(), tx, workspaceID, expectedRevision)
	if err != nil {
		s.writeRevisionError(w, r, "advance restoration revision", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision=$3 UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision=$3) UPDATE work_nodes n SET removed_revision=NULL,root_id=$4,partition_key=$5,updated_revision=$6,updated_at=now() FROM branch b WHERE n.id=b.id`, workspaceID, nodeID, removedRevision, parent.RootID, partitionKey, revision); err != nil {
		s.internalError(w, "restore branch", err)
		return
	}
	afterNodes, err := readSubtree(r, tx, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "read restored branch", err)
		return
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find branch restorer", err)
		return
	}
	beforeByID := map[string]workNode{}
	for _, node := range beforeNodes {
		beforeByID[node.ID] = node
	}
	for _, node := range afterNodes {
		if err := recordNodeChange(r.Context(), tx, node, "node.restored", revision, beforeByID[node.ID], actorID); err != nil {
			s.internalError(w, "record branch restoration", err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit branch restoration", err)
		return
	}
	writeJSON(w, http.StatusOK, afterNodes[0])
}

func (s *server) moveNode(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ParentID string `json:"parentId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	if input.ParentID == "" || input.ParentID == nodeID {
		writeError(w, http.StatusBadRequest, "a different parent task is required")
		return
	}
	expectedRevision, valid := serviceExpectedRevision(w, r)
	if !valid {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin branch move", err)
		return
	}
	defer tx.Rollback(r.Context())

	beforeNodes, err := lockSubtree(r, tx, workspaceID, nodeID)
	if err != nil {
		s.writeDatabaseError(w, "find branch", err)
		return
	}
	if beforeNodes[0].ParentID == nil {
		writeError(w, http.StatusConflict, "root goals cannot be moved under another task")
		return
	}
	for _, node := range beforeNodes {
		if node.LifecycleStatus != "planned" {
			writeError(w, http.StatusConflict, "only a fully planned branch can be moved")
			return
		}
		if node.ID == input.ParentID {
			writeError(w, http.StatusConflict, "a branch cannot be moved inside itself")
			return
		}
	}
	var target workNode
	var partitionKey string
	err = tx.QueryRow(r.Context(), nodeByIDSQL+` FOR UPDATE`, workspaceID, input.ParentID).Scan(&target.ID, &target.WorkspaceID, &target.RootID, &target.ParentID, &target.Title, &target.DesiredOutcome, &target.LifecycleStatus, &target.CreatedRevision, &target.UpdatedRevision, &target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		s.writeDatabaseError(w, "find destination task", err)
		return
	}
	if target.LifecycleStatus != "planned" {
		writeError(w, http.StatusConflict, "a branch can only be moved under planned work")
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT partition_key FROM work_nodes WHERE id=$1`, target.ID).Scan(&partitionKey); err != nil {
		s.internalError(w, "find destination partition", err)
		return
	}
	revision, err := nextRevisionExpected(r.Context(), tx, workspaceID, expectedRevision)
	if err != nil {
		s.writeRevisionError(w, r, "advance branch revision", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL) UPDATE work_nodes n SET parent_id=CASE WHEN n.id=$2 THEN $3 ELSE n.parent_id END,root_id=$4,partition_key=$5,updated_revision=$6,updated_at=now() FROM branch b WHERE n.id=b.id`, workspaceID, nodeID, input.ParentID, target.RootID, partitionKey, revision); err != nil {
		s.internalError(w, "move branch", err)
		return
	}
	afterNodes, err := readSubtree(r, tx, workspaceID, nodeID)
	if err != nil {
		s.internalError(w, "read moved branch", err)
		return
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find branch editor", err)
		return
	}
	beforeByID := map[string]workNode{}
	for _, node := range beforeNodes {
		beforeByID[node.ID] = node
	}
	for _, node := range afterNodes {
		if err := recordNodeChange(r.Context(), tx, node, "node.moved", revision, beforeByID[node.ID], actorID); err != nil {
			s.internalError(w, "record branch move", err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit branch move", err)
		return
	}
	writeJSON(w, http.StatusOK, afterNodes[0])
}

func (s *server) removeNode(w http.ResponseWriter, r *http.Request) {
	expectedRevision, valid := serviceExpectedRevision(w, r)
	if !valid {
		return
	}
	workspaceID, nodeID := r.PathValue("workspaceID"), r.PathValue("nodeID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin branch removal", err)
		return
	}
	defer tx.Rollback(r.Context())
	beforeNodes, err := lockSubtree(r, tx, workspaceID, nodeID)
	if err != nil {
		s.writeDatabaseError(w, "find branch", err)
		return
	}
	if beforeNodes[0].ParentID == nil {
		writeError(w, http.StatusConflict, "root goals cannot be removed")
		return
	}
	for _, node := range beforeNodes {
		if node.LifecycleStatus != "planned" {
			writeError(w, http.StatusConflict, "only a fully planned branch can be removed")
			return
		}
	}
	revision, err := nextRevisionExpected(r.Context(), tx, workspaceID, expectedRevision)
	if err != nil {
		s.writeRevisionError(w, r, "advance branch revision", err)
		return
	}
	if _, err = tx.Exec(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL) UPDATE work_nodes n SET removed_revision=$3,updated_revision=$3,updated_at=now() FROM branch b WHERE n.id=b.id`, workspaceID, nodeID, revision); err != nil {
		s.internalError(w, "remove branch", err)
		return
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find branch editor", err)
		return
	}
	for _, node := range beforeNodes {
		node.UpdatedRevision = revision
		if err := recordNodeChange(r.Context(), tx, node, "node.removed", revision, node, actorID); err != nil {
			s.internalError(w, "record branch removal", err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit branch removal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func lockSubtree(r *http.Request, tx pgx.Tx, workspaceID, nodeID string) ([]workNode, error) {
	rows, err := tx.Query(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision IS NULL UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL) SELECT n.id,n.workspace_id,n.root_id,n.parent_id,n.title,n.desired_outcome,n.lifecycle_status,n.created_revision,n.updated_revision,n.created_at,n.updated_at FROM work_nodes n JOIN branch b ON b.id=n.id ORDER BY n.created_at FOR UPDATE OF n`, workspaceID, nodeID)
	if err != nil {
		return nil, err
	}
	nodes, err := pgx.CollectRows(rows, pgx.RowToStructByName[workNode])
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, pgx.ErrNoRows
	}
	return nodes, nil
}

func readSubtree(r *http.Request, tx pgx.Tx, workspaceID, nodeID string) ([]workNode, error) {
	rows, err := tx.Query(r.Context(), `WITH RECURSIVE branch AS (SELECT id FROM work_nodes WHERE workspace_id=$1 AND id=$2 AND removed_revision IS NULL UNION ALL SELECT n.id FROM work_nodes n JOIN branch b ON n.parent_id=b.id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL) SELECT n.id,n.workspace_id,n.root_id,n.parent_id,n.title,n.desired_outcome,n.lifecycle_status,n.created_revision,n.updated_revision,n.created_at,n.updated_at FROM work_nodes n JOIN branch b ON b.id=n.id ORDER BY n.created_at`, workspaceID, nodeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[workNode])
}
