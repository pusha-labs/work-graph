package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const candidateColumns = `id,provider,installation_key,external_id,external_version,title,description,status,recommendations,work_graph_node_id,last_error,placement_parent_id,attempt_count,max_attempts,next_attempt_at,last_attempt_at,dead_lettered_at,created_at,updated_at`

func (h *Handler) ingestMockCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExternalID      string         `json:"externalId"`
		ExternalVersion string         `json:"externalVersion"`
		Title           string         `json:"title"`
		Description     string         `json:"description"`
		RawPayload      map[string]any `json:"rawPayload"`
	}
	if !decodeGatewayJSON(w, r, &input) {
		return
	}
	input.ExternalID, input.ExternalVersion, input.Title = strings.TrimSpace(input.ExternalID), strings.TrimSpace(input.ExternalVersion), strings.TrimSpace(input.Title)
	if input.ExternalID == "" || input.ExternalVersion == "" || input.Title == "" {
		writeGatewayError(w, http.StatusBadRequest, "externalId, externalVersion, and title are required")
		return
	}
	id, err := h.upsertCandidate(r.Context(), "mock", h.config.InstallationKey, input.ExternalID, input.ExternalVersion, input.Title, input.Description, input.RawPayload)
	if err != nil {
		h.internal(w, "save candidate", err)
		return
	}
	item, err := h.getCandidate(r.Context(), id)
	if err != nil {
		h.internal(w, "read candidate", err)
		return
	}
	writeGatewayJSON(w, http.StatusCreated, item)
}

func (h *Handler) upsertCandidate(ctx context.Context, provider, installationKey, externalID, externalVersion, title, description string, rawPayload any) (string, error) {
	var existingID, existingVersion string
	err := h.db.QueryRow(ctx, `SELECT id::text,external_version FROM gateway_candidates WHERE provider=$1 AND installation_key=$2 AND external_id=$3`, provider, installationKey, externalID).Scan(&existingID, &existingVersion)
	if err == nil && existingVersion == externalVersion {
		return existingID, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}
	recommendations, recommendationErr := h.recommend(ctx, title, description)
	status, lastError := "unplaced", ""
	if len(recommendations) > 0 {
		status = "suggested"
	}
	if recommendationErr != nil {
		h.logger.Warn("placement recommendations unavailable during ingestion", "provider", provider, "external_id", externalID, "error", recommendationErr)
		lastError = "Placement recommendations are temporarily unavailable; choose a branch manually."
	}
	recommendationJSON, _ := json.Marshal(recommendations)
	rawJSON, _ := json.Marshal(rawPayload)
	var id string
	err = h.db.QueryRow(ctx, `
		INSERT INTO gateway_candidates(provider,installation_key,external_id,external_version,title,description,raw_payload,status,recommendations,last_error)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(provider,installation_key,external_id) DO UPDATE SET
			external_version=EXCLUDED.external_version,title=EXCLUDED.title,description=EXCLUDED.description,raw_payload=EXCLUDED.raw_payload,
			status=CASE WHEN gateway_candidates.status='placed' THEN 'placed' ELSE EXCLUDED.status END,
			recommendations=CASE WHEN gateway_candidates.status='placed' THEN gateway_candidates.recommendations ELSE EXCLUDED.recommendations END,
			attempt_count=CASE WHEN gateway_candidates.status='placed' THEN gateway_candidates.attempt_count ELSE 0 END,
			next_attempt_at=NULL,dead_lettered_at=NULL,last_error=CASE WHEN gateway_candidates.status='placed' THEN gateway_candidates.last_error ELSE EXCLUDED.last_error END,updated_at=now()
		RETURNING id`, provider, installationKey, externalID, externalVersion, title, description, rawJSON, status, recommendationJSON, lastError).Scan(&id)
	return id, err
}

