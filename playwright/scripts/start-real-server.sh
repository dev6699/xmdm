#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

bash ./playwright/scripts/reset-fresh-state.sh
cd server
export XMDM_ADDR="${XMDM_ADDR:-:39092}"
exec go run ./cmd/server
