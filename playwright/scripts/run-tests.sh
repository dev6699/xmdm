#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

server_pid=""

cleanup() {
  if [ -n "$server_pid" ] && kill -0 "$server_pid" >/dev/null 2>&1; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi
}

trap cleanup INT TERM HUP EXIT

kill_port_listeners() {
  port=$1
  if ! command -v ss >/dev/null 2>&1; then
    return 0
  fi
  ss -ltnp "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | while read -r pid; do
    if [ -n "$pid" ]; then
      printf '%s\n' "[playwright] stopping existing listener on :${port} (pid ${pid})"
      kill "$pid" >/dev/null 2>&1 || true
    fi
  done
}

if [ -n "${XMDM_DASHBOARD_URL:-}" ]; then
  printf '%s\n' "[playwright] using external dashboard at ${XMDM_DASHBOARD_URL}"
else
  printf '%s\n' '[playwright] starting local dashboard server'
  kill_port_listeners 39092
  bash ./playwright/scripts/start-real-server.sh &
  server_pid=$!

  printf '%s\n' '[playwright] waiting for local dashboard readiness'
  export XMDM_DASHBOARD_URL="${XMDM_DASHBOARD_URL:-http://127.0.0.1:39092}"
  for attempt in $(seq 1 180); do
    if curl -fsS "${XMDM_DASHBOARD_URL}/admin/login" >/dev/null 2>&1; then
      printf '%s\n' '[playwright] local dashboard ready'
      break
    fi
    if [ "$attempt" -eq 1 ] || [ $((attempt % 15)) -eq 0 ]; then
      printf '%s\n' "[playwright] still waiting for local dashboard (${attempt}s)"
    fi
    sleep 1
  done
fi

cd playwright
npx playwright test "$@"
