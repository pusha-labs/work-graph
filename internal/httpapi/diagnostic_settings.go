package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

type diagnosticSettings struct {
	WideBranchChildren  int `json:"wideBranchChildren"`
	RequesterReviewDays int `json:"requesterReviewDays"`
	BlockedWorkDays     int `json:"blockedWorkDays"`
}

func (s *server) diagnosticSettingsForWorkspace(r *http.Request) (diagnosticSettings, error) {
	result := diagnosticSettings{WideBranchChildren: 8, RequesterReviewDays: 7, BlockedWorkDays: 14}
	err := s.db.QueryRow(r.Context(), `SELECT wide_branch_children,requester_review_days,blocked_work_days FROM workspace_diagnostic_settings WHERE workspace_id=$1`, r.PathValue("workspaceID")).Scan(&result.WideBranchChildren, &result.RequesterReviewDays, &result.BlockedWorkDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	return result, err
}

func (s *server) getDiagnosticSettings(w http.ResponseWriter, r *http.Request) {
	result, err := s.diagnosticSettingsForWorkspace(r)
	if err != nil {
		s.internalError(w, "get diagnostic settings", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) updateDiagnosticSettings(w http.ResponseWriter, r *http.Request) {
	var input diagnosticSettings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.WideBranchChildren < 4 || input.WideBranchChildren > 50 || input.RequesterReviewDays < 1 || input.RequesterReviewDays > 90 || input.BlockedWorkDays < 1 || input.BlockedWorkDays > 365 {
		writeError(w, http.StatusBadRequest, "diagnostic thresholds are outside their allowed ranges")
		return
	}
	if err := s.db.QueryRow(r.Context(), `INSERT INTO workspace_diagnostic_settings(workspace_id,wide_branch_children,requester_review_days,blocked_work_days) VALUES ($1,$2,$3,$4) ON CONFLICT(workspace_id) DO UPDATE SET wide_branch_children=EXCLUDED.wide_branch_children,requester_review_days=EXCLUDED.requester_review_days,blocked_work_days=EXCLUDED.blocked_work_days,updated_at=now() RETURNING wide_branch_children,requester_review_days,blocked_work_days`, r.PathValue("workspaceID"), input.WideBranchChildren, input.RequesterReviewDays, input.BlockedWorkDays).Scan(&input.WideBranchChildren, &input.RequesterReviewDays, &input.BlockedWorkDays); err != nil {
		s.internalError(w, "update diagnostic settings", err)
		return
	}
	writeJSON(w, http.StatusOK, input)
}
