package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type actor struct {
	ID            string       `json:"id" db:"id"`
	DisplayName   string       `json:"displayName" db:"display_name"`
	ActorType     string       `json:"actorType" db:"actor_type"`
	HasAccount    bool         `json:"hasAccount" db:"has_account"`
	WorkspaceRole *string      `json:"workspaceRole,omitempty" db:"workspace_role"`
	IsCurrent     bool         `json:"isCurrent" db:"is_current"`
	CreatedAt     time.Time    `json:"createdAt" db:"created_at"`
	Capabilities  []capability `json:"capabilities" db:"-"`
}

type capability struct {
	ID             string    `json:"id" db:"id"`
	Name           string    `json:"name" db:"name"`
	CapabilityType string    `json:"capabilityType" db:"capability_type"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	Sources        []string  `json:"sources,omitempty" db:"-"`
}

func (s *server) getDirectory(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	current, _ := accountFromContext(r.Context())
	var currentWorkspaceRole string
	if err := s.db.QueryRow(r.Context(), `SELECT aa.workspace_role FROM account_actors aa JOIN actors a ON a.id=aa.actor_id WHERE aa.account_id=$1 AND a.workspace_id=$2`, current.ID, workspaceID).Scan(&currentWorkspaceRole); err != nil {
		s.writeDatabaseError(w, "find workspace membership", err)
		return
	}
	actorRows, err := s.db.Query(r.Context(), `
		SELECT actor.id,actor.display_name,actor.actor_type,actor.has_account,membership.workspace_role,
		       COALESCE(membership.account_id=$2,false) AS is_current,actor.created_at
		FROM actors actor
		LEFT JOIN account_actors membership ON membership.actor_id=actor.id
		WHERE actor.workspace_id=$1 ORDER BY actor.display_name`, workspaceID, current.ID)
	if err != nil {
		s.internalError(w, "list actors", err)
		return
	}
	actors, err := pgx.CollectRows(actorRows, pgx.RowToStructByName[actor])
	actorRows.Close()
	if err != nil {
		s.internalError(w, "read actors", err)
		return
	}

	capabilityRows, err := s.db.Query(r.Context(), `SELECT id, name, capability_type, created_at FROM capabilities WHERE workspace_id = $1 ORDER BY capability_type, name`, workspaceID)
	if err != nil {
		s.internalError(w, "list capabilities", err)
		return
	}
	capabilities, err := pgx.CollectRows(capabilityRows, pgx.RowToStructByName[capability])
	capabilityRows.Close()
	if err != nil {
		s.internalError(w, "read capabilities", err)
		return
	}

	byActor := make(map[string][]capability)
	claimRows, err := s.db.Query(r.Context(), `
		SELECT ac.actor_id, c.id, c.name, c.capability_type, c.created_at,array_agg(DISTINCT ac.claim_source ORDER BY ac.claim_source)
		FROM actor_capabilities ac
		JOIN actors a ON a.id = ac.actor_id
		JOIN capabilities c ON c.id = ac.capability_id
		WHERE a.workspace_id = $1
		GROUP BY ac.actor_id,c.id,c.name,c.capability_type,c.created_at ORDER BY c.name`, workspaceID)
	if err != nil {
		s.internalError(w, "list actor capabilities", err)
		return
	}
	defer claimRows.Close()
	for claimRows.Next() {
		var actorID string
		var item capability
		if err := claimRows.Scan(&actorID, &item.ID, &item.Name, &item.CapabilityType, &item.CreatedAt, &item.Sources); err != nil {
			s.internalError(w, "read actor capability", err)
			return
		}
		byActor[actorID] = append(byActor[actorID], item)
	}
	for index := range actors {
		actors[index].Capabilities = byActor[actors[index].ID]
		if actors[index].Capabilities == nil {
			actors[index].Capabilities = []capability{}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"actors": actors, "capabilities": capabilities, "currentWorkspaceRole": currentWorkspaceRole})
}

func (s *server) updateWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		WorkspaceRole string `json:"workspaceRole"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.WorkspaceRole != "member" && input.WorkspaceRole != "admin" {
		writeError(w, http.StatusBadRequest, "workspace role must be member or admin")
		return
	}
	current, _ := accountFromContext(r.Context())
	var currentRole, targetRole, targetName string
	err := s.db.QueryRow(r.Context(), `
		SELECT current_membership.workspace_role,target_membership.workspace_role,target_actor.display_name
		FROM account_actors current_membership
		JOIN actors current_actor ON current_actor.id=current_membership.actor_id
		JOIN actors target_actor ON target_actor.workspace_id=current_actor.workspace_id AND target_actor.id=$3
		JOIN account_actors target_membership ON target_membership.actor_id=target_actor.id
		WHERE current_membership.account_id=$1 AND current_actor.workspace_id=$2`, current.ID, r.PathValue("workspaceID"), r.PathValue("actorID")).Scan(&currentRole, &targetRole, &targetName)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workspace member not found")
		return
	}
	if err != nil {
		s.internalError(w, "find workspace member", err)
		return
	}
	if targetRole == "owner" || (targetRole == "admin" && currentRole != "owner") || (input.WorkspaceRole == "admin" && currentRole != "owner") {
		writeError(w, http.StatusForbidden, "only an owner can manage administrators; owner access cannot be changed here")
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE account_actors SET workspace_role=$3 WHERE actor_id=$1 AND EXISTS(SELECT 1 FROM actors WHERE id=$1 AND workspace_id=$2)`, r.PathValue("actorID"), r.PathValue("workspaceID"), input.WorkspaceRole); err != nil {
		s.internalError(w, "update workspace member", err)
		return
	}
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "membership.role_changed", "Workspace role changed", targetName+" · "+targetRole+" → "+input.WorkspaceRole); err != nil {
		s.internalError(w, "audit workspace role change", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	current, _ := accountFromContext(r.Context())
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workspace member removal", err)
		return
	}
	defer tx.Rollback(r.Context())
	var currentRole, targetRole, targetName, targetAccountID string
	err = tx.QueryRow(r.Context(), `
		SELECT current_membership.workspace_role,target_membership.workspace_role,target_actor.display_name,target_membership.account_id
		FROM account_actors current_membership
		JOIN actors current_actor ON current_actor.id=current_membership.actor_id
		JOIN actors target_actor ON target_actor.workspace_id=current_actor.workspace_id AND target_actor.id=$3
		JOIN account_actors target_membership ON target_membership.actor_id=target_actor.id
		WHERE current_membership.account_id=$1 AND current_actor.workspace_id=$2
		FOR UPDATE OF target_membership,target_actor`, current.ID, r.PathValue("workspaceID"), r.PathValue("actorID")).Scan(&currentRole, &targetRole, &targetName, &targetAccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workspace member not found")
		return
	}
	if err != nil {
		s.internalError(w, "find workspace member", err)
		return
	}
	if targetAccountID == current.ID {
		writeError(w, http.StatusConflict, "you cannot remove your own access from this screen")
		return
	}
	if targetRole == "owner" || (targetRole == "admin" && currentRole != "owner") {
		writeError(w, http.StatusForbidden, "only an owner can remove administrators; owners cannot be removed")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM account_actors WHERE actor_id=$1`, r.PathValue("actorID")); err != nil {
		s.internalError(w, "remove workspace membership", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE actors SET has_account=false WHERE id=$1 AND workspace_id=$2`, r.PathValue("actorID"), r.PathValue("workspaceID")); err != nil {
		s.internalError(w, "retain former workspace actor", err)
		return
	}
	if err := recordAudit(r.Context(), tx, r.PathValue("workspaceID"), current.ID, "membership.removed", "Workspace access removed", targetName+" · "+targetRole); err != nil {
		s.internalError(w, "audit workspace member removal", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workspace member removal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeActorCapability(w http.ResponseWriter, r *http.Request) {
	command, err := s.db.Exec(r.Context(), `DELETE FROM actor_capabilities ac USING actors a,capabilities c WHERE ac.actor_id=a.id AND ac.capability_id=c.id AND a.workspace_id=$1 AND c.workspace_id=$1 AND ac.actor_id=$2 AND ac.capability_id=$3 AND ac.claim_source='admin'`, r.PathValue("workspaceID"), r.PathValue("actorID"), r.PathValue("capabilityID"))
	if err != nil {
		s.internalError(w, "remove administrative capability", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "administrative capability claim not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	if err = recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "capability.unassigned", "Administrative capability removed", "actor "+r.PathValue("actorID")+" · capability "+r.PathValue("capabilityID")); err != nil {
		s.internalError(w, "audit capability removal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) createActor(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
		ActorType   string `json:"actorType"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display name is required")
		return
	}
	if input.ActorType == "" {
		input.ActorType = "person"
	}
	if input.ActorType != "person" && input.ActorType != "automation" {
		writeError(w, http.StatusBadRequest, "invalid actor type")
		return
	}
	var result actor
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO actors(workspace_id, display_name, actor_type)
		VALUES ($1, $2, $3)
		RETURNING id, display_name, actor_type, has_account, created_at`, r.PathValue("workspaceID"), input.DisplayName, input.ActorType).Scan(
		&result.ID, &result.DisplayName, &result.ActorType, &result.HasAccount, &result.CreatedAt,
	)
	if err != nil {
		s.internalError(w, "create actor", err)
		return
	}
	result.Capabilities = []capability{}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "actor.created", "Actor added", result.DisplayName); err != nil {
		s.internalError(w, "audit actor creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) createCapability(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name           string `json:"name"`
		CapabilityType string `json:"capabilityType"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "capability name is required")
		return
	}
	if input.CapabilityType != "role" && input.CapabilityType != "skill" {
		writeError(w, http.StatusBadRequest, "invalid capability type")
		return
	}
	var result capability
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO capabilities(workspace_id, name, capability_type)
		VALUES ($1, $2, $3)
		RETURNING id, name, capability_type, created_at`, r.PathValue("workspaceID"), input.Name, input.CapabilityType).Scan(
		&result.ID, &result.Name, &result.CapabilityType, &result.CreatedAt,
	)
	if err != nil {
		s.internalError(w, "create capability", err)
		return
	}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "capability.created", "Capability added", result.Name+" · "+result.CapabilityType); err != nil {
		s.internalError(w, "audit capability creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) addActorCapability(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CapabilityID string `json:"capabilityId"`
		ClaimSource  string `json:"claimSource"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.ClaimSource == "" {
		input.ClaimSource = "admin"
	}
	allowedSources := map[string]bool{"self": true, "admin": true, "inferred": true, "imported": true}
	if !allowedSources[input.ClaimSource] {
		writeError(w, http.StatusBadRequest, "invalid claim source")
		return
	}
	command, err := s.db.Exec(r.Context(), `
		INSERT INTO actor_capabilities(actor_id, capability_id, claim_source)
		SELECT a.id, c.id, $4
		FROM actors a JOIN capabilities c ON c.workspace_id = a.workspace_id
		WHERE a.workspace_id = $1 AND a.id = $2 AND c.id = $3
		ON CONFLICT DO NOTHING`, r.PathValue("workspaceID"), r.PathValue("actorID"), input.CapabilityID, input.ClaimSource)
	if err != nil {
		s.internalError(w, "add actor capability", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "actor or capability not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "capability.assigned", "Capability assigned", "actor "+r.PathValue("actorID")+" · capability "+input.CapabilityID+" · "+input.ClaimSource); err != nil {
		s.internalError(w, "audit capability assignment", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
