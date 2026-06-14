#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

log() {
  printf '%s\n' "$1"
}

log '[playwright] reset local stack'
bash ./playwright/scripts/reset-fresh-state.sh

server_addr="${XMDM_ADDR:-:39092}"
server_port="${server_addr##*:}"
server_public_url="${XMDM_SERVER_PUBLIC_URL:-http://127.0.0.1:${server_port}}"
server_url="http://127.0.0.1:${server_port}/admin/login"
server_log=$(mktemp /tmp/xmdm-playwright-server.XXXXXX.log)
server_pid=""
tail_pid=""

cleanup() {
  if [ -n "$tail_pid" ] && kill -0 "$tail_pid" >/dev/null 2>&1; then
    kill "$tail_pid" >/dev/null 2>&1 || true
  fi
  if [ -n "$server_pid" ] && kill -0 "$server_pid" >/dev/null 2>&1; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$server_log"
}

trap cleanup INT TERM HUP EXIT

log "[playwright] start server on ${server_addr}"
(
  cd server
  export XMDM_ADDR="$server_addr"
  export XMDM_SERVER_PUBLIC_URL="$server_public_url"
  export XMDM_DISABLE_REQUEST_LOGS=1
  exec go run ./cmd/server
) >"$server_log" 2>&1 &
server_pid=$!

tail -n 0 -f "$server_log" &
tail_pid=$!

log "[playwright] waiting for server readiness at ${server_url}"
attempt=0
while ! curl -fsS "$server_url" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    log "[playwright] server exited before becoming ready"
    cat "$server_log" >&2
    exit 1
  fi
  if [ "$attempt" -eq 1 ] || [ $((attempt % 5)) -eq 0 ]; then
    log "[playwright] still waiting for server readiness (${attempt}s)"
  fi
  sleep 1
done

log "[playwright] server ready"
wait "$server_pid"
