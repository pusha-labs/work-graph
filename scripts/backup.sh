#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$script_dir/.." && pwd)"
cd "$project_dir"

postgres_service="${POSTGRES_SERVICE:-postgres}"
database_name="${POSTGRES_DB:-workgraph}"
database_user="${POSTGRES_USER:-workgraph}"
backup_dir="${BACKUP_DIR:-$project_dir/backups}"
timestamp="$(date -u '+%Y%m%dT%H%M%SZ')"
output_path="${1:-$backup_dir/work-graph-$timestamp.tar.gz}"

if [[ ! "$postgres_service" =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]]; then
  echo "POSTGRES_SERVICE contains unsupported characters." >&2
  exit 1
fi

case "$output_path" in
  /*) ;;
  *) output_path="$project_dir/$output_path" ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required to back up Work Graph." >&2
  exit 1
fi

mkdir -p "$(dirname "$output_path")"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT

echo "Creating a consistent PostgreSQL backup…"
docker compose exec -T "$postgres_service" pg_dump \
  -U "$database_user" \
  -d "$database_name" \
  --format=custom \
  --no-owner \
  --no-privileges >"$temporary_dir/database.dump"

if [[ ! -s "$temporary_dir/database.dump" ]]; then
  echo "The database dump is empty; no backup was created." >&2
  exit 1
fi

schema_version="$(docker compose exec -T "$postgres_service" psql -U "$database_user" -d "$database_name" -Atqc "SELECT COALESCE(MAX(version),'none') FROM schema_migrations" | tr -d '\r')"
app_revision="$(git rev-parse HEAD 2>/dev/null || printf 'unknown')"

if command -v shasum >/dev/null 2>&1; then
  dump_checksum="$(shasum -a 256 "$temporary_dir/database.dump" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  dump_checksum="$(sha256sum "$temporary_dir/database.dump" | awk '{print $1}')"
else
  echo "A SHA-256 checksum tool (shasum or sha256sum) is required." >&2
  exit 1
fi

{
  printf 'format_version=1\n'
  printf 'created_at=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf 'app_revision=%s\n' "$app_revision"
  printf 'schema_version=%s\n' "$schema_version"
  printf 'database=%s\n' "$database_name"
  printf 'dump_sha256=%s\n' "$dump_checksum"
  printf 'encryption_key_required=true\n'
} >"$temporary_dir/manifest.txt"

temporary_archive="$output_path.partial"
rm -f "$temporary_archive"
tar -czf "$temporary_archive" -C "$temporary_dir" manifest.txt database.dump
chmod 600 "$temporary_archive"
mv "$temporary_archive" "$output_path"

echo "Backup created: $output_path"
echo "Keep the matching WORK_GRAPH_MASTER_KEY separately; it is intentionally not included in the archive."
