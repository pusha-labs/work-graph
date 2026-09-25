package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type knowledgeHolder struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	Sources     []string `json:"sources"`
}

type knowledgeSubject struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	SubjectType string            `json:"subjectType"`
	CreatedAt   time.Time         `json:"createdAt"`
	Holders     []knowledgeHolder `json:"holders"`
}

func (s *server) listKnowledgeSubjects(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	rows, err := s.db.Query(r.Context(), `SELECT id,name,subject_type,created_at FROM knowledge_subjects WHERE workspace_id=$1 ORDER BY subject_type,name`, workspaceID)
	if err != nil {
		s.internalError(w, "list knowledge subjects", err)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (knowledgeSubject, error) {
		var item knowledgeSubject
		err := row.Scan(&item.ID, &item.Name, &item.SubjectType, &item.CreatedAt)
		item.Holders = []knowledgeHolder{}
		return item, err
	})
	rows.Close()
	if err != nil {
		s.internalError(w, "read knowledge subjects", err)
		return
	}
	byID := make(map[string]*knowledgeSubject, len(items))
	for index := range items {
		byID[items[index].ID] = &items[index]
	}
	holderRows, err := s.db.Query(r.Context(), `
		SELECT ak.subject_id,a.id,a.display_name,array_agg(DISTINCT ak.claim_source ORDER BY ak.claim_source)
		FROM actor_knowledge ak JOIN actors a ON a.id=ak.actor_id JOIN knowledge_subjects ks ON ks.id=ak.subject_id
		WHERE ks.workspace_id=$1 GROUP BY ak.subject_id,a.id,a.display_name ORDER BY a.display_name`, workspaceID)
	if err != nil {
		s.internalError(w, "list knowledge holders", err)
		return
	}
	defer holderRows.Close()
	for holderRows.Next() {
		var subjectID string
		var holder knowledgeHolder
		if err := holderRows.Scan(&subjectID, &holder.ID, &holder.DisplayName, &holder.Sources); err != nil {
			s.internalError(w, "read knowledge holder", err)
			return
		}
		if item := byID[subjectID]; item != nil {
			item.Holders = append(item.Holders, holder)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) createKnowledgeSubject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		SubjectType string `json:"subjectType"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	allowed := map[string]bool{"service": true, "project": true, "system": true, "domain": true, "other": true}
	if input.Name == "" || !allowed[input.SubjectType] {
		writeError(w, http.StatusBadRequest, "valid subject name and type are required")
		return
	}
	var result knowledgeSubject
	err := s.db.QueryRow(r.Context(), `INSERT INTO knowledge_subjects(workspace_id,name,subject_type) VALUES ($1,$2,$3) RETURNING id,name,subject_type,created_at`, r.PathValue("workspaceID"), input.Name, input.SubjectType).Scan(&result.ID, &result.Name, &result.SubjectType, &result.CreatedAt)
	if err != nil {
		s.internalError(w, "create knowledge subject", err)
		return
	}
	result.Holders = []knowledgeHolder{}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "knowledge_subject.created", "Knowledge subject added", result.Name+" · "+result.SubjectType); err != nil {
		s.internalError(w, "audit knowledge subject", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) addActorKnowledge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SubjectID string `json:"subjectId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	command, err := s.db.Exec(r.Context(), `
		INSERT INTO actor_knowledge(actor_id,subject_id,claim_source)
		SELECT a.id,ks.id,'admin' FROM actors a JOIN knowledge_subjects ks ON ks.workspace_id=a.workspace_id
		WHERE a.workspace_id=$1 AND a.id=$2 AND ks.id=$3
		ON CONFLICT DO NOTHING`, r.PathValue("workspaceID"), r.PathValue("actorID"), input.SubjectID)
	if err != nil {
		s.internalError(w, "assign actor knowledge", err)
		return
	}
	if command.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM actor_knowledge WHERE actor_id=$1 AND subject_id=$2 AND claim_source='admin')`, r.PathValue("actorID"), input.SubjectID).Scan(&exists)
		if !exists {
			writeError(w, http.StatusNotFound, "actor or knowledge subject not found")
			return
		}
	}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "knowledge.assigned", "Entity knowledge assigned", "actor "+r.PathValue("actorID")+" · subject "+input.SubjectID); err != nil {
		s.internalError(w, "audit knowledge assignment", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeActorKnowledge(w http.ResponseWriter, r *http.Request) {
	command, err := s.db.Exec(r.Context(), `DELETE FROM actor_knowledge ak USING actors a,knowledge_subjects ks WHERE ak.actor_id=a.id AND ak.subject_id=ks.id AND a.workspace_id=$1 AND ks.workspace_id=$1 AND ak.actor_id=$2 AND ak.subject_id=$3 AND ak.claim_source='admin'`, r.PathValue("workspaceID"), r.PathValue("actorID"), r.PathValue("subjectID"))
	if err != nil {
		s.internalError(w, "remove administrative knowledge", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "administrative knowledge claim not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	if err = recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "knowledge.unassigned", "Administrative knowledge removed", "actor "+r.PathValue("actorID")+" · subject "+r.PathValue("subjectID")); err != nil {
		s.internalError(w, "audit knowledge removal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
