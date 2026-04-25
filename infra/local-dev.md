# Local Development

## Requirements

- Docker Engine and Docker Compose
- Go toolchain for the server
- Android Studio or the Android command-line toolchain
- A provisionable Android emulator or physical device for agent testing

## Local Stack

Bring up the supporting services first:

```sh
cd infra
docker compose up -d
./migrate.sh
```

The local stack provides:

- PostgreSQL
- SeaweedFS S3-compatible object storage
- MQTT broker

## Bootstrap Flow

1. Start the local stack.
2. Apply the database migrations and seed data.
3. Run the server against the local database using `XMDM_POSTGRES_DSN='postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable'`.
4. Point the Android agent at the local server URL.
5. Enroll a device and verify sync.
6. When you need to reprovision a physical device, use the adb-backed content E2E in [`server/e2e/content_test.go`](../server/e2e/content_test.go) as the canonical reprovision and verification flow.

For server tests, create or use a separate database and set `XMDM_TEST_POSTGRES_DSN` before running `go test ./...`. Do not point the test DSN at the runtime database.

To create the local test database and print a safe DSN, run:

```sh
eval "$(./test-db-env.sh)"
go test ./...
```

## Notes

- Keep local credentials separate from production credentials.
- The compose file pins image tags so the local environment does not drift unexpectedly.
- SeaweedFS exposes an S3 endpoint on `localhost:8333` and accepts arbitrary access and secret keys by default, so use a local-only credential pair in app and server config.
- The server defaults use `XMDM_OBJECT_STORAGE_ENDPOINT=http://127.0.0.1:8333`, `XMDM_OBJECT_STORAGE_BUCKET=xmdm`, and matching local credentials when the env vars are unset.
- If a service container is stale, stop the stack and recreate it before debugging the agent.
