#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

printf '%s\n' '[infra] docker compose up -d'
docker compose up -d
printf '%s\n' '[infra] run migrations'
sh ./migrate.sh

cat <<'EOF'
Local stack is starting.

Next:
- run the Go server with the local config
- point the Android agent at the local server URL
- verify enrollment and sync
EOF
