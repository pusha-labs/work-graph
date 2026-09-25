#!/usr/bin/env bash
set -euo pipefail

base_url="${WORK_GRAPH_URL:-http://localhost:8088}"
demo_email="${DEMO_EMAIL:-demo@workgraph.local}"
demo_name="${DEMO_NAME:-Demo Owner}"
workspace_name="${DEMO_WORKSPACE_NAME:-Work Graph Demo}"
cookie_jar="$(mktemp)"
trap 'rm -f "$cookie_jar"' EXIT

auth_args=(-b "$cookie_jar" -c "$cookie_jar")
if [[ -n "${DEMO_SESSION_TOKEN:-}" ]]; then
  auth_args=(-H "Cookie: work_graph_session=${DEMO_SESSION_TOKEN}")
fi

api_get() {
  curl -fsS "${auth_args[@]}" "$base_url$1"
}

api_post() {
  curl -fsS "${auth_args[@]}" -H 'Content-Type: application/json' -d "$2" "$base_url$1"
}

auth_status="$(api_get /api/v1/auth/status)"
if [[ "$(jq -r '.authenticated' <<<"$auth_status")" != "true" ]]; then
  if [[ -z "${DEMO_PASSWORD:-}" ]]; then
    echo "DEMO_PASSWORD is required when no authenticated DEMO_SESSION_TOKEN is supplied." >&2
    exit 1
  fi
  if [[ "$(jq -r '.setupRequired' <<<"$auth_status")" == "true" ]]; then
    api_post /api/v1/auth/register "$(jq -cn --arg displayName "$demo_name" --arg email "$demo_email" --arg password "$DEMO_PASSWORD" '{displayName:$displayName,email:$email,password:$password}')" >/dev/null
  else
    api_post /api/v1/auth/login "$(jq -cn --arg email "$demo_email" --arg password "$DEMO_PASSWORD" '{email:$email,password:$password}')" >/dev/null
  fi
fi

existing="$(api_get /api/v1/workspaces | jq -r --arg name "$workspace_name" '.[] | select(.name==$name) | .id' | head -n 1)"
if [[ -n "$existing" ]]; then
  echo "Demo workspace already exists: $workspace_name ($existing)"
  exit 0
fi

created="$(api_post /api/v1/workspaces "$(jq -cn --arg name "$workspace_name" --arg rootTitle 'Build a demonstration house' '{name:$name,rootTitle:$rootTitle}')")"
workspace_id="$(jq -r '.workspace.id' <<<"$created")"
root_id="$(jq -r '.root.id' <<<"$created")"

create_node() {
  local title="$1" outcome="$2" parent_id="$3"
  api_post "/api/v1/workspaces/$workspace_id/nodes" "$(jq -cn --arg title "$title" --arg desiredOutcome "$outcome" --arg parentId "$parent_id" '{title:$title,desiredOutcome:$desiredOutcome,parentId:$parentId}')"
}

create_capability() {
  local name="$1" kind="$2"
  api_post "/api/v1/workspaces/$workspace_id/capabilities" "$(jq -cn --arg name "$name" --arg capabilityType "$kind" '{name:$name,capabilityType:$capabilityType}')"
}

create_subject() {
  local name="$1" kind="$2"
  api_post "/api/v1/workspaces/$workspace_id/knowledge-subjects" "$(jq -cn --arg name "$name" --arg subjectType "$kind" '{name:$name,subjectType:$subjectType}')"
}

plan="$(create_node 'Plan the house' 'An approved design, budget, and construction sequence exist.' "$root_id")"
site="$(create_node 'Prepare the site' 'The site is cleared, measured, and ready for foundation work.' "$root_id")"
foundation="$(create_node 'Build the foundation' 'A cured and inspected foundation is ready for structural work.' "$(jq -r '.id' <<<"$site")")"
utilities="$(create_node 'Install underground utilities' 'Water, drainage, and electrical entries are tested before backfill.' "$(jq -r '.id' <<<"$site")")"
inspection="$(create_node 'Perform final inspection' 'The completed house passes the final quality and safety review.' "$root_id")"

manager="$(create_capability 'Project manager' role)"
engineer="$(create_capability 'Civil engineer' role)"
concrete="$(create_capability 'Concrete work' skill)"
inspector="$(create_capability 'Quality inspection' skill)"
permit_specialist="$(create_capability 'Local permit specialist' skill)"
house_subject="$(create_subject 'Demo House' project)"
water_subject="$(create_subject 'Water system' system)"
permit_subject="$(create_subject 'Local permit process' domain)"

directory="$(api_get "/api/v1/workspaces/$workspace_id/directory")"
owner_actor_id="$(jq -r '.actors[] | select(.hasAccount==true) | .id' <<<"$directory" | head -n 1)"

create_actor() {
  local name="$1"
  api_post "/api/v1/workspaces/$workspace_id/actors" "$(jq -cn --arg displayName "$name" '{displayName:$displayName,actorType:"person"}')"
}

elena="$(create_actor 'Elena · Project manager')"
sam="$(create_actor 'Sam · Civil engineer')"
quinn="$(create_actor 'Quinn · Quality inspector')"

