package gateway

import (
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type treeOption struct {
	ID, Title, Path, Prefix, Status, TreeURL string
	Recommended                              bool
}

type inboxCandidate struct {
	Candidate
	Options   []treeOption
	NativeURL string
}

type inboxPage struct {
	Items          []inboxCandidate
	OutboundEvents []OutboundEvent
	Installations  []Installation
	CoreError      string
}

var treeInboxTemplate = template.Must(template.New("tree-inbox").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Work Graph · Integration inbox</title>
<style>
:root{color-scheme:light}*{box-sizing:border-box}body{margin:0;background:#f5f6f3;color:#20332b;font:15px system-ui,-apple-system,sans-serif}header{padding:22px max(24px,5vw);background:#173c30;color:white;display:flex;justify-content:space-between;align-items:center}header strong{font-size:18px}header span{color:#bcd0c6;font-size:13px}main{max-width:1040px;margin:auto;padding:32px max(20px,4vw)}.heading{display:flex;justify-content:space-between;align-items:end;margin-bottom:20px}.heading h1{font-size:25px;margin:0}.heading p{margin:5px 0 0;color:#6c7b74}.count{background:#e5ece8;border-radius:99px;padding:5px 10px;color:#385448;font-size:13px}.card{background:white;border:1px solid #dce4df;border-radius:16px;padding:20px;margin:14px 0;box-shadow:0 5px 18px #173c3008}.card.placed{opacity:.7}.meta{display:flex;gap:8px;align-items:center}.badge{padding:4px 8px;border-radius:20px;background:#fff0c2;color:#725818;font-size:12px}.badge.placed{background:#e3eee8;color:#28654d}.badge.retrying{background:#e8eef7;color:#375a86}.badge.dead_letter{background:#f7e3df;color:#8a3d32}.external{font:12px ui-monospace,monospace;color:#76837d}.card h2{font-size:18px;margin:13px 0 5px}.description{color:#53665d;margin:0 0 16px}.suggestion{border:1px solid #cbdad2;background:#f7faf8;border-radius:12px;padding:13px;margin:9px 0}.choice{display:flex;gap:10px;align-items:flex-start;cursor:pointer}.choice input{margin-top:4px}.choice strong{display:block}.path,.reason{font-size:12px;color:#66766e;margin-top:3px}.confidence{color:#28654d;font-size:12px;margin-left:5px}.open-tree{color:#28654d;text-decoration:none;font-size:12px}details{margin:13px 0}summary{cursor:pointer;color:#28654d;font-weight:600}.tree-picker{margin-top:10px;border:1px solid #dfe6e2;border-radius:12px;max-height:330px;overflow:auto;padding:6px}.tree-row{display:flex;align-items:center;gap:8px;padding:8px 9px;border-radius:8px;margin:1px 0}.tree-row:hover{background:#f0f5f2}.tree-row label{flex:1;display:flex;gap:8px;align-items:center;cursor:pointer}.prefix{font:13px ui-monospace,monospace;color:#9aaba2;white-space:pre}.tree-title{font-weight:550}.status{font-size:11px;color:#7a8982;margin-left:auto}.recommended-mark{font-size:11px;color:#28654d;background:#e3eee8;padding:2px 6px;border-radius:8px}.actions{display:flex;gap:9px;align-items:center;margin-top:14px}button{font:inherit;padding:9px 14px;border-radius:9px;background:#28654d;color:white;border:0;cursor:pointer}.native-link{color:#28654d;text-decoration:none;font-weight:600}.empty,.error{background:white;border:1px solid #dce4df;border-radius:14px;padding:24px;color:#6c7b74}.error{border-color:#e7c8c2;color:#8a3d32}.last-error{color:#9b4135;font-size:13px}.retry-panel{background:#eef3f8;border-radius:10px;padding:12px;color:#375a86}@media(max-width:650px){header span{display:none}.heading{align-items:start;gap:12px}.tree-row{padding-left:5px}.prefix{max-width:72px;overflow:hidden}}
</style></head><body>
<header><strong>Work Graph · Integration inbox</strong><span>External work enters the tree only after confirmation</span></header>
<main><div class="heading"><div><h1>Connections</h1><p>Provider credentials stay encrypted inside this gateway.</p></div><span class="count">{{len .Installations}} configured</span></div>
{{range .Installations}}<article class="card"><div class="meta"><span class="badge {{.Status}}">{{.Provider}} · {{.Status}}</span><span class="external">credentials v{{.CredentialVersion}}</span></div><h2>{{.DisplayName}}</h2><p class="description">{{.BaseURL}} · {{.AuthType}}{{if .HasCredentials}} · credentials stored{{end}}{{if .SyncEnabled}}{{if .LastSyncAt}} · last sync {{.LastSyncAt}}{{end}}{{if .NextSyncAt}} · next sync {{.NextSyncAt}}{{end}}{{end}}</p>{{if .LastError}}<p class="last-error">{{.LastError}}</p>{{end}}{{if .SyncError}}<p class="last-error">{{.SyncError}} · failed attempts {{.SyncFailures}}</p>{{end}}<div class="actions">{{if and (eq .Provider "jira_cloud") (eq .Status "connected")}}<form method="post" action="/api/v1/installations/{{.ID}}/sync"><button>Sync now</button></form>{{else if and (eq .Provider "jira_cloud") (ne .Status "disabled")}}<a class="native-link" href="/api/v1/installations/{{.ID}}/oauth/start">Connect with Atlassian →</a>{{end}}{{if ne .Status "disabled"}}<form method="post" action="/api/v1/installations/{{.ID}}/disable" onsubmit="return confirm('Disable this connection and erase its stored credentials?')"><button>Disable and erase credentials</button></form>{{end}}</div>{{if and (eq .Provider "jira_cloud") (eq .Status "connected")}}<details><summary>Synchronization settings and history</summary><form method="post" action="/api/v1/installations/{{.ID}}/settings"><label style="display:block;margin:10px 0">JQL filter<input style="display:block;width:100%;padding:9px;margin-top:4px" name="jql" value="{{index .Configuration "jql"}}"></label><label>Interval, minutes <input name="intervalMinutes" type="number" min="1" max="1440" value="{{.SyncIntervalMinutes}}" required></label> <label>Batch <input name="maxIssues" type="number" min="1" max="100" value="{{index .Configuration "maxIssues"}}" required></label><div class="actions"><button>Validate and save</button></div></form><div class="actions">{{if .SyncEnabled}}<form method="post" action="/api/v1/installations/{{.ID}}/pause"><button>Pause</button></form>{{else}}<form method="post" action="/api/v1/installations/{{.ID}}/resume"><button>Resume</button></form>{{end}}<form method="post" action="/api/v1/installations/{{.ID}}/reset-cursor" onsubmit="return confirm('Reset the cursor and re-read all matching Jira issues? Existing mappings will be preserved.')"><button>Reset cursor</button></form></div>{{range .RecentRuns}}<p class="description"><span class="badge {{.Status}}">{{.Status}}</span> {{.Trigger}} · {{.StartedAt}} · read {{.IssuesRead}}, imported {{.IssuesImported}}</p>{{else}}<p class="description">No synchronization runs yet.</p>{{end}}</details>{{end}}</article>{{else}}<div class="empty">No external system is configured yet.</div>{{end}}
<details><summary>Add Jira Cloud connection</summary><form class="card" method="post" action="/api/v1/installations"><label style="display:block;margin:12px 0">Connection name<input style="display:block;width:100%;padding:10px;margin-top:5px" name="displayName" required placeholder="e.g. Product Jira"></label><label style="display:block;margin:12px 0">Jira site URL<input style="display:block;width:100%;padding:10px;margin-top:5px" name="baseUrl" type="url" required placeholder="https://company.atlassian.net"></label><label style="display:block;margin:12px 0">JQL filter<input style="display:block;width:100%;padding:10px;margin-top:5px" name="jql" placeholder="project = DEMO ORDER BY updated DESC"></label><label style="display:block;margin:12px 0">OAuth client ID<input style="display:block;width:100%;padding:10px;margin-top:5px" name="clientId" required autocomplete="off"></label><label style="display:block;margin:12px 0">OAuth client secret<input style="display:block;width:100%;padding:10px;margin-top:5px" name="clientSecret" type="password" required autocomplete="new-password"></label><p class="description">The first manual import reads at most 25 recent matching issues. The client credentials are encrypted immediately.</p><button>Save encrypted draft</button></form></details>
<div class="heading" style="margin-top:38px"><div><h1>Placement inbox</h1><p>Review where imported work belongs in the goal tree.</p></div><span class="count">{{len .Items}} items</span></div>
{{if .CoreError}}<div class="error">The Work Graph tree is temporarily unavailable: {{.CoreError}}</div>{{end}}
{{range .Items}}<article class="card {{if eq .Status "placed"}}placed{{end}}"><div class="meta"><span class="badge {{.Status}}">{{.Provider}} · {{.Status}}</span><span class="external">{{.ExternalID}} · v{{.ExternalVersion}}</span></div><h2>{{.Title}}</h2>{{if .Description}}<p class="description">{{.Description}}</p>{{end}}
{{if .WorkGraphNodeID}}<div class="actions"><span>Added to the native tree.</span>{{if .NativeURL}}<a class="native-link" href="{{.NativeURL}}">Show task in tree →</a>{{end}}</div>
{{else if eq .Status "retrying"}}<div class="retry-panel"><strong>Placement will retry automatically</strong><div>Failed attempts: {{.AttemptCount}} of {{.MaxAttempts}}{{if .NextAttemptAt}} · next attempt {{.NextAttemptAt}}{{end}}</div>{{if .LastError}}<div>{{.LastError}}</div>{{end}}<form class="actions" method="post" action="/api/v1/candidates/{{.ID}}/retry"><button>Retry now</button></form></div>
{{else if .Options}}{{if eq .Status "dead_letter"}}<div class="error"><strong>Automatic placement stopped after {{.AttemptCount}} attempts.</strong> Review the error and select a branch to try again.</div>{{end}}<form method="post" action="/api/v1/candidates/{{.ID}}/place">
{{range .Recommendations}}<div class="suggestion"><label class="choice"><input type="radio" name="parentId" value="{{.ParentID}}" required><span><strong>{{.ParentTitle}} <span class="confidence">{{.Confidence}} · score {{.Score}}</span></strong><span class="path">{{range $index,$part:=.Path}}{{if $index}} / {{end}}{{$part}}{{end}}</span><span class="reason">{{range $index,$reason:=.Reasons}}{{if $index}} · {{end}}{{$reason}}{{end}}</span></span></label></div>{{end}}
<details {{if not .Recommendations}}open{{end}}><summary>{{if .Recommendations}}Choose another branch{{else}}Choose a branch manually{{end}}</summary><div class="tree-picker">{{range .Options}}<div class="tree-row"><label title="{{.Path}}"><input type="radio" name="parentId" value="{{.ID}}" required><span class="prefix">{{.Prefix}}</span><span class="tree-title">{{.Title}}</span>{{if .Recommended}}<span class="recommended-mark">suggested</span>{{end}}<span class="status">{{.Status}}</span></label><a class="open-tree" href="{{.TreeURL}}" title="Show this branch in Work Graph">↗</a></div>{{end}}</div></details><div class="actions"><button>Place in selected branch</button></div></form>
{{else}}<p class="description">The tree could not be loaded, so placement is paused.</p>{{end}}{{if .LastError}}<p class="last-error">{{.LastError}}</p>{{end}}</article>{{else}}<div class="empty">No imported work is waiting for placement.</div>{{end}}
<div class="heading" style="margin-top:38px"><div><h1>Native change journal</h1><p>Changes ready for a future provider adapter. Gateway-originated revisions are suppressed.</p></div><span class="count">{{len .OutboundEvents}} events</span></div>
{{range .OutboundEvents}}<article class="card"><div class="meta"><span class="badge {{.ProjectionStatus}}">{{.ProjectionStatus}}</span><span class="external">{{.Provider}} · {{.ExternalID}} · revision {{.WorkspaceRevision}}</span></div><h2>{{.EventType}}</h2><p class="description">{{if .ActorName}}Changed by {{.ActorName}} · {{end}}{{.OccurredAt}}{{if .SuppressionReason}} · {{.SuppressionReason}}{{end}}</p></article>{{else}}<div class="empty">No mapped native changes have been observed yet.</div>{{end}}</main></body></html>`))

func (h *Handler) inbox(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT `+candidateColumns+` FROM gateway_candidates ORDER BY CASE WHEN status='placed' THEN 1 ELSE 0 END,updated_at DESC`)
	if err != nil {
		h.internal(w, "load inbox", err)
		return
	}
	defer rows.Close()
	candidates := []Candidate{}
	for rows.Next() {
		item, scanErr := scanCandidate(rows)
		if scanErr != nil {
			h.internal(w, "scan inbox", scanErr)
			return
		}
		candidates = append(candidates, item)
	}
	workspaceID, nodes, coreErr := h.coreNodes(r.Context())
	options := buildTreeOptions(nodes, h.config.PublicCoreURL, workspaceID)
	page := inboxPage{Items: make([]inboxCandidate, 0, len(candidates))}
	page.Installations, err = h.installations(r)
	if err != nil {
		h.internal(w, "load gateway installations", err)
		return
	}
	page.OutboundEvents, err = h.outboundEvents(r.Context(), 20)
	if err != nil {
		h.internal(w, "load outbound journal", err)
		return
	}
	if coreErr != nil {
		page.CoreError = coreErr.Error()
	}
	for _, candidate := range candidates {
		recommended := map[string]bool{}
		for _, recommendation := range candidate.Recommendations {
			recommended[recommendation.ParentID] = true
		}
		candidateOptions := append([]treeOption(nil), options...)
		for index := range candidateOptions {
			candidateOptions[index].Recommended = recommended[candidateOptions[index].ID]
		}
		view := inboxCandidate{Candidate: candidate, Options: candidateOptions}
		if candidate.WorkGraphNodeID != nil {
			view.NativeURL = nativeTreeURL(h.config.PublicCoreURL, workspaceID, *candidate.WorkGraphNodeID)
		}
		page.Items = append(page.Items, view)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = treeInboxTemplate.Execute(w, page)
}

func buildTreeOptions(nodes []CoreNode, publicURL, workspaceID string) []treeOption {
	children := map[string][]CoreNode{}
	for _, node := range nodes {
		parent := ""
		if node.ParentID != nil {
			parent = *node.ParentID
		}
		children[parent] = append(children[parent], node)
	}
	for key := range children {
		sort.SliceStable(children[key], func(i, j int) bool { return children[key][i].Title < children[key][j].Title })
	}
	result := []treeOption{}
	var visit func(string, int, []string)
	visit = func(parent string, depth int, path []string) {
		for _, node := range children[parent] {
			nodePath := append(append([]string(nil), path...), node.Title)
			prefix := ""
			if depth > 0 {
				prefix = strings.Repeat("│  ", depth-1) + "├─ "
			}
			result = append(result, treeOption{ID: node.ID, Title: node.Title, Path: strings.Join(nodePath, " / "), Prefix: prefix, Status: node.Status, TreeURL: nativeTreeURL(publicURL, workspaceID, node.ID)})
			visit(node.ID, depth+1, nodePath)
		}
	}
	visit("", 0, nil)
	return result
}

func nativeTreeURL(publicURL, workspaceID, nodeID string) string {
	if strings.TrimSpace(publicURL) == "" || nodeID == "" {
		return ""
	}
	return strings.TrimRight(publicURL, "/") + "/?view=structure&workspace=" + url.QueryEscape(workspaceID) + "&node=" + url.QueryEscape(nodeID)
}
