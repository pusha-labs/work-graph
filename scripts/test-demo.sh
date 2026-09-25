#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

export COMPOSE_PROJECT_NAME="work-graph-demo-test"
export WORK_GRAPH_PORT="0"
export WORK_GRAPH_MASTER_KEY="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
export WORK_GRAPH_REGISTRATION_MODE="open"
demo_password="demo-verification-password"
demo_workspace="Demo verification workspace"
temporary_dir="$(mktemp -d)"
cookie_jar="$temporary_dir/cookies"
member_cookie_jar="$temporary_dir/member-cookies"

cleanup() {
  docker compose down -v >/dev/null 2>&1 || true
  rm -rf "$temporary_dir"
}
trap cleanup EXIT

docker compose up -d --build postgres api web >/dev/null
published="$(docker compose port web 80)"
port="${published##*:}"
if [[ -z "$port" || "$port" == "$published" ]]; then
  echo "Could not determine the isolated demo port." >&2
  exit 1
fi
base_url="http://127.0.0.1:$port"

ready=false
for _ in $(seq 1 60); do
  if curl -fsS "$base_url/api/v1/ready" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != "true" ]]; then
  echo "The isolated demo deployment did not become ready." >&2
  exit 1
fi

DEMO_PASSWORD="$demo_password" DEMO_WORKSPACE_NAME="$demo_workspace" WORK_GRAPH_URL="$base_url" "$script_dir/demo.sh" >/dev/null
# The seed must be safe to invoke repeatedly against a long-running public demo.
DEMO_PASSWORD="$demo_password" DEMO_WORKSPACE_NAME="$demo_workspace" WORK_GRAPH_URL="$base_url" "$script_dir/demo.sh" >/dev/null

curl -fsS -c "$cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg email 'demo@workgraph.local' --arg password "$demo_password" '{email:$email,password:$password}')" \
  "$base_url/api/v1/auth/login" >/dev/null

workspaces="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces")"
workspace_count="$(jq --arg name "$demo_workspace" '[.[] | select(.name==$name)] | length' <<<"$workspaces")"
workspace_id="$(jq -r --arg name "$demo_workspace" '.[] | select(.name==$name) | .id' <<<"$workspaces")"
if [[ "$workspace_count" != "1" || -z "$workspace_id" ]]; then
  echo "Demo seeding did not produce exactly one expected workspace." >&2
  exit 1
fi

# An independently registered account must be able to accept an invitation to
# an existing workspace without creating a second account.
member_email="member@workgraph.local"
member_password="member-verification-password"
invitation_response="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg email "$member_email" '{email:$email,displayName:"Demo Member",workspaceRole:"member"}')" \
  "$base_url/api/v1/workspaces/$workspace_id/invitations")"
invitation_token="$(jq -r '.token' <<<"$invitation_response")"
curl -fsS -c "$member_cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg displayName 'Demo Member' --arg email "$member_email" --arg password "$member_password" '{displayName:$displayName,email:$email,password:$password}')" \
  "$base_url/api/v1/auth/register" >/dev/null
curl -fsS -b "$member_cookie_jar" -c "$member_cookie_jar" -H 'Content-Type: application/json' -d '{}' \
  "$base_url/api/v1/auth/invitations/$invitation_token/accept" >/dev/null
member_workspace_count="$(curl -fsS -b "$member_cookie_jar" "$base_url/api/v1/workspaces" | jq --arg id "$workspace_id" '[.[] | select(.id==$id)] | length')"
if [[ "$member_workspace_count" != "1" ]]; then
  echo "An existing account could not join the invited workspace." >&2
  exit 1
fi
member_actor_id="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces/$workspace_id/directory" | jq -r '.actors[] | select(.displayName=="Demo Member" and .hasAccount==true) | .id')"
curl -fsS -b "$cookie_jar" -X PATCH -H 'Content-Type: application/json' -d '{"workspaceRole":"admin"}' \
  "$base_url/api/v1/workspaces/$workspace_id/actors/$member_actor_id/membership" >/dev/null
curl -fsS -b "$cookie_jar" -X PATCH -H 'Content-Type: application/json' -d '{"workspaceRole":"member"}' \
  "$base_url/api/v1/workspaces/$workspace_id/actors/$member_actor_id/membership" >/dev/null
