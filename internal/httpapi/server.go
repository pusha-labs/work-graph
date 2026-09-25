package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type server struct {
	logger *slog.Logger
	db     *pgxpool.Pool
}

type workspace struct {
	ID                   string    `json:"id" db:"id"`
	Name                 string    `json:"name" db:"name"`
	Revision             int64     `json:"revision" db:"revision"`
	WorkDistributionMode string    `json:"workDistributionMode" db:"work_distribution_mode"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
}

type workNode struct {
	ID              string    `json:"id" db:"id"`
	WorkspaceID     string    `json:"workspaceId" db:"workspace_id"`
	RootID          string    `json:"rootId" db:"root_id"`
	ParentID        *string   `json:"parentId" db:"parent_id"`
	Title           string    `json:"title" db:"title"`
	DesiredOutcome  string    `json:"desiredOutcome" db:"desired_outcome"`
	LifecycleStatus string    `json:"lifecycleStatus" db:"lifecycle_status"`
	CreatedRevision int64     `json:"createdRevision" db:"created_revision"`
	UpdatedRevision int64     `json:"updatedRevision" db:"updated_revision"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time `json:"updatedAt" db:"updated_at"`
}

type changeEvent struct {
	ID                string          `json:"id" db:"id"`
	WorkspaceRevision int64           `json:"workspaceRevision" db:"workspace_revision"`
	EntityID          string          `json:"entityId" db:"entity_id"`
	EventType         string          `json:"eventType" db:"event_type"`
	BeforeState       json.RawMessage `json:"beforeState" db:"before_state"`
	AfterState        json.RawMessage `json:"afterState" db:"after_state"`
	OccurredAt        time.Time       `json:"occurredAt" db:"occurred_at"`
}

