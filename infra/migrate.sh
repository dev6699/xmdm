#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"
server_dir=$(CDPATH= cd -- "$script_dir/../server" && pwd)

postgres_db=${XMDM_POSTGRES_DB:-xmdm}
postgres_user=${XMDM_POSTGRES_USER:-xmdm}
postgres_password=${XMDM_POSTGRES_PASSWORD:-xmdm}

compose() {
  docker compose "$@"
}

wait_for_postgres() {
  attempts=30
  while [ "$attempts" -gt 0 ]; do
    if compose exec -T postgres pg_isready -h 127.0.0.1 -U "$postgres_user" -d "$postgres_db" >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts - 1))
    sleep 1
  done
  echo "postgres is not ready" >&2
  exit 1
}

wait_for_postgres

export XMDM_POSTGRES_DSN="postgres://$postgres_user:$postgres_password@127.0.0.1:5432/$postgres_db?sslmode=disable"
cd "$server_dir"
go run ./cmd/server -migrate-only
