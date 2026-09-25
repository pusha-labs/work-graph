#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

export COMPOSE_PROJECT_NAME="work-graph-gateway-test"
export WORK_GRAPH_PORT="0"
export WORK_GRAPH_GATEWAY_PORT="0"
export WORK_GRAPH_MASTER_KEY="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
export GATEWAY_ADMIN_TOKEN="gateway-test-admin-token"
export GATEWAY_MASTER_KEY="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
demo_password="gateway-verification-password"
temporary_dir="$(mktemp -d)"
cookie_jar="$temporary_dir/cookies"
gateway_container="work-graph-gateway-test-runtime"
compose=(docker compose -f compose.yaml -f compose.test-gateway.yaml)

cleanup() {
  docker rm -f "$gateway_container" >/dev/null 2>&1 || true
  "${compose[@]}" down -v >/dev/null 2>&1 || true
  rm -rf "$temporary_dir"
}
trap cleanup EXIT
on_error() {
  local exit_code="$1"
  local line_number="$2"
  local failed_command="$3"
  echo "::error file=scripts/test-gateway.sh,line=${line_number}::Integration Gateway verification failed (exit ${exit_code}): ${failed_command}" >&2
  echo "Integration Gateway verification failed; service logs follow." >&2
  docker logs "$gateway_container" >&2 || true
  "${compose[@]}" logs --no-color gateway-postgres api web >&2 || true
}
trap 'on_error "$?" "$LINENO" "$BASH_COMMAND"' ERR

"${compose[@]}" up -d --build postgres api web >/dev/null
web_port="$("${compose[@]}" port web 80)"
core_url="http://127.0.0.1:${web_port##*:}"
for _ in $(seq 1 60); do
  curl -fsS "$core_url/api/v1/ready" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "$core_url/api/v1/ready" >/dev/null

DEMO_PASSWORD="$demo_password" DEMO_WORKSPACE_NAME="Gateway verification" WORK_GRAPH_URL="$core_url" "$script_dir/demo.sh" >/dev/null
curl -fsS -c "$cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg email 'demo@workgraph.local' --arg password "$demo_password" '{email:$email,password:$password}')" \
  "$core_url/api/v1/auth/login" >/dev/null
workspace_id="$(curl -fsS -b "$cookie_jar" "$core_url/api/v1/workspaces" | jq -r '.[0].id')"
credential="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d '{"name":"Gateway verification","scopes":["work:read","work:write"]}' \
  "$core_url/api/v1/workspaces/$workspace_id/service-accounts")"
export WORK_GRAPH_SERVICE_TOKEN="$(jq -r '.token' <<<"$credential")"
export WORK_GRAPH_WORKSPACE_ID="$workspace_id"

"${compose[@]}" exec -T postgres createdb -U workgraph gateway
"${compose[@]}" build gateway >/dev/null
gateway_image="work-graph-gateway-test:local"
docker run -d --rm --name "$gateway_container" --network "${COMPOSE_PROJECT_NAME}_default" --entrypoint work-graph-gateway \
  -p '127.0.0.1::8090' \
  -e 'GATEWAY_DATABASE_URL=postgres://workgraph:workgraph@postgres:5432/gateway?sslmode=disable' \
  -e 'GATEWAY_HTTP_ADDRESS=:8090' -e "GATEWAY_ADMIN_TOKEN=$GATEWAY_ADMIN_TOKEN" \
  -e "GATEWAY_MASTER_KEY=$GATEWAY_MASTER_KEY" \
  -e 'GATEWAY_INSTALLATION_KEY=mock-local' -e 'WORK_GRAPH_URL=http://web' -e "WORK_GRAPH_PUBLIC_URL=$core_url" \
  -e "WORK_GRAPH_WORKSPACE_ID=$WORK_GRAPH_WORKSPACE_ID" -e "WORK_GRAPH_SERVICE_TOKEN=$WORK_GRAPH_SERVICE_TOKEN" \
  "$gateway_image" >/dev/null
gateway_port="$(docker port "$gateway_container" 8090/tcp)"
gateway_url="http://127.0.0.1:${gateway_port##*:}"
for _ in $(seq 1 60); do
  curl -fsS "$gateway_url/ready" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "$gateway_url/ready" >/dev/null