func New(logger *slog.Logger, db *pgxpool.Pool) http.Handler {
	s := &server{logger: logger, db: db}
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/workspaces", s.listWorkspaces)
	protected.HandleFunc("POST /api/v1/workspaces", s.createWorkspace)
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/distribution", s.authorizeWorkspaceAdmin(s.updateWorkspaceDistribution))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/nodes", s.authorizeWorkspaceServiceScope("work:read", s.listNodes))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes", s.authorizeWorkspaceServiceScope("work:write", s.createNode))
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/nodes/{nodeID}", s.authorizeWorkspaceServiceScope("work:write", s.updateNode))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/move", s.authorizeWorkspaceServiceScope("work:write", s.moveNode))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/nodes/{nodeID}", s.authorizeWorkspaceServiceScope("work:write", s.removeNode))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/removed-branches", s.authorizeWorkspaceServiceScope("work:read", s.listRemovedBranches))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/removed-branches/{nodeID}/restore", s.authorizeWorkspaceServiceScope("work:write", s.restoreNode))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/history", s.authorizeWorkspaceServiceScope("work:read", s.listHistory))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/revisions/{revision}/nodes", s.authorizeWorkspaceServiceScope("work:read", s.nodesAtRevision))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/placement-recommendations", s.authorizeWorkspaceServiceScope("work:read", s.recommendPlacement))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/events", s.authorizeWorkspaceServiceScope("work:read", s.listCommittedEvents))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/directory", s.authorizeWorkspace(s.getDirectory))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/actors", s.authorizeWorkspaceAdmin(s.createActor))
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/actors/{actorID}/membership", s.authorizeWorkspaceAdmin(s.updateWorkspaceMember))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/actors/{actorID}/membership", s.authorizeWorkspaceAdmin(s.removeWorkspaceMember))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/capabilities", s.authorizeWorkspaceAdmin(s.createCapability))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/actors/{actorID}/capabilities", s.authorizeWorkspaceAdmin(s.addActorCapability))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/invitations", s.authorizeWorkspaceAdmin(s.listInvitations))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/invitations", s.authorizeWorkspaceAdmin(s.createInvitation))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/circle", s.authorizeWorkspace(s.getWorkCircle))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/requirements", s.authorizeWorkspace(s.addWorkRequirement))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/knowledge-requirements", s.authorizeWorkspace(s.addWorkKnowledgeRequirement))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps", s.authorizeWorkspace(s.addWorkflowStep))
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}", s.authorizeWorkspace(s.updateWorkflowStep))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}", s.authorizeWorkspace(s.deleteWorkflowStep))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}/move", s.authorizeWorkspace(s.moveWorkflowStep))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}/bids", s.authorizeWorkspace(s.listWorkflowStepBids))
	protected.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}/bids/me", s.authorizeWorkspace(s.putMyWorkflowStepBid))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}/bids/me", s.authorizeWorkspace(s.withdrawMyWorkflowStepBid))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/workflow-steps/{stepID}/bids/select", s.authorizeWorkspace(s.selectWorkflowStepBid))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/workflow-modules", s.authorizeWorkspace(s.listWorkflowModules))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/module-installations", s.authorizeWorkspaceAdmin(s.listModuleInstallations))
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/module-installations/{moduleID}/{moduleVersion}", s.authorizeWorkspaceAdmin(s.updateModuleInstallation))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/secrets", s.authorizeWorkspace(s.listWorkspaceSecrets))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/secrets", s.authorizeWorkspaceAdmin(s.createWorkspaceSecret))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/secrets/{secretID}/rotate", s.authorizeWorkspaceAdmin(s.rotateWorkspaceSecret))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/secrets/{secretID}", s.authorizeWorkspaceAdmin(s.disableWorkspaceSecret))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/actions/{action}", s.authorizeWorkspace(s.performWorkAction))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/me", s.authorizeWorkspace(s.getMyProfile))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/my-work", s.authorizeWorkspace(s.getMyWork))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/exchange", s.authorizeWorkspace(s.listMyExchangeOffers))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/me/skills", s.authorizeWorkspace(s.addMySkill))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/me/skills/{capabilityID}", s.authorizeWorkspace(s.removeMySkill))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/me/knowledge", s.authorizeWorkspace(s.addMyKnowledge))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/me/knowledge/{subjectID}", s.authorizeWorkspace(s.removeMyKnowledge))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/knowledge-subjects", s.authorizeWorkspace(s.listKnowledgeSubjects))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/knowledge-subjects", s.authorizeWorkspaceAdmin(s.createKnowledgeSubject))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/actors/{actorID}/knowledge", s.authorizeWorkspaceAdmin(s.addActorKnowledge))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/actors/{actorID}/knowledge/{subjectID}", s.authorizeWorkspaceAdmin(s.removeActorKnowledge))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/actors/{actorID}/capabilities/{capabilityID}", s.authorizeWorkspaceAdmin(s.removeActorCapability))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/priorities", s.authorizeWorkspace(s.getPriorityInbox))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/priority-decisions", s.authorizeWorkspaceAdmin(s.recordPriorityDecision))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/criticality", s.authorizeWorkspace(s.listCriticalitySignals))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/nodes/{nodeID}/criticality", s.authorizeWorkspace(s.createCriticalitySignal))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/criticality/{signalID}", s.authorizeWorkspace(s.revokeCriticalitySignal))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/activity", s.authorizeWorkspace(s.listActivity))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/diagnostics", s.authorizeWorkspace(s.listDiagnostics))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/diagnostic-settings", s.authorizeWorkspace(s.getDiagnosticSettings))
	protected.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/diagnostic-settings", s.authorizeWorkspaceAdmin(s.updateDiagnosticSettings))
	protected.HandleFunc("GET /api/v1/workspaces/{workspaceID}/service-accounts", s.authorizeWorkspaceAdmin(s.listServiceAccounts))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/service-accounts", s.authorizeWorkspaceAdmin(s.createServiceAccount))
	protected.HandleFunc("POST /api/v1/workspaces/{workspaceID}/service-accounts/{serviceAccountID}/rotate", s.authorizeWorkspaceAdmin(s.rotateServiceAccountToken))
	protected.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/service-accounts/{serviceAccountID}", s.authorizeWorkspaceAdmin(s.revokeServiceAccount))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/ready", s.ready)
	mux.Handle("GET /api/v1/auth/status", s.withOptionalAuthentication(http.HandlerFunc(s.authStatus)))
	mux.HandleFunc("POST /api/v1/auth/register", s.registerAccount)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/auth/password-reset", s.requestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password-reset/{token}", s.resetPassword)
	mux.HandleFunc("GET /api/v1/auth/invitations/{token}", s.invitationDetails)
	mux.Handle("POST /api/v1/auth/invitations/{token}/accept", s.withOptionalAuthentication(http.HandlerFunc(s.acceptInvitation)))
	mux.HandleFunc("POST /api/v1/runner/http/lease", s.leaseHTTPExecution)
	mux.HandleFunc("GET /api/v1/runner/http/executions/{executionID}", s.getHTTPExecutionStatus)
	mux.HandleFunc("POST /api/v1/runner/http/complete", s.completeHTTPExecution)
	mux.Handle("/api/v1/", s.withAuthentication(protected))
	return withCORS(withRequestLogging(logger, mux))
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	if principal, ok := servicePrincipalFromContext(r); ok {
		var item workspace
		if err := s.db.QueryRow(r.Context(), `SELECT id,name,revision,work_distribution_mode,created_at FROM workspaces WHERE id=$1`, principal.WorkspaceID).Scan(&item.ID, &item.Name, &item.Revision, &item.WorkDistributionMode, &item.CreatedAt); err != nil {
			s.internalError(w, "read service workspace", err)
			return
		}
		writeJSON(w, http.StatusOK, []workspace{item})
		return
	}
	current, _ := accountFromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `SELECT DISTINCT w.id,w.name,w.revision,w.work_distribution_mode,w.created_at FROM workspaces w JOIN actors a ON a.workspace_id=w.id JOIN account_actors aa ON aa.actor_id=a.id WHERE aa.account_id=$1 ORDER BY w.created_at`, current.ID)
	if err != nil {
		s.internalError(w, "list workspaces", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[workspace])
	if err != nil {
		s.internalError(w, "read workspaces", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := servicePrincipalFromContext(r); ok {
		writeError(w, http.StatusForbidden, "service accounts cannot create workspaces")
		return
	}
	var input struct {
		Name      string `json:"name"`
		RootTitle string `json:"rootTitle"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.RootTitle = strings.TrimSpace(input.RootTitle)
	if input.Name == "" || input.RootTitle == "" {
		writeError(w, http.StatusBadRequest, "workspace name and root title are required")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin workspace creation", err)
		return
	}
	defer tx.Rollback(r.Context())

	var result workspace
	err = tx.QueryRow(r.Context(), `
		INSERT INTO workspaces(name, revision) VALUES ($1, 1)
		RETURNING id,name,revision,work_distribution_mode,created_at`, input.Name).Scan(
		&result.ID, &result.Name, &result.Revision, &result.WorkDistributionMode, &result.CreatedAt,
	)
	if err != nil {
		s.internalError(w, "create workspace", err)
		return
	}
	current, _ := accountFromContext(r.Context())
	var requesterID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO actors(workspace_id, display_name, has_account)
		VALUES ($1, $2, true)
		RETURNING id`, result.ID, current.DisplayName).Scan(&requesterID); err != nil {
		s.internalError(w, "create workspace owner", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO account_actors(account_id,actor_id,workspace_role) VALUES ($1,$2,'owner')`, current.ID, requesterID); err != nil {
		s.internalError(w, "link workspace owner", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO workspace_module_installations(workspace_id,module_id,module_version,enabled,publisher_trusted) SELECT $1,module_id,module_version,publisher='Work Graph',publisher='Work Graph' FROM workflow_modules`, result.ID); err != nil {
		s.internalError(w, "install workspace modules", err)
		return
	}

	var node workNode
	err = scanNode(tx.QueryRow(r.Context(), `
		WITH identity AS (SELECT uuidv7() AS id)
		INSERT INTO work_nodes(id, workspace_id, root_id, parent_id, partition_key, node_type, title, created_revision, updated_revision)
		SELECT id, $1, id, NULL, id, 'goal', $2, 1, 1 FROM identity
		RETURNING id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
		          created_revision, updated_revision, created_at, updated_at`, result.ID, input.RootTitle), &node)
	if err != nil {
		s.internalError(w, "create root goal", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO work_node_participants(work_node_id, actor_id, participant_role, sequence_number) VALUES ($1, $2, 'requester', 0)`, node.ID, requesterID); err != nil {
		s.internalError(w, "add root requester", err)
		return
	}
	if err := recordNodeChange(r.Context(), tx, node, "node.created", 1, nil, requesterID); err != nil {
		s.internalError(w, "record root goal", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit workspace creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"workspace": result, "root": node})
}

func (s *server) listNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
		       created_revision, updated_revision, created_at, updated_at
		FROM work_nodes
		WHERE workspace_id = $1 AND removed_revision IS NULL
		ORDER BY created_at`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list nodes", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[workNode])
	if err != nil {
		s.internalError(w, "read nodes", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) listHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, workspace_revision, entity_id, event_type, before_state, after_state, occurred_at
		FROM change_events
		WHERE workspace_id = $1 AND entity_type = 'work_node'
		ORDER BY workspace_revision DESC, occurred_at DESC
		LIMIT 100`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list history", err)
		return
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[changeEvent])
	if err != nil {
		s.internalError(w, "read history", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) nodesAtRevision(w http.ResponseWriter, r *http.Request) {
	revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
	if err != nil || revision < 1 {
		writeError(w, http.StatusBadRequest, "revision must be a positive integer")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	var currentRevision int64
	if err := s.db.QueryRow(r.Context(), `SELECT revision FROM workspaces WHERE id = $1`, workspaceID).Scan(&currentRevision); err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	if revision > currentRevision {
		writeError(w, http.StatusBadRequest, "revision does not exist")
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT entity_id, event_type, after_state
		FROM change_events
		WHERE workspace_id = $1 AND workspace_revision <= $2 AND entity_type = 'work_node'
		ORDER BY workspace_revision, occurred_at, id`, workspaceID, revision)
	if err != nil {
		s.internalError(w, "read revision events", err)
		return
	}
	defer rows.Close()

	snapshot := make(map[string]workNode)
	for rows.Next() {
		var entityID, eventType string
		var state json.RawMessage
		if err := rows.Scan(&entityID, &eventType, &state); err != nil {
			s.internalError(w, "scan revision event", err)
			return
		}
		if eventType == "node.removed" || len(state) == 0 || string(state) == "null" {
			delete(snapshot, entityID)
			continue
		}
		var node workNode
		if err := json.Unmarshal(state, &node); err != nil {
			s.internalError(w, "decode revision event", err)
			return
		}
		snapshot[entityID] = node
	}
	if err := rows.Err(); err != nil {
		s.internalError(w, "iterate revision events", err)
		return
	}

	nodes := make([]workNode, 0, len(snapshot))
	for _, node := range snapshot {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].CreatedAt.Before(nodes[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"revision": revision, "currentRevision": currentRevision, "nodes": nodes})
}

func (s *server) createNode(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ParentID       *string `json:"parentId"`
		Title          string  `json:"title"`
		DesiredOutcome string  `json:"desiredOutcome"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	principal, idempotencyKey, valid := serviceIdempotencyKey(w, r)
	if !valid {
		return
	}
	expectedRevision, valid := serviceExpectedRevision(w, r)
	if !valid {
		return
	}
	fingerprint, err := requestFingerprint(input)
	if err != nil {
		s.internalError(w, "fingerprint node creation", err)
		return
	}

	workspaceID := r.PathValue("workspaceID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin node creation", err)
		return
	}
	defer tx.Rollback(r.Context())
	prior, priorStatus, found, err := findServiceCommandResult(r, tx, principal, idempotencyKey, fingerprint)
	if err != nil {
		s.internalError(w, "find prior service command", err)
		return
	}
	if found {
		if priorStatus == http.StatusConflict {
			writeError(w, http.StatusConflict, "idempotency key was already used with different request content")
			return
		}
		w.Header().Set("Idempotency-Replayed", "true")
		writeJSON(w, priorStatus, prior)
		return
	}

	revision, err := nextRevisionExpected(r.Context(), tx, workspaceID, expectedRevision)
	if err != nil {
		s.writeRevisionError(w, r, "advance workspace revision", err)
		return
	}

	var node workNode
	if input.ParentID == nil {
		err = scanNode(tx.QueryRow(r.Context(), `
			WITH identity AS (SELECT uuidv7() AS id)
			INSERT INTO work_nodes(id, workspace_id, root_id, parent_id, partition_key, node_type, title, desired_outcome, created_revision, updated_revision)
			SELECT id, $1, id, NULL, id, 'goal', $2, $3, $4, $4 FROM identity
			RETURNING id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
			          created_revision, updated_revision, created_at, updated_at`, workspaceID, input.Title, input.DesiredOutcome, revision), &node)
	} else {
		err = scanNode(tx.QueryRow(r.Context(), `
			INSERT INTO work_nodes(workspace_id, root_id, parent_id, partition_key, node_type, title, desired_outcome, created_revision, updated_revision)
			SELECT p.workspace_id, p.root_id, p.id, p.partition_key, 'task', $3, $4, $5, $5
			FROM work_nodes p
			WHERE p.id = $2 AND p.workspace_id = $1 AND p.removed_revision IS NULL
			RETURNING id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
			          created_revision, updated_revision, created_at, updated_at`, workspaceID, *input.ParentID, input.Title, input.DesiredOutcome, revision), &node)
	}
	if err != nil {
		s.writeDatabaseError(w, "create node", err)
		return
	}
	requesterID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find requester", err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO work_node_participants(work_node_id, actor_id, participant_role, sequence_number) VALUES ($1, $2, 'requester', 0)`, node.ID, requesterID); err != nil {
		s.internalError(w, "add node requester", err)
		return
	}
	if err := recordNodeChange(r.Context(), tx, node, "node.created", revision, nil, requesterID); err != nil {
		s.internalError(w, "record node creation", err)
		return
	}
	if err := saveServiceCommandResult(r, tx, principal, idempotencyKey, fingerprint, http.StatusCreated, node); err != nil {
		s.internalError(w, "save service command result", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit node creation", err)
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

func (s *server) updateNode(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title           string `json:"title"`
		DesiredOutcome  string `json:"desiredOutcome"`
		LifecycleStatus string `json:"lifecycleStatus"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	allowedStatus := map[string]bool{"planned": true, "active": true, "blocked": true, "review": true, "closed": true}
	if !allowedStatus[input.LifecycleStatus] {
		writeError(w, http.StatusBadRequest, "invalid lifecycle status")
		return
	}
	expectedRevision, valid := serviceExpectedRevision(w, r)
	if !valid {
		return
	}

	workspaceID := r.PathValue("workspaceID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		s.internalError(w, "begin node update", err)
		return
	}
	defer tx.Rollback(r.Context())

	revision, err := nextRevisionExpected(r.Context(), tx, workspaceID, expectedRevision)
	if err != nil {
		s.writeRevisionError(w, r, "advance workspace revision", err)
		return
	}
	var before workNode
	if err := scanNode(tx.QueryRow(r.Context(), nodeByIDSQL+` FOR UPDATE`, workspaceID, r.PathValue("nodeID")), &before); err != nil {
		s.writeDatabaseError(w, "find node", err)
		return
	}
	if input.LifecycleStatus != before.LifecycleStatus {
		writeError(w, http.StatusConflict, "lifecycle status must be changed through a workflow action")
		return
	}
	actorID, err := s.currentActorID(r.Context(), workspaceID)
	if err != nil {
		s.writeDatabaseError(w, "find change author", err)
		return
	}

	var updated workNode
	err = scanNode(tx.QueryRow(r.Context(), `
		UPDATE work_nodes SET title = $3, desired_outcome = $4, lifecycle_status = $5,
		                      updated_revision = $6, updated_at = now()
		WHERE workspace_id = $1 AND id = $2 AND removed_revision IS NULL
		RETURNING id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
		          created_revision, updated_revision, created_at, updated_at`, workspaceID, r.PathValue("nodeID"),
		input.Title, input.DesiredOutcome, input.LifecycleStatus, revision), &updated)
	if err != nil {
		s.writeDatabaseError(w, "update node", err)
		return
	}
	if err := recordNodeChange(r.Context(), tx, updated, "node.updated", revision, before, actorID); err != nil {
		s.internalError(w, "record node update", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internalError(w, "commit node update", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

const nodeByIDSQL = `
	SELECT id, workspace_id, root_id, parent_id, title, desired_outcome, lifecycle_status,
	       created_revision, updated_revision, created_at, updated_at
	FROM work_nodes WHERE workspace_id = $1 AND id = $2 AND removed_revision IS NULL`

type rowScanner interface {
	Scan(...any) error
}

func scanNode(row rowScanner, node *workNode) error {
	return row.Scan(&node.ID, &node.WorkspaceID, &node.RootID, &node.ParentID, &node.Title, &node.DesiredOutcome,
		&node.LifecycleStatus, &node.CreatedRevision, &node.UpdatedRevision, &node.CreatedAt, &node.UpdatedAt)
}

func nextRevision(ctx context.Context, tx pgx.Tx, workspaceID string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `UPDATE workspaces SET revision = revision + 1, updated_at = now() WHERE id = $1 RETURNING revision`, workspaceID).Scan(&revision)
	return revision, err
}

func recordNodeChange(ctx context.Context, tx pgx.Tx, node workNode, eventType string, revision int64, before any, actorID string) error {
	afterBytes, err := json.Marshal(node)
	if err != nil {
		return err
	}
	afterJSON := json.RawMessage(afterBytes)
	if eventType == "node.removed" {
		afterJSON = nil
	}
	var beforeJSON any
	if before != nil {
		beforeBytes, marshalErr := json.Marshal(before)
		err = marshalErr
		if err != nil {
			return err
		}
		beforeJSON = json.RawMessage(beforeBytes)
	}
	_, err = tx.Exec(ctx, `
		WITH event AS (
			INSERT INTO change_events(workspace_id, root_id, workspace_revision, correlation_id, entity_type, entity_id, event_type, before_state, after_state, actor_id)
			VALUES ($1, $2, $3, uuidv7(), 'work_node', $4, $5, $6, $7, $8)
			RETURNING correlation_id
		)
		INSERT INTO outbox_events(workspace_id, event_type, aggregate_type, aggregate_id, payload, workspace_revision, actor_id, correlation_id)
		SELECT $1, $5, 'work_node', $4, $7, $3, $8, correlation_id FROM event`, node.WorkspaceID, node.RootID, revision, node.ID, eventType, beforeJSON, afterJSON, actorID)
	return err
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (s *server) writeDatabaseError(w http.ResponseWriter, operation string, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	s.internalError(w, operation, err)
}

func (s *server) internalError(w http.ResponseWriter, operation string, err error) {
	s.logger.Error(operation, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
