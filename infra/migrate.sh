#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

postgres_db=${XMDM_POSTGRES_DB:-xmdm}
postgres_user=${XMDM_POSTGRES_USER:-xmdm}
postgres_password=${XMDM_POSTGRES_PASSWORD:-xmdm}

compose() {
  docker compose "$@"
}

psql() {
  compose exec -T -e PGPASSWORD="$postgres_password" postgres psql -h 127.0.0.1 -U "$postgres_user" -d "$postgres_db" -v ON_ERROR_STOP=1 "$@"
}

wait_for_postgres() {
  attempts=30
  while [ "$attempts" -gt 0 ]; do
    if psql -At -c 'SELECT 1' >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts - 1))
    sleep 1
  done
  echo "postgres is not ready" >&2
  exit 1
}

extract_up() {
  awk '
    /^-- \+goose Up$/ { in_up = 1; next }
    /^-- \+goose Down$/ { in_up = 0; next }
    in_up { print }
  ' "$1"
}

wait_for_postgres

psql <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  filename text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
SQL

for migration in "$script_dir"/../server/migrations/*.sql; do
  filename=$(basename "$migration")
  applied=$(psql -At -c "SELECT 1 FROM schema_migrations WHERE filename = '$filename';")
  if [ "$applied" = "1" ]; then
    continue
  fi
  extract_up "$migration" | psql
  psql -c "INSERT INTO schema_migrations (filename) VALUES ('$filename');"
done
