package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (h *Handler) recommend(ctx context.Context, title, description string) ([]Recommendation, error) {
	workspaceID, _, err := h.workspace(ctx)
	if err != nil {
		return nil, err
	}
	var response struct {
		Recommendations []Recommendation `json:"recommendations"`
	}
	err = h.coreJSON(ctx, http.MethodPost, "/api/v1/workspaces/"+url.PathEscape(workspaceID)+"/placement-recommendations", map[string]any{"title": title, "desiredOutcome": description, "capabilityIds": []string{}, "knowledgeSubjectIds": []string{}}, nil, &response)
	return response.Recommendations, err
}

type CoreNode struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Title    string  `json:"title"`
	Status   string  `json:"lifecycleStatus"`
}

func (h *Handler) coreNodes(ctx context.Context) (string, []CoreNode, error) {
	workspaceID, _, err := h.workspace(ctx)
	if err != nil {
		return "", nil, err
	}
	var nodes []CoreNode
	err = h.coreJSON(ctx, http.MethodGet, "/api/v1/workspaces/"+url.PathEscape(workspaceID)+"/nodes", nil, nil, &nodes)
	return workspaceID, nodes, err
}

func (h *Handler) createNativeNode(ctx context.Context, item Candidate, parentID string) (string, int64, string, error) {
	workspaceID, revision, err := h.workspace(ctx)
	if err != nil {
		return "", 0, "", err
	}
	headers := map[string]string{
		"Idempotency-Key": item.Provider + ":" + item.InstallationKey + ":" + item.ExternalID + ":create",
		"If-Match":        `"` + strconv.FormatInt(revision, 10) + `"`,
	}
	var response struct {
		ID              string `json:"id"`
		UpdatedRevision int64  `json:"updatedRevision"`
	}
	err = h.coreJSON(ctx, http.MethodPost, "/api/v1/workspaces/"+url.PathEscape(workspaceID)+"/nodes", map[string]any{"parentId": parentID, "title": item.Title, "desiredOutcome": item.Description}, headers, &response)
	return response.ID, response.UpdatedRevision, workspaceID, err
}

func (h *Handler) workspace(ctx context.Context) (string, int64, error) {
	if h.config.ServiceToken == "" {
		return "", 0, fmt.Errorf("Work Graph service token is not configured")
	}
	var workspaces []struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := h.coreJSON(ctx, http.MethodGet, "/api/v1/workspaces", nil, nil, &workspaces); err != nil {
		return "", 0, err
	}
	for _, workspace := range workspaces {
		if h.config.WorkspaceID == "" || h.config.WorkspaceID == workspace.ID {
			h.config.WorkspaceID = workspace.ID
			return workspace.ID, workspace.Revision, nil
		}
	}
	return "", 0, fmt.Errorf("configured Work Graph workspace was not found")
}

func (h *Handler) coreJSON(ctx context.Context, method, path string, body any, headers map[string]string, destination any) error {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.config.CoreURL, "/")+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+h.config.ServiceToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := h.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&problem)
		if problem.Error == "" {
			problem.Error = response.Status
		}
		return fmt.Errorf("Work Graph: %s", problem.Error)
	}
	return json.NewDecoder(response.Body).Decode(destination)
}
