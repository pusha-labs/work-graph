package httpapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type placementRecommendation struct {
	ParentID    string   `json:"parentId"`
	ParentTitle string   `json:"parentTitle"`
	Path        []string `json:"path"`
	Score       int      `json:"score"`
	Confidence  string   `json:"confidence"`
	Reasons     []string `json:"reasons"`
}

type placementSignals struct {
	Capabilities map[string]string
	Knowledge    map[string]string
}

var placementSeparator = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func (s *server) recommendPlacement(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title               string   `json:"title"`
		DesiredOutcome      string   `json:"desiredOutcome"`
		CapabilityIDs       []string `json:"capabilityIds"`
		KnowledgeSubjectIDs []string `json:"knowledgeSubjectIds"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title, input.DesiredOutcome = strings.TrimSpace(input.Title), strings.TrimSpace(input.DesiredOutcome)
	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	workspaceID := r.PathValue("workspaceID")
	var revision int64
	if err := s.db.QueryRow(r.Context(), `SELECT revision FROM workspaces WHERE id=$1`, workspaceID).Scan(&revision); err != nil {
		s.writeDatabaseError(w, "find workspace", err)
		return
	}
	nodes, err := s.readPlacementNodes(r, workspaceID)
	if err != nil {
		s.internalError(w, "read placement nodes", err)
		return
	}
	requestedCapabilities, err := s.readNamedSignals(r, workspaceID, "capabilities", input.CapabilityIDs)
	if err != nil {
		s.internalError(w, "read placement capabilities", err)
		return
	}
	requestedKnowledge, err := s.readNamedSignals(r, workspaceID, "knowledge_subjects", input.KnowledgeSubjectIDs)
	if err != nil {
		s.internalError(w, "read placement knowledge", err)
		return
	}
	if len(requestedCapabilities) != len(uniqueStrings(input.CapabilityIDs)) || len(requestedKnowledge) != len(uniqueStrings(input.KnowledgeSubjectIDs)) {
		writeError(w, http.StatusBadRequest, "all placement signals must belong to the workspace")
		return
	}
	signalsByNode, err := s.readPlacementSignals(r, workspaceID)
	if err != nil {
		s.internalError(w, "read placement signals", err)
		return
	}
	recommendations := rankPlacement(input.Title+" "+input.DesiredOutcome, nodes, signalsByNode, requestedCapabilities, requestedKnowledge)
	w.Header().Set("ETag", `"`+formatRevision(revision)+`"`)
	writeJSON(w, http.StatusOK, map[string]any{"workspaceRevision": revision, "recommendations": recommendations})
}

func (s *server) readPlacementNodes(r *http.Request, workspaceID string) ([]workNode, error) {
	rows, err := s.db.Query(r.Context(), `SELECT id,workspace_id,root_id,parent_id,title,desired_outcome,lifecycle_status,created_revision,updated_revision,created_at,updated_at FROM work_nodes WHERE workspace_id=$1 AND removed_revision IS NULL ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[workNode])
}

