#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

export COMPOSE_PROJECT_NAME="work-graph-backup-test"
export RESTORE_SERVICES="api"
temporary_dir="$(mktemp -d)"
export BACKUP_DIR="$temporary_dir"

cleanup() {
  docker compose down -v >/dev/null 2>&1 || true
  rm -rf "$temporary_dir"
}
trap cleanup EXIT

docker compose up -d --build postgres api >/dev/null
latest_schema="$(find internal/database/migrations -maxdepth 1 -name '*.sql' -type f -exec basename {} \; | sort | tail -n 1)"
for _ in $(seq 1 40); do
  if docker compose exec -T postgres pg_isready -U workgraph -d workgraph >/dev/null 2>&1 && \
     docker compose exec -T postgres psql -U workgraph -d workgraph -Atqc "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='$latest_schema')" 2>/dev/null | grep -q t; then
    break
  fi
  sleep 1
done

docker compose exec -T postgres psql -U workgraph -d workgraph -v ON_ERROR_STOP=1 \
  -c "INSERT INTO workspaces(name) VALUES ('Backup verification marker');" >/dev/null

archive="$temporary_dir/verification.tar.gz"
BACKUP_DIR="$temporary_dir" "$script_dir/backup.sh" "$archive" >/dev/null

docker compose exec -T postgres psql -U workgraph -d workgraph -v ON_ERROR_STOP=1 \
  -c "UPDATE workspaces SET name='Changed after backup' WHERE name='Backup verification marker';" >/dev/null

RESTORE_CONFIRM=replace-data "$script_dir/restore.sh" "$archive" >/dev/null

restored_name="$(docker compose exec -T postgres psql -U workgraph -d workgraph -Atqc "SELECT name FROM workspaces WHERE name='Backup verification marker'")"
if [[ "$restored_name" != "Backup verification marker" ]]; then
  echo "Backup/restore verification failed." >&2
  exit 1
fi

echo "Backup/restore verification passed."