installation="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"provider":"jira_cloud","installationKey":"jira-demo","displayName":"Demo Jira Cloud","baseUrl":"https://example.atlassian.net","authType":"oauth2","configuration":{"projectKeys":["DEMO"]},"credentials":{"clientId":"demo-client","clientSecret":"never-return-this-secret"}}' \
  "$gateway_url/api/v1/installations")"
installations="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" "$gateway_url/api/v1/installations")"
if [[ "$(jq -r '.hasCredentials' <<<"$installation")" != "true" || "$(jq -r '.[0].credentialVersion' <<<"$installations")" != "1" || "$installation$installations" == *"never-return-this-secret"* ]]; then
  echo "Gateway installation credentials were not stored and redacted correctly." >&2
  exit 1
fi

unauthorized="$(curl -sS -o /dev/null -w '%{http_code}' "$gateway_url/api/v1/candidates")"
if [[ "$unauthorized" != "401" ]]; then
  echo "Gateway API was accessible without its administrator token (HTTP $unauthorized)." >&2
  exit 1
fi

candidate="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"externalId":"MOCK-1","externalVersion":"1","title":"Prepare construction site utilities","description":"The ground and utilities are ready","rawPayload":{"source":"test"}}' \
  "$gateway_url/api/v1/mock/candidates")"
candidate_id="$(jq -r '.id' <<<"$candidate")"
parent_id="$(jq -r '.recommendations[0].parentId' <<<"$candidate")"
if [[ "$(jq -r '.status' <<<"$candidate")" != "suggested" || -z "$parent_id" || "$parent_id" == "null" ]]; then
  echo "Mock candidate did not enter the suggested placement inbox." >&2
  exit 1
fi
inbox_html="$(curl -fsS -u "admin:$GATEWAY_ADMIN_TOKEN" "$gateway_url/")"
if [[ "$inbox_html" != *"Demo Jira Cloud"* || "$inbox_html" == *"never-return-this-secret"* || "$inbox_html" != *"Choose another branch"* || "$inbox_html" != *"Prepare construction site utilities"* || "$inbox_html" != *"Build a demonstration house"* ]]; then
  echo "Gateway inbox did not render the candidate and complete tree picker." >&2
  exit 1
fi

placed="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg parent "$parent_id" '{parentId:$parent}')" \
  "$gateway_url/api/v1/candidates/$candidate_id/place")"
node_id="$(jq -r '.workGraphNodeId' <<<"$placed")"
if [[ "$(jq -r '.status' <<<"$placed")" != "placed" || -z "$node_id" || "$node_id" == "null" ]]; then
  echo "Confirmed gateway placement did not create a durable mapping: $placed" >&2
  exit 1
fi
node_matches="$(curl -fsS -H "Authorization: Bearer $WORK_GRAPH_SERVICE_TOKEN" "$core_url/api/v1/workspaces/$workspace_id/nodes" | jq --arg id "$node_id" '[.[] | select(.id==$id)] | length')"
if [[ "$node_matches" != "1" ]]; then
  echo "The gateway mapping does not reference exactly one native node." >&2
  exit 1
fi
native_node="$(curl -fsS -H "Authorization: Bearer $WORK_GRAPH_SERVICE_TOKEN" "$core_url/api/v1/workspaces/$workspace_id/nodes" | jq -c --arg id "$node_id" '.[]|select(.id==$id)')"
curl -fsS -b "$cookie_jar" -X PATCH -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg title 'Prepare construction site utilities · reviewed' --arg outcome "$(jq -r '.desiredOutcome' <<<"$native_node")" --arg status "$(jq -r '.lifecycleStatus' <<<"$native_node")" '{title:$title,desiredOutcome:$outcome,lifecycleStatus:$status}')" \
  "$core_url/api/v1/workspaces/$workspace_id/nodes/$node_id" >/dev/null
outbound_events='[]'
for _ in $(seq 1 12); do
  outbound_events="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" "$gateway_url/api/v1/outbound-events")"
  if jq -e --arg id "$node_id" 'any(.[]; .workGraphNodeId==$id and .eventType=="node.updated" and .projectionStatus=="pending")' <<<"$outbound_events" >/dev/null; then break; fi
  sleep 1