curl -fsS -b "$cookie_jar" -X DELETE \
  "$base_url/api/v1/workspaces/$workspace_id/actors/$member_actor_id/membership" >/dev/null
if jq -e --arg id "$workspace_id" 'any(.id==$id)' <<<"$(curl -fsS -b "$member_cookie_jar" "$base_url/api/v1/workspaces")" >/dev/null; then
  echo "A removed member retained workspace access." >&2
  exit 1
fi
replacement_invitation="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg email "$member_email" '{email:$email,displayName:"Demo Member",workspaceRole:"member"}')" \
  "$base_url/api/v1/workspaces/$workspace_id/invitations")"
replacement_token="$(jq -r '.token' <<<"$replacement_invitation")"
curl -fsS -b "$member_cookie_jar" -H 'Content-Type: application/json' -d '{}' \
  "$base_url/api/v1/auth/invitations/$replacement_token/accept" >/dev/null
restored_actor_id="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces/$workspace_id/directory" | jq -r '.actors[] | select(.displayName=="Demo Member" and .hasAccount==true) | .id')"
if [[ "$restored_actor_id" != "$member_actor_id" ]]; then
  echo "Re-inviting a removed member did not restore the preserved actor." >&2
  exit 1
fi

node_count="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces/$workspace_id/nodes" | jq 'length')"
if (( node_count < 6 )); then
  echo "The demo tree is incomplete: expected at least 6 nodes, found $node_count." >&2
  exit 1
fi

diagnostics="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces/$workspace_id/diagnostics")"
for expected_kind in no_eligible_performer concentrated_knowledge; do
  if ! jq -e --arg kind "$expected_kind" 'any(.kind==$kind)' <<<"$diagnostics" >/dev/null; then
    echo "The demo is missing the $expected_kind example." >&2
    exit 1
  fi
done

service_credential="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d '{"name":"Demo gateway","scopes":["work:read","work:write"]}' \
  "$base_url/api/v1/workspaces/$workspace_id/service-accounts")"
service_token="$(jq -r '.token' <<<"$service_credential")"
service_account_id="$(jq -r '.serviceAccount.id' <<<"$service_credential")"
if [[ "$service_token" != wgsa_* || -z "$service_account_id" ]]; then
  echo "Service account creation did not return a usable one-time token." >&2
  exit 1
fi

service_workspaces="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces")"
if ! jq -e --arg id "$workspace_id" 'length==1 and .[0].id==$id' <<<"$service_workspaces" >/dev/null; then
  echo "The service account could not discover its own workspace." >&2
  exit 1
fi
workspace_revision="$(jq -r '.[0].revision' <<<"$service_workspaces")"
baseline_events="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/events?limit=500")"
event_cursor="$(jq -r '.nextCursor' <<<"$baseline_events")"
placement="$(curl -fsS -H "Authorization: Bearer $service_token" -H 'Content-Type: application/json' \
  -d '{"title":"Prepare construction site utilities","desiredOutcome":"The ground and utilities are ready","capabilityIds":[],"knowledgeSubjectIds":[]}' \
  "$base_url/api/v1/workspaces/$workspace_id/placement-recommendations")"
if ! jq -e '.workspaceRevision > 0 and (.recommendations | length) > 0 and .recommendations[0].score > 0 and (.recommendations[0].reasons | length) > 0' <<<"$placement" >/dev/null; then
  echo "Placement recommendations did not return ranked, explained evidence." >&2
  exit 1
fi
root_id="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/nodes" | jq -r '.[] | select(.parentId==null) | .id' | head -n 1)"
gateway_command="$(jq -cn --arg parent "$root_id" '{parentId:$parent,title:"Imported gateway task",desiredOutcome:"Verify scoped integration writes"}')"
gateway_node="$(curl -fsS -H "Authorization: Bearer $service_token" -H "If-Match: \"$workspace_revision\"" -H 'Content-Type: application/json' -H 'Idempotency-Key: jira:DEMO-1:create' \
  -d "$gateway_command" "$base_url/api/v1/workspaces/$workspace_id/nodes")"
gateway_node_replay="$(curl -fsS -H "Authorization: Bearer $service_token" -H "If-Match: \"$workspace_revision\"" -H 'Content-Type: application/json' -H 'Idempotency-Key: jira:DEMO-1:create' \
  -d "$gateway_command" "$base_url/api/v1/workspaces/$workspace_id/nodes")"
