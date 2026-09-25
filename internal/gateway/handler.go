package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	logger *slog.Logger
	db     *pgxpool.Pool
	config Config
	client *http.Client
	router http.Handler
}

type Recommendation struct {
	ParentID    string   `json:"parentId"`
	ParentTitle string   `json:"parentTitle"`
	Path        []string `json:"path"`
	Score       int      `json:"score"`
	Confidence  string   `json:"confidence"`
	Reasons     []string `json:"reasons"`
}

type Candidate struct {
	ID                string           `json:"id"`
	Provider          string           `json:"provider"`
	InstallationKey   string           `json:"installationKey"`
	ExternalID        string           `json:"externalId"`
	ExternalVersion   string           `json:"externalVersion"`
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	Status            string           `json:"status"`
	Recommendations   []Recommendation `json:"recommendations"`
	WorkGraphNodeID   *string          `json:"workGraphNodeId"`
	LastError         string           `json:"lastError"`
	PlacementParentID *string          `json:"placementParentId"`
	AttemptCount      int              `json:"attemptCount"`
	MaxAttempts       int              `json:"maxAttempts"`
	NextAttemptAt     *time.Time       `json:"nextAttemptAt"`
	LastAttemptAt     *time.Time       `json:"lastAttemptAt"`
	DeadLetteredAt    *time.Time       `json:"deadLetteredAt"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

func NewHandler(logger *slog.Logger, db *pgxpool.Pool, config Config) *Handler {
	h := &Handler{logger: logger, db: db, config: config, client: &http.Client{Timeout: 15 * time.Second}}
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/candidates", h.listCandidates)
	protected.HandleFunc("POST /api/v1/mock/candidates", h.ingestMockCandidate)
	protected.HandleFunc("POST /api/v1/candidates/{candidateID}/place", h.placeCandidate)
	protected.HandleFunc("POST /api/v1/candidates/{candidateID}/retry", h.retryCandidate)
	protected.HandleFunc("GET /api/v1/outbound-events", h.listOutboundEvents)
	protected.HandleFunc("GET /api/v1/installations", h.listInstallations)
	protected.HandleFunc("POST /api/v1/installations", h.createInstallation)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/disable", h.disableInstallation)
	protected.HandleFunc("GET /api/v1/installations/{installationID}/oauth/start", h.startJiraOAuth)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/sync", h.syncJiraInstallation)
	protected.HandleFunc("GET /api/v1/installations/{installationID}/sync-runs", h.listSyncRuns)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/settings", h.updateJiraSettings)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/pause", h.pauseJiraSync)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/resume", h.resumeJiraSync)
	protected.HandleFunc("POST /api/v1/installations/{installationID}/reset-cursor", h.resetJiraCursor)
	protected.HandleFunc("GET /", h.inbox)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /ready", h.ready)
	mux.HandleFunc("GET /api/v1/oauth/jira/callback", h.finishJiraOAuth)
	mux.Handle("/", h.withAdminAuthentication(protected))
	h.router = mux
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	if h.config.ServiceToken == "" || h.config.AdminToken == "" {
		writeGatewayJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "reason": "gateway security is not configured"})
		return
	}
	writeGatewayJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) withAdminAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.config.AdminToken == "" {
			writeGatewayError(w, http.StatusServiceUnavailable, "GATEWAY_ADMIN_TOKEN is not configured")
			return
		}
		provided := ""
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			provided = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		} else if _, password, ok := r.BasicAuth(); ok {
			provided = password
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(h.config.AdminToken)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Work Graph Integration Gateway"`)
			writeGatewayError(w, http.StatusUnauthorized, "gateway administrator authentication required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
				writeGatewayError(w, http.StatusForbidden, "cross-origin gateway mutation rejected")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func decodeGatewayJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeGatewayError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeGatewayError(w http.ResponseWriter, status int, message string) {
	writeGatewayJSON(w, status, map[string]string{"error": message})
}
func writeGatewayJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) internal(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, "error", err)
	writeGatewayError(w, http.StatusInternalServerError, "internal error")
}
