#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

if [ -z "${XMDM_TEST_POSTGRES_DSN:-}" ]; then
  eval "$(./test-db-env.sh)"
fi

source_db=${XMDM_TEST_POSTGRES_DB:-xmdm_test}
source_user=${XMDM_TEST_POSTGRES_USER:-xmdm}
source_password=${XMDM_TEST_POSTGRES_PASSWORD:-xmdm}
source_host=${XMDM_TEST_POSTGRES_HOST:-127.0.0.1}
source_port=${XMDM_TEST_POSTGRES_PORT:-5432}
restore_db=${XMDM_RESTORE_POSTGRES_DB:-xmdm_restore_$(date +%Y%m%d%H%M%S)}

dump_file=$(mktemp /tmp/xmdm-backup-restore.XXXXXX.sql)

cleanup() {
  rm -f "$dump_file"
  docker compose exec -T -e PGPASSWORD="$source_password" postgres \
    psql -h "$source_host" -p "$source_port" -U "$source_user" -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE IF EXISTS $restore_db" >/dev/null 2>&1 || true
}
trap cleanup EXIT

XMDM_POSTGRES_DB="$source_db" sh ./migrate.sh >/dev/null

docker compose exec -T -e PGPASSWORD="$source_password" postgres \
  pg_dump -h "$source_host" -p "$source_port" -U "$source_user" -d "$source_db" \
  --no-owner --no-privileges > "$dump_file"

docker compose exec -T -e PGPASSWORD="$source_password" postgres \
  psql -h "$source_host" -p "$source_port" -U "$source_user" -d postgres -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS $restore_db" >/dev/null
docker compose exec -T -e PGPASSWORD="$source_password" postgres \
  psql -h "$source_host" -p "$source_port" -U "$source_user" -d postgres -v ON_ERROR_STOP=1 \
  -c "CREATE DATABASE $restore_db" >/dev/null
docker compose exec -T -e PGPASSWORD="$source_password" postgres \
  psql -h "$source_host" -p "$source_port" -U "$source_user" -d "$restore_db" -v ON_ERROR_STOP=1 \
  < "$dump_file" >/dev/null

tables="schema_migrations tenants roles users groups policies devices device_groups artifacts files managed_files apps app_versions enrollment_tokens audit_events commands device_logs device_info device_telemetry"

for table in $tables; do
  source_count=$(docker compose exec -T -e PGPASSWORD="$source_password" postgres \
    psql -h "$source_host" -p "$source_port" -U "$source_user" -d "$source_db" -At -v ON_ERROR_STOP=1 \
    -c "SELECT count(*) FROM $table;")
  restored_count=$(docker compose exec -T -e PGPASSWORD="$source_password" postgres \
    psql -h "$source_host" -p "$source_port" -U "$source_user" -d "$restore_db" -At -v ON_ERROR_STOP=1 \
    -c "SELECT count(*) FROM $table;")
  if [ "$source_count" != "$restored_count" ]; then
    printf 'restore verification failed for %s: source=%s restored=%s\n' "$table" "$source_count" "$restored_count" >&2
    exit 1
  fi
done

printf 'restore drill succeeded: source=%s restore=%s\n' "$source_db" "$restore_db"