func (s *server) readNamedSignals(r *http.Request, workspaceID, table string, ids []string) (map[string]string, error) {
	result := map[string]string{}
	if len(ids) == 0 {
		return result, nil
	}
	query := `SELECT id::text,name FROM ` + table + ` WHERE workspace_id=$1 AND id::text=ANY($2)`
	rows, err := s.db.Query(r.Context(), query, workspaceID, uniqueStrings(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		result[id] = name
	}
	return result, rows.Err()
}

func (s *server) readPlacementSignals(r *http.Request, workspaceID string) (map[string]placementSignals, error) {
	result := map[string]placementSignals{}
	queries := []struct {
		query      string
		capability bool
	}{
		{`SELECT s.work_node_id::text,c.id::text,c.name FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workflow_step_capabilities r ON r.workflow_step_id=s.id JOIN capabilities c ON c.id=r.capability_id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL`, true},
		{`SELECT s.work_node_id::text,k.id::text,k.name FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id JOIN workflow_step_knowledge r ON r.workflow_step_id=s.id JOIN knowledge_subjects k ON k.id=r.subject_id WHERE n.workspace_id=$1 AND n.removed_revision IS NULL`, false},
	}
	for _, item := range queries {
		rows, err := s.db.Query(r.Context(), item.query, workspaceID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var nodeID, id, name string
			if err := rows.Scan(&nodeID, &id, &name); err != nil {
				rows.Close()
				return nil, err
			}
			signals := result[nodeID]
			if signals.Capabilities == nil {
				signals.Capabilities, signals.Knowledge = map[string]string{}, map[string]string{}
			}
			if item.capability {
				signals.Capabilities[id] = name
			} else {
				signals.Knowledge[id] = name
			}
			result[nodeID] = signals
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

func rankPlacement(text string, nodes []workNode, signalsByNode map[string]placementSignals, requestedCapabilities, requestedKnowledge map[string]string) []placementRecommendation {
	terms := placementTerms(text)
	byID := map[string]workNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	result := make([]placementRecommendation, 0)
	for _, node := range nodes {
		score, reasons := 0, []string{}
		shared := intersectTerms(terms, placementTerms(node.Title+" "+node.DesiredOutcome))
		if len(shared) > 0 {
			points := min(len(shared)*6, 30)
			score += points
			reasons = append(reasons, "Shared terms: "+strings.Join(shared, ", "))
		}
		signals := signalsByNode[node.ID]
		for id, name := range requestedKnowledge {
			if _, match := signals.Knowledge[id]; match {
				score += 35
				reasons = append(reasons, "Uses knowledge of "+name)
			}
		}
		for id, name := range requestedCapabilities {
			if _, match := signals.Capabilities[id]; match {
				score += 25
				reasons = append(reasons, "Uses capability "+name)
			}
		}
		if score == 0 {
			continue
		}
		depth := nodeDepth(node, byID)
		score = min(score+min(depth, 5), 100)
		sort.Strings(reasons)
		result = append(result, placementRecommendation{ParentID: node.ID, ParentTitle: node.Title, Path: nodePath(node, byID), Score: score, Confidence: placementConfidence(score), Reasons: reasons})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return strings.Join(result[i].Path, "\x00") < strings.Join(result[j].Path, "\x00")
	})
	if len(result) > 5 {
		result = result[:5]
	}
	return result
}

func placementTerms(value string) []string {
	stop := map[string]bool{"and": true, "the": true, "for": true, "with": true, "that": true, "this": true, "from": true, "или": true, "для": true, "это": true, "как": true, "что": true, "при": true}
	unique := map[string]bool{}
	for _, term := range placementSeparator.Split(strings.ToLower(value), -1) {
		if len([]rune(term)) >= 3 && !stop[term] {
			unique[term] = true
		}
	}
	result := make([]string, 0, len(unique))
	for term := range unique {
		result = append(result, term)
	}
	sort.Strings(result)
	return result
}

func intersectTerms(left, right []string) []string {
	set := map[string]bool{}
	for _, value := range left {
		set[value] = true
	}
	result := []string{}
	for _, value := range right {
		if set[value] {
			result = append(result, value)
		}
	}
	return result
}

func nodeDepth(node workNode, byID map[string]workNode) int {
	depth := 0
	for node.ParentID != nil {
		parent, ok := byID[*node.ParentID]
		if !ok {
			break
		}
		depth++
		node = parent
	}
	return depth
}

func nodePath(node workNode, byID map[string]workNode) []string {
	path := []string{node.Title}
	for node.ParentID != nil {
		parent, ok := byID[*node.ParentID]
		if !ok {
			break
		}
		path = append(path, parent.Title)
		node = parent
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func placementConfidence(score int) string {
	if score >= 60 {
		return "high"
	}
	if score >= 25 {
		return "medium"
	}
	return "low"
}

func uniqueStrings(values []string) []string {
	set := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !set[value] {
			set[value] = true
			result = append(result, value)
		}
	}
	return result
}

func formatRevision(revision int64) string {
	return strconv.FormatInt(revision, 10)
}
