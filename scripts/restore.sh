#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

archive_path="${1:-}"
postgres_service="${POSTGRES_SERVICE:-postgres}"
database_name="${POSTGRES_DB:-workgraph}"
database_user="${POSTGRES_USER:-workgraph}"
restore_services="${RESTORE_SERVICES:-api web}"

if [[ ! "$database_name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$database_user" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
  echo "POSTGRES_DB and POSTGRES_USER must be simple PostgreSQL identifiers." >&2
  exit 1
fi
read -r -a restore_service_list <<<"$restore_services"
if [[ ${#restore_service_list[@]} -eq 0 ]]; then
  echo "RESTORE_SERVICES must contain at least one Compose service." >&2
  exit 1
fi
for service in "${restore_service_list[@]}"; do
  if [[ ! "$service" =~ ^[A-Za-z0-9_-]+$ ]]; then
    echo "Invalid service name in RESTORE_SERVICES: $service" >&2
    exit 1
  fi
done

if [[ -z "$archive_path" ]]; then
  echo "Usage: RESTORE_CONFIRM=replace-data ./scripts/restore.sh <backup.tar.gz>" >&2
  exit 1
fi
case "$archive_path" in
  /*) ;;
  *) archive_path="$project_dir/$archive_path" ;;
esac
if [[ ! -f "$archive_path" ]]; then
  echo "Backup archive not found: $archive_path" >&2
  exit 1
fi
if [[ "${RESTORE_CONFIRM:-}" != "replace-data" ]]; then
  echo "Restore replaces the current Work Graph database." >&2
  echo "Run again with RESTORE_CONFIRM=replace-data after checking the archive path." >&2
  exit 1
fi

temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT

archive_entries="$(tar -tzf "$archive_path")"
if [[ "$archive_entries" != $'manifest.txt\ndatabase.dump' ]]; then
  echo "Unsupported archive contents. Expected only manifest.txt and database.dump." >&2
  exit 1
fi
tar -xzf "$archive_path" -C "$temporary_dir" manifest.txt database.dump

manifest_value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$temporary_dir/manifest.txt"
}

format_version="$(manifest_value format_version)"
backup_schema="$(manifest_value schema_version)"
expected_checksum="$(manifest_value dump_sha256)"
if [[ "$format_version" != "1" || -z "$backup_schema" || -z "$expected_checksum" ]]; then
  echo "The backup manifest is incomplete or unsupported." >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  actual_checksum="$(shasum -a 256 "$temporary_dir/database.dump" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "$temporary_dir/database.dump" | awk '{print $1}')"
else
  echo "A SHA-256 checksum tool (shasum or sha256sum) is required." >&2
  exit 1
fi
if [[ "$actual_checksum" != "$expected_checksum" ]]; then
  echo "Backup checksum verification failed. The archive may be damaged." >&2
  exit 1
fi

latest_schema="$(find internal/database/migrations -maxdepth 1 -name '*.sql' -type f -exec basename {} \; | sort | tail -n 1)"
if [[ "$backup_schema" != "none" && "$backup_schema" > "$latest_schema" ]]; then
  echo "This backup uses schema $backup_schema, newer than this checkout supports ($latest_schema)." >&2
  exit 1
fi

safety_dir="${BACKUP_DIR:-$project_dir/backups}"
mkdir -p "$safety_dir"
safety_backup="$safety_dir/pre-restore-$(date -u '+%Y%m%dT%H%M%SZ').tar.gz"
docker compose up -d "$postgres_service" >/dev/null
if [[ "${SKIP_SAFETY_BACKUP:-}" == "empty-or-unrecoverable-database" ]]; then
  safety_backup=""
  echo "Skipping the pre-restore backup by explicit operator request."
else
  echo "Creating a safety backup of the current database…"
  "$script_dir/backup.sh" "$safety_backup"
fi

echo "Stopping application writers…"
docker compose stop api >/dev/null
docker compose --profile automation stop runner >/dev/null 2>&1 || true

echo "Replacing the database from the verified archive…"
docker compose exec -T "$postgres_service" psql -U "$database_user" -d postgres -v ON_ERROR_STOP=1 \
  -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$database_name' AND pid <> pg_backend_pid();" \
  -c "DROP DATABASE IF EXISTS \"$database_name\";" \
  -c "CREATE DATABASE \"$database_name\" OWNER \"$database_user\";" >/dev/null

if ! docker compose exec -T "$postgres_service" pg_restore \
  -U "$database_user" \
  -d "$database_name" \
  --no-owner \
  --no-privileges \
  --exit-on-error <"$temporary_dir/database.dump"; then
  echo "Restore failed. Application services remain stopped." >&2
  if [[ -n "$safety_backup" ]]; then
    echo "The previous database is preserved in: $safety_backup" >&2
  fi
  exit 1
fi

docker compose up -d "${restore_service_list[@]}" >/dev/null

echo "Restore completed successfully."
if [[ -n "$safety_backup" ]]; then
  echo "Safety backup of the replaced database: $safety_backup"
fi
echo "Ensure this installation uses the WORK_GRAPH_MASTER_KEY that belongs to the restored data."