done
if ! jq -e --arg id "$node_id" 'any(.[]; .workGraphNodeId==$id and .eventType=="node.created" and .projectionStatus=="suppressed") and any(.[]; .workGraphNodeId==$id and .eventType=="node.updated" and .projectionStatus=="pending")' <<<"$outbound_events" >/dev/null; then
  echo "Gateway did not durably journal and classify mapped native events: $outbound_events" >&2
  docker logs "$gateway_container" >&2 || true
  exit 1
fi
placed_inbox_html="$(curl -fsS -u "admin:$GATEWAY_ADMIN_TOKEN" "$gateway_url/")"
if [[ "$placed_inbox_html" != *"Show task in tree"* || "$placed_inbox_html" != *"node=$node_id"* || "$placed_inbox_html" != *"Native change journal"* ]]; then
  echo "Gateway inbox did not link the placed item back to its native tree node." >&2
  exit 1
fi

retry_candidate="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"externalId":"MOCK-RETRY","externalVersion":"1","title":"Prepare construction site utilities retry test","description":"Exercise durable delivery failures","rawPayload":{"source":"test"}}' \
  "$gateway_url/api/v1/mock/candidates")"
retry_id="$(jq -r '.id' <<<"$retry_candidate")"
if [[ ! "$retry_id" =~ ^[0-9a-f-]{36}$ ]]; then
  echo "Gateway returned an invalid retry candidate id." >&2
  exit 1
fi
retry_response="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"parentId":"00000000-0000-0000-0000-000000000001"}' \
  "$gateway_url/api/v1/candidates/$retry_id/place")"
if [[ "$(jq -r '.status' <<<"$retry_response")" != "retrying" || "$(jq -r '.attemptCount' <<<"$retry_response")" != "1" || "$(jq -r '.nextAttemptAt' <<<"$retry_response")" == "null" ]]; then
  echo "Failed placement was not scheduled durably: $retry_response" >&2
  exit 1
fi
"${compose[@]}" exec -T postgres psql -U workgraph -d gateway -c "UPDATE gateway_candidates SET next_attempt_at=now() WHERE id='$retry_id'::uuid" >/dev/null
for _ in $(seq 1 10); do
  retry_response="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" "$gateway_url/api/v1/candidates" | jq -c --arg id "$retry_id" '.[]|select(.id==$id)')"
  [[ "$(jq -r '.attemptCount' <<<"$retry_response")" -ge 2 ]] && break
  sleep 1
done
if [[ "$(jq -r '.attemptCount' <<<"$retry_response")" -lt 2 ]]; then
  echo "Gateway retry worker did not process a due attempt: $retry_response" >&2
  exit 1
fi
"${compose[@]}" exec -T postgres psql -U workgraph -d gateway -c "UPDATE gateway_candidates SET max_attempts=attempt_count+1,next_attempt_at=now() WHERE id='$retry_id'::uuid" >/dev/null
for _ in $(seq 1 10); do
  retry_response="$(curl -fsS -H "Authorization: Bearer $GATEWAY_ADMIN_TOKEN" "$gateway_url/api/v1/candidates" | jq -c --arg id "$retry_id" '.[]|select(.id==$id)')"
  [[ "$(jq -r '.status' <<<"$retry_response")" == "dead_letter" ]] && break
  sleep 1
done
if [[ "$(jq -r '.status' <<<"$retry_response")" != "dead_letter" || "$(jq -r '.deadLetteredAt' <<<"$retry_response")" == "null" ]]; then
  echo "Exhausted placement did not enter the dead-letter inbox: $retry_response" >&2
  exit 1
fi
dead_letter_html="$(curl -fsS -u "admin:$GATEWAY_ADMIN_TOKEN" "$gateway_url/")"
if [[ "$dead_letter_html" != *"Automatic placement stopped"* ]]; then
  echo "Gateway inbox did not explain the dead-letter state." >&2
  exit 1
fi

echo "Integration Gateway verification passed on isolated ports."
