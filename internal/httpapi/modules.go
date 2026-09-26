package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type workflowModule struct {
	ModuleID            string          `json:"moduleId"`
	ModuleVersion       string          `json:"moduleVersion"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	StepType            string          `json:"stepType"`
	SearchTerms         []string        `json:"searchTerms"`
	ConfigurationSchema json.RawMessage `json:"configurationSchema"`
	Publisher           string          `json:"publisher"`
}

type moduleInstallation struct {
	workflowModule
	Installed        bool     `json:"installed"`
	Enabled          bool     `json:"enabled"`
	AllowedHosts     []string `json:"allowedHosts"`
	AllowSecrets     bool     `json:"allowSecrets"`
	PublisherTrusted bool     `json:"publisherTrusted"`
}

func (s *server) listWorkflowModules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT m.module_id,m.module_version,m.name,m.description,m.step_type,m.search_terms,m.configuration_schema,m.publisher FROM workflow_modules m JOIN workspace_module_installations i ON i.module_id=m.module_id AND i.module_version=m.module_version WHERE m.enabled AND i.enabled AND i.publisher_trusted AND i.workspace_id=$1 ORDER BY m.name,m.module_version DESC`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list workflow modules", err)
		return
	}
	defer rows.Close()
	modules := []workflowModule{}
	for rows.Next() {
		var module workflowModule
		if err := rows.Scan(&module.ModuleID, &module.ModuleVersion, &module.Name, &module.Description, &module.StepType, &module.SearchTerms, &module.ConfigurationSchema, &module.Publisher); err != nil {
			s.internalError(w, "read workflow module", err)
			return
		}
		modules = append(modules, module)
	}
	writeJSON(w, http.StatusOK, modules)
}

func (s *server) listModuleInstallations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT m.module_id,m.module_version,m.name,m.description,m.step_type,m.search_terms,m.configuration_schema,m.publisher,i.workspace_id IS NOT NULL,COALESCE(i.enabled,false),COALESCE(i.allowed_hosts,'{}'::text[]),COALESCE(i.allow_secrets,false),COALESCE(i.publisher_trusted,false) FROM workflow_modules m LEFT JOIN workspace_module_installations i ON i.module_id=m.module_id AND i.module_version=m.module_version AND i.workspace_id=$1 WHERE m.enabled ORDER BY m.name,m.module_version DESC`, r.PathValue("workspaceID"))
	if err != nil {
		s.internalError(w, "list module installations", err)
		return
	}
	defer rows.Close()
	items := []moduleInstallation{}
	for rows.Next() {
		var item moduleInstallation
		if err := rows.Scan(&item.ModuleID, &item.ModuleVersion, &item.Name, &item.Description, &item.StepType, &item.SearchTerms, &item.ConfigurationSchema, &item.Publisher, &item.Installed, &item.Enabled, &item.AllowedHosts, &item.AllowSecrets, &item.PublisherTrusted); err != nil {
			s.internalError(w, "read module installation", err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) installModule(w http.ResponseWriter, r *http.Request) {
	result, err := s.db.Exec(r.Context(), `
		INSERT INTO workspace_module_installations(workspace_id,module_id,module_version,enabled,publisher_trusted)
		SELECT $1,module_id,module_version,false,false FROM workflow_modules
		WHERE module_id=$2 AND module_version=$3 AND enabled
		ON CONFLICT DO NOTHING`, r.PathValue("workspaceID"), r.PathValue("moduleID"), r.PathValue("moduleVersion"))
	if err != nil {
		s.internalError(w, "install workflow module", err)
		return
	}
	if result.RowsAffected() == 0 {
		var exists bool
		if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workflow_modules WHERE module_id=$1 AND module_version=$2 AND enabled)`, r.PathValue("moduleID"), r.PathValue("moduleVersion")).Scan(&exists); err != nil {
			s.internalError(w, "find workflow module", err)
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "module version not found")
			return
		}
		writeError(w, http.StatusConflict, "module version is already installed")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) updateModuleInstallation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled          bool     `json:"enabled"`
		AllowedHosts     []string `json:"allowedHosts"`
		AllowSecrets     bool     `json:"allowSecrets"`
		PublisherTrusted bool     `json:"publisherTrusted"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled && !input.PublisherTrusted {
		writeError(w, http.StatusBadRequest, "publisher trust is required before a module can be enabled")
		return
	}
	if !input.Enabled {
		var referenced bool
		if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM workflow_steps s JOIN work_nodes n ON n.id=s.work_node_id WHERE n.workspace_id=$1 AND s.module_id=$2 AND s.module_version=$3 AND s.step_status NOT IN ('completed','failed'))`, r.PathValue("workspaceID"), r.PathValue("moduleID"), r.PathValue("moduleVersion")).Scan(&referenced); err != nil {
			s.internalError(w, "check module references", err)
			return
		}
		if referenced {
			writeError(w, http.StatusConflict, "module is referenced by unfinished route stages")
			return
		}
	}
	hosts := make([]string, 0, len(input.AllowedHosts))
	seen := map[string]bool{}
	for _, raw := range input.AllowedHosts {
		host := strings.ToLower(strings.TrimSpace(raw))
		if host == "" {
			continue
		}
		parsed, err := url.Parse("https://" + host)
		if err != nil || parsed.Hostname() != host || parsed.Port() != "" || strings.ContainsAny(host, "/*") {
			writeError(w, http.StatusBadRequest, "allowed hosts must be exact hostnames without scheme, path, port, or wildcard")
			return
		}
		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	result, err := s.db.Exec(r.Context(), `UPDATE workspace_module_installations SET enabled=$4,allowed_hosts=$5,allow_secrets=$6,publisher_trusted=$7,updated_at=now() WHERE workspace_id=$1 AND module_id=$2 AND module_version=$3`, r.PathValue("workspaceID"), r.PathValue("moduleID"), r.PathValue("moduleVersion"), input.Enabled, hosts, input.AllowSecrets, input.PublisherTrusted)
	if err != nil {
		s.internalError(w, "update module installation", err)
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "module installation not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) validateModulePolicy(r *http.Request, workspaceID, moduleID, moduleVersion string, configuration map[string]any) string {
	var enabled, allowSecrets bool
	var allowedHosts []string
	var publisherTrusted bool
	if err := s.db.QueryRow(r.Context(), `SELECT enabled,allowed_hosts,allow_secrets,publisher_trusted FROM workspace_module_installations WHERE workspace_id=$1 AND module_id=$2 AND module_version=$3`, workspaceID, moduleID, moduleVersion).Scan(&enabled, &allowedHosts, &allowSecrets, &publisherTrusted); err != nil || !enabled || !publisherTrusted {
		return "workflow module is disabled in this workspace"
	}
	if moduleID != "builtin.http" {
		return ""
	}
	parsed, err := url.Parse(stringValue(configuration["url"]))
	if err != nil || parsed.Hostname() == "" {
		return "a valid HTTP URL is required"
	}
	host := strings.ToLower(parsed.Hostname())
	allowed := false
	for _, item := range allowedHosts {
		allowed = allowed || item == host
	}
	if !allowed {
		return "destination host is not permitted for this module"
	}
	if stringValue(configuration["secretId"]) != "" && !allowSecrets {
		return "secret access is disabled for this module"
	}
	return ""
}
