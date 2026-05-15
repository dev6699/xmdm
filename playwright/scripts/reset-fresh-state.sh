#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

cd infra
docker compose down -v >/dev/null 2>&1 || true
./bootstrap-local.sh >/dev/null
