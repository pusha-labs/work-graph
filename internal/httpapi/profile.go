package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"
)

type capabilityClaim struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	CapabilityType string          `json:"capabilityType"`
	Sources        []string        `json:"sources"`
	Evidence       []claimEvidence `json:"evidence"`
}

type claimEvidence struct {
	Source     string    `json:"source"`
	Level      string    `json:"level"`
	Note       string    `json:"note"`
	ReviewedAt time.Time `json:"reviewedAt"`
}

type actorProfile struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName"`
	ActorType   string            `json:"actorType"`
	Role        string            `json:"workspaceRole"`
	Claims      []capabilityClaim `json:"claims"`
	Knowledge   []knowledgeClaim  `json:"knowledge"`
	CreatedAt   time.Time         `json:"createdAt"`
	Estimation  estimationStats   `json:"estimation"`
}

type estimationStats struct {
	ObservationCount          int        `json:"observationCount"`
	MeanRelativeError         float64    `json:"meanRelativeError"`
	MeanAbsoluteRelativeError float64    `json:"meanAbsoluteRelativeError"`
	Stability                 float64    `json:"stability"`
	LongOverrunCount          int        `json:"longOverrunCount"`
	LongOverrunRate           float64    `json:"longOverrunRate"`
	MeanLongOverrunSeverity   float64    `json:"meanLongOverrunSeverity"`
	UpdatedAt                 *time.Time `json:"updatedAt"`
}

type knowledgeClaim struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	SubjectType string          `json:"subjectType"`
	Sources     []string        `json:"sources"`
	Evidence    []claimEvidence `json:"evidence"`
}

