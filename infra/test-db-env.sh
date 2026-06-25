#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

test_db=${XMDM_TEST_POSTGRES_DB:-xmdm_test}
test_user=${XMDM_TEST_POSTGRES_USER:-xmdm}
test_password=${XMDM_TEST_POSTGRES_PASSWORD:-xmdm}
test_host=${XMDM_TEST_POSTGRES_HOST:-127.0.0.1}
test_port=${XMDM_TEST_POSTGRES_PORT:-5432}

docker compose up -d postgres >/dev/null

printf '%s\n' '[infra] waiting for postgres' >&2
attempts=30
while [ "$attempts" -gt 0 ]; do
  if docker compose exec -T postgres pg_isready -h 127.0.0.1 -U "$test_user" -d postgres >/dev/null 2>&1; then
    break
  fi
  attempts=$((attempts - 1))
  sleep 1
done
if [ "$attempts" -eq 0 ]; then
  echo "postgres is not ready" >&2
  exit 1
fi

exists=$(docker compose exec -T -e PGPASSWORD="$test_password" postgres psql -h 127.0.0.1 -U "$test_user" -d postgres -At -v ON_ERROR_STOP=1 -c "SELECT 1 FROM pg_database WHERE datname = '$test_db';")
if [ "$exists" != "1" ]; then
  docker compose exec -T -e PGPASSWORD="$test_password" postgres psql -h 127.0.0.1 -U "$test_user" -d postgres -v ON_ERROR_STOP=1 -q -c "CREATE DATABASE $test_db;" >/dev/null
fi

XMDM_POSTGRES_DB="$test_db" sh ./migrate.sh >/dev/null

printf 'export XMDM_TEST_POSTGRES_DSN=%s\n' "postgres://$test_user:$test_password@$test_host:$test_port/$test_db?sslmode=disable"
