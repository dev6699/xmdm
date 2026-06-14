# Server

Go control plane implementation lives here.

### Admin Auth

The admin console provides a session-based admin auth flow:

- `GET /admin/login` renders a simple form
- `POST /admin/login` creates a session cookie
- `POST /admin/logout` clears the session cookie
- `GET /admin/me` verifies the active session
- `GET /api/v1/devices` exercises a permission-gated admin route

Run it with:

```sh
cd server
go test ./...
go run ./cmd/server
```

The server uses the Postgres-backed stores by default. If you want to point at a different database, set `XMDM_POSTGRES_DSN` before starting the server. The local compose database uses:

```sh
export XMDM_POSTGRES_DSN='postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable'
```

Server tests require a dedicated test database and will fail fast unless `XMDM_TEST_POSTGRES_DSN` is set. Use a separate database name, not the runtime database:

```sh
eval "$(../infra/test-db-env.sh)"
# Run all tests except e2e (requires running server/services) with one package at a time,
# because the suite shares a single test database:
XMDM_TEST_POSTGRES_DSN="$XMDM_POSTGRES_DSN" go test -p 1 $(go list ./... | grep -v "/e2e")
# Run all tests including e2e:
go test ./...
```

### Core Schema

The first PostgreSQL migration lives in [migrations/20260423183000_core_schema.sql](migrations/20260423183000_core_schema.sql).

It creates the tenant-aware foundation tables:

- tenants
- roles
- users
- groups
- policies
- devices
- device_groups

### App Management

App management and immutable version upload now live under `/api/v1/apps` and `/api/v1/apps/{id}/versions`.

The admin E2E coverage verifies:

- app create, list, update, and retire
- app version upload with publish support
- version listing for a managed app

### File Storage

File upload and artifact metadata storage now live under `/api/v1/files`.

The admin E2E coverage verifies:

- multipart file upload persists the binary into object storage plus the file and artifact metadata
- file listing includes the backing artifact details
- file retirement marks the logical file retired while preserving the artifact metadata

The server defaults to the local SeaweedFS S3 endpoint on `localhost:8333` and reads the object-store settings from `XMDM_OBJECT_STORAGE_ENDPOINT`, `XMDM_OBJECT_STORAGE_REGION`, `XMDM_OBJECT_STORAGE_ACCESS_KEY`, `XMDM_OBJECT_STORAGE_SECRET_KEY`, and `XMDM_OBJECT_STORAGE_BUCKET`.

### Artifact Cleanup

Ops can inspect and purge orphan artifact records with [cmd/artifact-cleanup/README.md](cmd/artifact-cleanup/README.md).

### Cleanup Pass

Ops can run the hardening cleanup pass for stale enrollment tokens, stuck commands, and orphan artifact records with [cmd/cleanup-pass/README.md](cmd/cleanup-pass/README.md).

### Push Transport

The server now constructs an internal MQTT publisher in [internal/push](internal/push) at startup and points it at `127.0.0.1:1883` by default.
Override `XMDM_MQTT_ADDRESS` if you want to use a different broker.
The dynsec control client authenticates as `admin` by default; override `XMDM_MQTT_DYNSEC_ADMIN_USER` and `XMDM_MQTT_DYNSEC_PASSWORD` to match a different broker identity.
The command queue publishes command envelopes to `devices/{deviceId}/commands` immediately after enqueue and marks the row `sent` when MQTT succeeds, while keeping the row readable through the polling fallback path if publish fails.
Device MQTT credentials are provisioned automatically when enrollment binds a device and are revoked when the device is retired.
Override `XMDM_MQTT_DYNSEC_ADDRESS`, `XMDM_MQTT_DYNSEC_CLIENT_ID`, `XMDM_MQTT_DYNSEC_ADMIN_USER`, `XMDM_MQTT_DYNSEC_PASSWORD`, `XMDM_MQTT_DYNSEC_KEEPALIVE`, or `XMDM_MQTT_DYNSEC_DIAL_TIMEOUT` if your broker control path differs from the local compose defaults.
The command publisher uses the broker `xmdm-server` client by default; override `XMDM_MQTT_USERNAME` and `XMDM_MQTT_PASSWORD` if you change that broker identity.
For topic isolation, see [../infra/mosquitto/mqtt-security.md](../infra/mosquitto/mqtt-security.md).

### Polling Fallback

The server now exposes `GET /api/v1/devices/{deviceId}/commands` as the HTTP fallback path for pending commands.
The endpoint authenticates with `X-XMDM-Device-Secret`, reads queued or sent command rows from PostgreSQL, and returns them in a `commands` array.
The browser dashboard exposes `POST /admin/commands/create` for queued command creation, and the request target can expand to a device, a group, or a broadcast set.
The dashboard also exposes command and audit pages at `/admin/commands` and `/admin/audit` for recent command rows and audit events.
Device acknowledgements use `POST /api/v1/devices/{deviceId}/commands/{commandId}/ack` with the same device secret header and update the command row to `acked` or `failed`.
The messaging and audit surface is dashboard-first for operators, with device-facing HTTP endpoints for sync and acknowledgements.

### Migration Tooling

The server applies embedded Goose migrations at startup. The migration helper lives in [migrate.go](migrate.go), and the `server` command also supports `-migrate-only` for bootstrap scripts.

It applies the SQL files in [migrations/](migrations/) once and records applied files in `schema_migrations`.

The local bootstrap script in [../infra/migrate.sh](../infra/migrate.sh) now delegates to the embedded Go migrator for the initial database setup.

### Audit Capture

Admin create, update, and retire operations append immutable audit events in the `audit_events` table.

### Admin E2E

The root E2E suite in [e2e/README.md](e2e/README.md) runs the login, CRUD, logout, enrollment, telemetry, command, and audit flows through the HTTP handler stack without a socket listener.

It exercises the same entrypoints that the roadmap uses for the clean-install verification checkpoint.

### Plugin Isolation

Optional plugin routes are registered through an explicit manager object. The manager exposes admin-authenticated plugin metadata, including enablement state and static route/device-action/command-type declarations, without forcing core routes to depend on any plugin.

The default server wiring keeps that manager disabled, so core admin routes continue to work even when no plugins are enabled.