func (s *server) getMyProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspaceID")
	current, _ := accountFromContext(r.Context())
	var result actorProfile
	err := s.db.QueryRow(r.Context(), `
		SELECT a.id,a.display_name,a.actor_type,aa.workspace_role,a.created_at
		FROM account_actors aa JOIN actors a ON a.id=aa.actor_id
		WHERE aa.account_id=$1 AND a.workspace_id=$2`, current.ID, workspaceID).Scan(
		&result.ID, &result.DisplayName, &result.ActorType, &result.Role, &result.CreatedAt)
	if err != nil {
		s.writeDatabaseError(w, "find profile", err)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT c.id,c.name,c.capability_type,array_agg(ac.claim_source ORDER BY ac.claim_source),jsonb_agg(jsonb_build_object('source',ac.claim_source,'level',ac.claim_level,'note',ac.evidence_note,'reviewedAt',ac.reviewed_at) ORDER BY ac.claim_source)
		FROM actor_capabilities ac JOIN capabilities c ON c.id=ac.capability_id
		WHERE ac.actor_id=$1 GROUP BY c.id,c.name,c.capability_type ORDER BY c.capability_type,c.name`, result.ID)
	if err != nil {
		s.internalError(w, "list profile capabilities", err)
		return
	}
	defer rows.Close()
	result.Claims = []capabilityClaim{}
	for rows.Next() {
		var claim capabilityClaim
		var evidence []byte
		if err := rows.Scan(&claim.ID, &claim.Name, &claim.CapabilityType, &claim.Sources, &evidence); err != nil {
			s.internalError(w, "read profile capability", err)
			return
		}
		_ = json.Unmarshal(evidence, &claim.Evidence)
		result.Claims = append(result.Claims, claim)
	}
	knowledgeRows, err := s.db.Query(r.Context(), `SELECT ks.id,ks.name,ks.subject_type,array_agg(ak.claim_source ORDER BY ak.claim_source),jsonb_agg(jsonb_build_object('source',ak.claim_source,'level',ak.claim_level,'note',ak.evidence_note,'reviewedAt',ak.reviewed_at) ORDER BY ak.claim_source) FROM actor_knowledge ak JOIN knowledge_subjects ks ON ks.id=ak.subject_id WHERE ak.actor_id=$1 GROUP BY ks.id,ks.name,ks.subject_type ORDER BY ks.subject_type,ks.name`, result.ID)
	if err != nil {
		s.internalError(w, "list profile knowledge", err)
		return
	}
	defer knowledgeRows.Close()
	result.Knowledge = []knowledgeClaim{}
	for knowledgeRows.Next() {
		var claim knowledgeClaim
		var evidence []byte
		if err = knowledgeRows.Scan(&claim.ID, &claim.Name, &claim.SubjectType, &claim.Sources, &evidence); err != nil {
			s.internalError(w, "read profile knowledge", err)
			return
		}
		_ = json.Unmarshal(evidence, &claim.Evidence)
		result.Knowledge = append(result.Knowledge, claim)
	}
	var m2, severitySum float64
	if err = s.db.QueryRow(r.Context(), `SELECT COALESCE(es.observation_count,0),COALESCE(es.mean_relative_error,0),COALESCE(es.relative_error_m2,0),COALESCE(es.mean_absolute_relative_error,0),COALESCE(es.long_overrun_count,0),COALESCE(es.long_overrun_severity_sum,0),es.updated_at FROM actors a LEFT JOIN actor_estimation_stats es ON es.actor_id=a.id WHERE a.id=$1`, result.ID).Scan(&result.Estimation.ObservationCount, &result.Estimation.MeanRelativeError, &m2, &result.Estimation.MeanAbsoluteRelativeError, &result.Estimation.LongOverrunCount, &severitySum, &result.Estimation.UpdatedAt); err != nil {
		s.internalError(w, "read estimation statistics", err)
		return
	}
	if result.Estimation.ObservationCount > 1 {
		result.Estimation.Stability = math.Sqrt(m2 / float64(result.Estimation.ObservationCount-1))
	}
	if result.Estimation.ObservationCount > 0 {
		result.Estimation.LongOverrunRate = float64(result.Estimation.LongOverrunCount) / float64(result.Estimation.ObservationCount)
	}
	if result.Estimation.LongOverrunCount > 0 {
		result.Estimation.MeanLongOverrunSeverity = severitySum / float64(result.Estimation.LongOverrunCount)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) addMySkill(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CapabilityID string `json:"capabilityId"`
		Level        string `json:"level"`
		Note         string `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Level = claimLevel(input.Level)
	input.Note = strings.TrimSpace(input.Note)
	if input.Level == "" || len(input.Note) > 1000 {
		writeError(w, http.StatusBadRequest, "valid claim level and note up to 1000 characters are required")
		return
	}
	actorID, err := s.currentActorID(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		s.writeDatabaseError(w, "find current profile", err)
		return
	}
	command, err := s.db.Exec(r.Context(), `
		INSERT INTO actor_capabilities(actor_id,capability_id,claim_source,claim_level,evidence_note,reviewed_at)
		SELECT $1,c.id,'self',$4,$5,now() FROM capabilities c
		WHERE c.id=$2 AND c.workspace_id=$3 AND c.capability_type='skill'
		ON CONFLICT(actor_id,capability_id,claim_source) DO UPDATE SET claim_level=EXCLUDED.claim_level,evidence_note=EXCLUDED.evidence_note,reviewed_at=now()`, actorID, input.CapabilityID, r.PathValue("workspaceID"), input.Level, input.Note)
	if err != nil {
		s.internalError(w, "add self-declared skill", err)
		return
	}
	if command.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM actor_capabilities WHERE actor_id=$1 AND capability_id=$2 AND claim_source='self')`, actorID, input.CapabilityID).Scan(&exists)
		if !exists {
			writeError(w, http.StatusBadRequest, "only skills from this workspace can be self-declared")
			return
		}
	}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "skill.self_declared", "Skill self-declared", input.CapabilityID); err != nil {
		s.internalError(w, "audit self-declared skill", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeMySkill(w http.ResponseWriter, r *http.Request) {
	actorID, err := s.currentActorID(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		s.writeDatabaseError(w, "find current profile", err)
		return
	}
	command, err := s.db.Exec(r.Context(), `DELETE FROM actor_capabilities WHERE actor_id=$1 AND capability_id=$2 AND claim_source='self'`, actorID, r.PathValue("capabilityID"))
	if err != nil {
		s.internalError(w, "remove self-declared skill", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "self-declared skill not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	if err := recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "skill.self_removed", "Self-declared skill removed", r.PathValue("capabilityID")); err != nil {
		s.internalError(w, "audit removed skill", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addMyKnowledge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SubjectID string `json:"subjectId"`
		Level     string `json:"level"`
		Note      string `json:"note"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Level, input.Note = claimLevel(input.Level), strings.TrimSpace(input.Note)
	if input.Level == "" || len(input.Note) > 1000 {
		writeError(w, http.StatusBadRequest, "valid claim level and note up to 1000 characters are required")
		return
	}
	actorID, err := s.currentActorID(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		s.writeDatabaseError(w, "find current profile", err)
		return
	}
	command, err := s.db.Exec(r.Context(), `INSERT INTO actor_knowledge(actor_id,subject_id,claim_source,claim_level,evidence_note,reviewed_at) SELECT $1,ks.id,'self',$4,$5,now() FROM knowledge_subjects ks WHERE ks.id=$2 AND ks.workspace_id=$3 ON CONFLICT(actor_id,subject_id,claim_source) DO UPDATE SET claim_level=EXCLUDED.claim_level,evidence_note=EXCLUDED.evidence_note,reviewed_at=now()`, actorID, input.SubjectID, r.PathValue("workspaceID"), input.Level, input.Note)
	if err != nil {
		s.internalError(w, "add self-declared knowledge", err)
		return
	}
	if command.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM actor_knowledge WHERE actor_id=$1 AND subject_id=$2 AND claim_source='self')`, actorID, input.SubjectID).Scan(&exists)
		if !exists {
			writeError(w, http.StatusBadRequest, "only subjects from this workspace can be self-declared")
			return
		}
	}
	current, _ := accountFromContext(r.Context())
	if err = recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "knowledge.self_declared", "Knowledge self-declared", input.SubjectID); err != nil {
		s.internalError(w, "audit self-declared knowledge", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func claimLevel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "working"
	}
	if map[string]bool{"awareness": true, "working": true, "advanced": true, "expert": true}[value] {
		return value
	}
	return ""
}

func (s *server) removeMyKnowledge(w http.ResponseWriter, r *http.Request) {
	actorID, err := s.currentActorID(r.Context(), r.PathValue("workspaceID"))
	if err != nil {
		s.writeDatabaseError(w, "find current profile", err)
		return
	}
	command, err := s.db.Exec(r.Context(), `DELETE FROM actor_knowledge WHERE actor_id=$1 AND subject_id=$2 AND claim_source='self'`, actorID, r.PathValue("subjectID"))
	if err != nil {
		s.internalError(w, "remove self-declared knowledge", err)
		return
	}
	if command.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "self-declared knowledge not found")
		return
	}
	current, _ := accountFromContext(r.Context())
	if err = recordAudit(r.Context(), s.db, r.PathValue("workspaceID"), current.ID, "knowledge.self_removed", "Self-declared knowledge removed", r.PathValue("subjectID")); err != nil {
		s.internalError(w, "audit removed knowledge", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
