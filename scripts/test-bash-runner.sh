#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

export COMPOSE_PROJECT_NAME="work-graph-bash-test"
export WORK_GRAPH_PORT="0"
export WORK_GRAPH_MASTER_KEY="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
export WORK_GRAPH_REGISTRATION_MODE="open"
export RUNNER_TOKEN="bash-runner-verification-token"
temporary_dir="$(mktemp -d)"
cookie_jar="$temporary_dir/cookies"

cleanup() {
  docker compose down -v >/dev/null 2>&1 || true
  rm -rf "$temporary_dir"
}
trap cleanup EXIT

docker compose --profile bash-automation up -d --build postgres api web bash-runner >/dev/null
published="$(docker compose port web 80)"
port="${published##*:}"
base_url="http://127.0.0.1:$port"

for _ in $(seq 1 60); do
  curl -fsS "$base_url/api/v1/ready" >/dev/null 2>&1 && break
  sleep 1
done

curl -fsS -c "$cookie_jar" -H 'Content-Type: application/json' \
  -d '{"displayName":"Runner Test","email":"runner-test@workgraph.local","password":"runner-test-password"}' \
  "$base_url/api/v1/auth/register" >/dev/null
created="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d '{"name":"Bash runner verification","rootTitle":"Verify isolated automation"}' \
  "$base_url/api/v1/workspaces")"
workspace_id="$(jq -r '.workspace.id' <<<"$created")"
root_id="$(jq -r '.root.id' <<<"$created")"
curl -fsS -b "$cookie_jar" -X PATCH -H 'Content-Type: application/json' \
  -d '{"enabled":true,"publisherTrusted":true,"allowedHosts":[],"allowSecrets":false}' \
  "$base_url/api/v1/workspaces/$workspace_id/module-installations/builtin.bash/0.1.0" >/dev/null
node="$(curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg parentId "$root_id" '{parentId:$parentId,title:"Run safe script",desiredOutcome:"Output and exit code are preserved"}')" \
  "$base_url/api/v1/workspaces/$workspace_id/nodes")"
node_id="$(jq -r '.id' <<<"$node")"
curl -fsS -b "$cookie_jar" -H 'Content-Type: application/json' \
  -d '{"name":"Generate output","stepType":"script","moduleId":"builtin.bash","moduleVersion":"0.1.0","configuration":{"script":"printf verified","timeoutSeconds":5}}' \
  "$base_url/api/v1/workspaces/$workspace_id/nodes/$node_id/workflow-steps" >/dev/null

for _ in $(seq 1 30); do
  circle="$(curl -fsS -b "$cookie_jar" "$base_url/api/v1/workspaces/$workspace_id/nodes/$node_id/circle")"
  status="$(jq -r '.workflowSteps[0].executions[0].executionStatus // empty' <<<"$circle")"
  [[ "$status" == "succeeded" ]] && break
  sleep 1
done

if [[ "$status" != "succeeded" ]] || ! jq -e '.workflowSteps[0].executions[0].result.stdout=="verified" and .workflowSteps[0].executions[0].result.exitCode==0' <<<"$circle" >/dev/null; then
  echo "The Bash runner did not preserve a successful isolated execution." >&2
  docker compose logs bash-runner api >&2
  exit 1
fi

echo "Bash runner verification passed in an isolated deployment."