func (h *Handler) listCandidates(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	query := `SELECT ` + candidateColumns + ` FROM gateway_candidates`
	args := []any{}
	if status != "" {
		query += ` WHERE status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		h.internal(w, "list candidates", err)
		return
	}
	defer rows.Close()
	items := []Candidate{}
	for rows.Next() {
		item, err := scanCandidate(rows)
		if err != nil {
			h.internal(w, "scan candidate", err)
			return
		}
		items = append(items, item)
	}
	writeGatewayJSON(w, http.StatusOK, items)
}

func (h *Handler) placeCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ParentID string `json:"parentId"`
	}
	form := strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	if form {
		if err := r.ParseForm(); err != nil {
			writeGatewayError(w, http.StatusBadRequest, "invalid form")
			return
		}
		input.ParentID = r.FormValue("parentId")
	} else if !decodeGatewayJSON(w, r, &input) {
		return
	}
	item, err := h.getCandidate(r.Context(), r.PathValue("candidateID"))
	if err == pgx.ErrNoRows {
		writeGatewayError(w, http.StatusNotFound, "candidate not found")
		return
	}
	if err != nil {
		h.internal(w, "read candidate", err)
		return
	}
	if item.WorkGraphNodeID == nil {
		parentID := strings.TrimSpace(input.ParentID)
		if parentID == "" {
			writeGatewayError(w, http.StatusBadRequest, "parentId is required")
			return
		}
		if _, updateErr := h.db.Exec(r.Context(), `UPDATE gateway_candidates SET placement_parent_id=$2,dead_lettered_at=NULL,updated_at=now() WHERE id=$1`, item.ID, parentID); updateErr != nil {
			h.internal(w, "remember placement", updateErr)
			return
		}
		var createErr error
		item, createErr = h.commitPlacement(r.Context(), item, parentID)
		if createErr != nil {
			item, err = h.recordPlacementFailure(r.Context(), item.ID, createErr)
			if err != nil {
				h.internal(w, "schedule placement retry", err)
				return
			}
			if form {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			writeGatewayJSON(w, http.StatusAccepted, item)
			return
		}
	}
	if form {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeGatewayJSON(w, http.StatusOK, item)
}

func (h *Handler) commitPlacement(ctx context.Context, item Candidate, parentID string) (Candidate, error) {
	nodeID, revision, workspaceID, err := h.createNativeNode(ctx, item, parentID)
	if err != nil {
		return item, err
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return item, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO gateway_mappings(provider,installation_key,external_id,external_version,workspace_id,work_graph_node_id,last_native_revision) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(provider,installation_key,external_id) DO UPDATE SET external_version=EXCLUDED.external_version,work_graph_node_id=EXCLUDED.work_graph_node_id,last_native_revision=EXCLUDED.last_native_revision,updated_at=now()`, item.Provider, item.InstallationKey, item.ExternalID, item.ExternalVersion, workspaceID, nodeID, revision)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE gateway_candidates SET status='placed',work_graph_node_id=$2,last_error='',next_attempt_at=NULL,dead_lettered_at=NULL,last_attempt_at=now(),updated_at=now() WHERE id=$1`, item.ID, nodeID)
	}
	if err != nil {
		return item, err
	}
	if err = tx.Commit(ctx); err != nil {
		return item, err
	}
	return h.getCandidate(ctx, item.ID)
}

func (h *Handler) getCandidate(ctx context.Context, id string) (Candidate, error) {
	return scanCandidate(h.db.QueryRow(ctx, `SELECT `+candidateColumns+` FROM gateway_candidates WHERE id::text=$1`, id))
}

func (h *Handler) retryCandidate(w http.ResponseWriter, r *http.Request) {
	item, err := h.getCandidate(r.Context(), r.PathValue("candidateID"))
	if err == pgx.ErrNoRows {
		writeGatewayError(w, http.StatusNotFound, "candidate not found")
		return
	}
	if err != nil {
		h.internal(w, "read candidate", err)
		return
	}
	if item.WorkGraphNodeID != nil {
		writeGatewayError(w, http.StatusConflict, "candidate is already placed")
		return
	}
	if item.PlacementParentID == nil {
		writeGatewayError(w, http.StatusConflict, "choose a parent before retrying placement")
		return
	}
	_, err = h.db.Exec(r.Context(), `UPDATE gateway_candidates SET status='retrying',attempt_count=0,next_attempt_at=now(),dead_lettered_at=NULL,last_error='',updated_at=now() WHERE id=$1`, item.ID)
	if err != nil {
		h.internal(w, "retry candidate", err)
		return
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	item, _ = h.getCandidate(r.Context(), item.ID)
	writeGatewayJSON(w, http.StatusAccepted, item)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 15 * time.Second * time.Duration(1<<min(attempt-1, 6))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func (h *Handler) recordPlacementFailure(ctx context.Context, id string, cause error) (Candidate, error) {
	item, err := h.getCandidate(ctx, id)
	if err != nil {
		return Candidate{}, err
	}
	attempt := item.AttemptCount + 1
	status := "retrying"
	var next *time.Time
	var dead *time.Time
	now := time.Now().UTC()
	if attempt >= item.MaxAttempts {
		status = "dead_letter"
		dead = &now
	} else {
		value := now.Add(retryDelay(attempt))
		next = &value
	}
	_, err = h.db.Exec(ctx, `UPDATE gateway_candidates SET status=$2,attempt_count=$3,next_attempt_at=$4,last_attempt_at=$5,dead_lettered_at=$6,last_error=$7,updated_at=now() WHERE id=$1`, id, status, attempt, next, now, dead, cause.Error())
	if err != nil {
		return Candidate{}, err
	}
	return h.getCandidate(ctx, id)
}

type scanner interface{ Scan(...any) error }

func scanCandidate(row scanner) (Candidate, error) {
	var item Candidate
	var recommendations []byte
	err := row.Scan(&item.ID, &item.Provider, &item.InstallationKey, &item.ExternalID, &item.ExternalVersion, &item.Title, &item.Description, &item.Status, &recommendations, &item.WorkGraphNodeID, &item.LastError, &item.PlacementParentID, &item.AttemptCount, &item.MaxAttempts, &item.NextAttemptAt, &item.LastAttemptAt, &item.DeadLetteredAt, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(recommendations, &item.Recommendations)
	}
	return item, err
}
