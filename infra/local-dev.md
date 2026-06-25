# Local Development

## Requirements

- Docker Engine and Docker Compose
- Go toolchain for the server
- Android Studio or the Android command-line toolchain
- A provisionable Android emulator or physical device for launcher testing

## Local Stack

Bring up the supporting services first:

```sh
cd infra
docker compose up -d
sh ./migrate.sh
```

The local stack provides:

- PostgreSQL
- SeaweedFS S3-compatible object storage
- MQTT broker

The MQTT broker starts with Mosquitto dynamic security enabled. The broker seeds its admin password from `MOSQUITTO_DYNSEC_PASSWORD`, and the server provisions device clients automatically during enrollment.

## Bootstrap Flow

1. Start the local stack.
2. Apply the database migrations and seed data.
3. Run the server against the local database using `XMDM_POSTGRES_DSN='postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable'`.
4. Point the Android launcher at the local server URL.
5. Enroll a device and verify sync.
6. When you need to reprovision a physical device, use the adb-backed coverage described in [Server E2E](../server/e2e/README.md) as the canonical reprovision and verification flow.

For server tests, create or use a separate database and set `XMDM_TEST_POSTGRES_DSN` before running `go test ./...`. Do not point the test DSN at the runtime database.

To create the local test database and print a safe DSN, run:

```sh
eval "$(./test-db-env.sh)"
cd ../server
go test ./...
```

To verify a backup and restore drill against the local test database, run:

```sh
./backup-restore-drill.sh
```

For incident recovery and release rollback steps, see [../docs/disaster-recovery-and-rollback.md](../docs/disaster-recovery-and-rollback.md).

For the final hardening cleanup pass, see [../docs/cleanup-pass.md](../docs/cleanup-pass.md).

## Notes

- Keep local credentials separate from production credentials.
- The compose file pins image tags so the local environment does not drift unexpectedly.
- SeaweedFS exposes an S3 endpoint on `localhost:8333` and accepts arbitrary access and secret keys by default, so use a local-only credential pair in app and server config.
- The server defaults use `XMDM_OBJECT_STORAGE_ENDPOINT=http://127.0.0.1:8333`, `XMDM_OBJECT_STORAGE_BUCKET=xmdm`, and matching local credentials when the env vars are unset.
- The broker defaults use `MOSQUITTO_DYNSEC_PASSWORD=xmdm-admin` unless you override it.
- The server-side broker provisioner uses `XMDM_MQTT_DYNSEC_ADDRESS=127.0.0.1:1883`, `XMDM_MQTT_DYNSEC_CLIENT_ID=xmdm-dynsec`, `XMDM_MQTT_DYNSEC_ADMIN_USER=admin`, `XMDM_MQTT_DYNSEC_PASSWORD=xmdm-admin`, and default keepalive and dial timeout values.
- The command publisher uses the broker `xmdm-server` client by default via `XMDM_MQTT_USERNAME=xmdm-server` and `XMDM_MQTT_PASSWORD=xmdm-server-secret`.
- The HTTP polling fallback uses `GET /api/v1/devices/{deviceId}/commands` with `X-XMDM-Device-Secret` and reads PostgreSQL `commands` rows.
- If a service container is stale, stop the stack and recreate it before debugging the launcher.