if [[ "$(jq -r '.id' <<<"$gateway_node")" != "$(jq -r '.id' <<<"$gateway_node_replay")" ]]; then
  echo "Replaying an integration command created a duplicate node." >&2
  exit 1
fi
gateway_node_count="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/nodes" | jq '[.[] | select(.title=="Imported gateway task")] | length')"
if [[ "$gateway_node_count" != "1" ]]; then
  echo "Expected one imported gateway node after replay, found $gateway_node_count." >&2
  exit 1
fi
conflict_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $service_token" -H "If-Match: \"$workspace_revision\"" -H 'Content-Type: application/json' -H 'Idempotency-Key: jira:DEMO-1:create' \
  -d "$(jq -cn --arg parent "$root_id" '{parentId:$parent,title:"Different task",desiredOutcome:"Must conflict"}')" "$base_url/api/v1/workspaces/$workspace_id/nodes")"
if [[ "$conflict_status" != "409" ]]; then
  echo "Reusing an idempotency key with different content did not conflict (HTTP $conflict_status)." >&2
  exit 1
fi
stale_status="$(curl -sS -o "$temporary_dir/stale.json" -w '%{http_code}' -H "Authorization: Bearer $service_token" -H "If-Match: \"$workspace_revision\"" -H 'Content-Type: application/json' -H 'Idempotency-Key: jira:DEMO-2:create' \
  -d "$(jq -cn --arg parent "$root_id" '{parentId:$parent,title:"Stale gateway task",desiredOutcome:"Must require a fresh read"}')" "$base_url/api/v1/workspaces/$workspace_id/nodes")"
if [[ "$stale_status" != "412" || "$(jq -r '.currentRevision' "$temporary_dir/stale.json")" != "$((workspace_revision + 1))" ]]; then
  echo "A stale integration command was not rejected with the current revision (HTTP $stale_status)." >&2
  exit 1
fi
committed_events="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/events?after=$event_cursor&limit=10")"
gateway_node_id="$(jq -r '.id' <<<"$gateway_node")"
if ! jq -e --arg id "$gateway_node_id" '(.events | length)==1 and .events[0].eventType=="node.created" and .events[0].aggregateId==$id and .events[0].workspaceRevision>0 and .events[0].actorName=="Demo gateway" and (.events[0].correlationId|length)>0' <<<"$committed_events" >/dev/null; then
  echo "The committed event feed did not expose the imported node with revision and actor attribution." >&2
  exit 1
fi
replayed_events="$(curl -fsS -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/events?after=$event_cursor&limit=10")"
if [[ "$(jq -r '.events[0].id' <<<"$committed_events")" != "$(jq -r '.events[0].id' <<<"$replayed_events")" ]]; then
  echo "Reading from the same durable cursor did not replay the same event." >&2
  exit 1
fi

curl -fsS -b "$cookie_jar" -X DELETE "$base_url/api/v1/workspaces/$workspace_id/service-accounts/$service_account_id" >/dev/null
revoked_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $service_token" "$base_url/api/v1/workspaces/$workspace_id/nodes")"
if [[ "$revoked_status" != "401" ]]; then
  echo "A revoked service token remained usable (HTTP $revoked_status)." >&2
  exit 1
fi

for _ in $(seq 1 8); do
  failed_login_status="$(curl -sS -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"email":"rate-limit@workgraph.local","password":"incorrect-password"}' "$base_url/api/v1/auth/login")"
  if [[ "$failed_login_status" != "401" ]]; then
    echo "Expected a rejected login before the rate limit, received HTTP $failed_login_status." >&2
    exit 1
  fi
done
limited_login_status="$(curl -sS -D "$temporary_dir/rate-limit-headers" -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"email":"rate-limit@workgraph.local","password":"incorrect-password"}' "$base_url/api/v1/auth/login")"
if [[ "$limited_login_status" != "429" ]] || ! grep -qi '^Retry-After:' "$temporary_dir/rate-limit-headers"; then
  echo "Repeated login attempts were not rate limited with a Retry-After response." >&2
  exit 1
fi

echo "Demo deployment verification passed on an isolated port."