for capability_id in "$(jq -r '.id' <<<"$manager")" "$(jq -r '.id' <<<"$engineer")" "$(jq -r '.id' <<<"$concrete")" "$(jq -r '.id' <<<"$inspector")"; do
  api_post "/api/v1/workspaces/$workspace_id/actors/$owner_actor_id/capabilities" "$(jq -cn --arg capabilityId "$capability_id" '{capabilityId:$capabilityId,claimSource:"admin"}')" >/dev/null
done

assign_capability() {
  local actor_id="$1" capability_id="$2"
  api_post "/api/v1/workspaces/$workspace_id/actors/$actor_id/capabilities" "$(jq -cn --arg capabilityId "$capability_id" '{capabilityId:$capabilityId,claimSource:"admin"}')" >/dev/null
}

assign_capability "$(jq -r '.id' <<<"$elena")" "$(jq -r '.id' <<<"$manager")"
assign_capability "$(jq -r '.id' <<<"$sam")" "$(jq -r '.id' <<<"$engineer")"
assign_capability "$(jq -r '.id' <<<"$sam")" "$(jq -r '.id' <<<"$concrete")"
assign_capability "$(jq -r '.id' <<<"$quinn")" "$(jq -r '.id' <<<"$inspector")"

for subject_id in "$(jq -r '.id' <<<"$house_subject")" "$(jq -r '.id' <<<"$water_subject")"; do
  api_post "/api/v1/workspaces/$workspace_id/actors/$owner_actor_id/knowledge" "$(jq -cn --arg subjectId "$subject_id" '{subjectId:$subjectId}')" >/dev/null
done

for actor_id in "$(jq -r '.id' <<<"$elena")" "$(jq -r '.id' <<<"$sam")"; do
  api_post "/api/v1/workspaces/$workspace_id/actors/$actor_id/knowledge" "$(jq -cn --arg subjectId "$(jq -r '.id' <<<"$house_subject")" '{subjectId:$subjectId}')" >/dev/null
done
api_post "/api/v1/workspaces/$workspace_id/actors/$(jq -r '.id' <<<"$sam")/knowledge" "$(jq -cn --arg subjectId "$(jq -r '.id' <<<"$water_subject")" '{subjectId:$subjectId}')" >/dev/null
# Keep this subject with one holder to demonstrate a continuity-risk diagnostic.
api_post "/api/v1/workspaces/$workspace_id/actors/$(jq -r '.id' <<<"$quinn")/knowledge" "$(jq -cn --arg subjectId "$(jq -r '.id' <<<"$permit_subject")" '{subjectId:$subjectId}')" >/dev/null

add_requirement() {
  local node_id="$1" capability_id="$2"
  api_post "/api/v1/workspaces/$workspace_id/nodes/$node_id/requirements" "$(jq -cn --arg capabilityId "$capability_id" '{capabilityId:$capabilityId}')" >/dev/null
}

add_knowledge() {
  local node_id="$1" subject_id="$2"
  api_post "/api/v1/workspaces/$workspace_id/nodes/$node_id/knowledge-requirements" "$(jq -cn --arg subjectId "$subject_id" '{subjectId:$subjectId}')" >/dev/null
}

add_requirement "$(jq -r '.id' <<<"$plan")" "$(jq -r '.id' <<<"$manager")"
add_knowledge "$(jq -r '.id' <<<"$plan")" "$(jq -r '.id' <<<"$house_subject")"
add_requirement "$(jq -r '.id' <<<"$foundation")" "$(jq -r '.id' <<<"$concrete")"
add_knowledge "$(jq -r '.id' <<<"$foundation")" "$(jq -r '.id' <<<"$house_subject")"
add_requirement "$(jq -r '.id' <<<"$utilities")" "$(jq -r '.id' <<<"$engineer")"
add_knowledge "$(jq -r '.id' <<<"$utilities")" "$(jq -r '.id' <<<"$water_subject")"
add_requirement "$(jq -r '.id' <<<"$inspection")" "$(jq -r '.id' <<<"$inspector")"
# Deliberately leave this capability unassigned so contributors can see and resolve
# the explainable "No eligible performer" diagnostic in the tree.
add_requirement "$(jq -r '.id' <<<"$inspection")" "$(jq -r '.id' <<<"$permit_specialist")"
add_knowledge "$(jq -r '.id' <<<"$inspection")" "$(jq -r '.id' <<<"$permit_subject")"

api_post "/api/v1/workspaces/$workspace_id/nodes/$(jq -r '.id' <<<"$foundation")/workflow-steps" "$(jq -cn --arg name 'Engineering review' --arg capabilityId "$(jq -r '.id' <<<"$engineer")" --arg subjectId "$(jq -r '.id' <<<"$house_subject")" '{name:$name,capabilityId:$capabilityId,subjectId:$subjectId}')" >/dev/null

if critical_until="$(date -u -v+7d '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null)"; then :; else critical_until="$(date -u -d '+7 days' '+%Y-%m-%dT%H:%M:%SZ')"; fi
api_post "/api/v1/workspaces/$workspace_id/nodes/$(jq -r '.id' <<<"$utilities")/criticality" "$(jq -cn --arg reason 'Utility inspection slot is available only this week' --arg criticalUntil "$critical_until" '{reason:$reason,criticalUntil:$criticalUntil}')" >/dev/null

echo "Demo workspace created: $workspace_name"
echo "Workspace ID: $workspace_id"
echo "Open: $base_url"
