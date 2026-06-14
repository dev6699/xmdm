#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

cd infra
printf '%s\n' '[playwright] docker compose down -v'
docker compose down -v || true
printf '%s\n' '[playwright] bootstrap local infra'
./bootstrap-local.sh
