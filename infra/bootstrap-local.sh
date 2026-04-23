#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

docker compose up -d

cat <<'EOF'
Local stack is starting.

Next:
- run the Go server with the local config
- point the Android agent at the local server URL
- verify enrollment and sync
EOF
