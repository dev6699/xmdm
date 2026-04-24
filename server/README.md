# Server

Go control plane implementation lives here.

### Admin Auth

The admin console provides a session-based admin auth flow:

- `GET /api/v1/admin/login` renders a simple form
- `POST /api/v1/admin/login` creates a session cookie
- `POST /api/v1/admin/logout` clears the session cookie
- `GET /api/v1/admin/me` verifies the active session
- `GET /api/v1/admin/devices` exercises a permission-gated admin route

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

### Migration Tooling

The local bootstrap migrator lives in [../infra/migrate.sh](../infra/migrate.sh).

It applies the SQL files in [migrations/](migrations/) once, records applied files in `schema_migrations`, and seeds the single active tenant row required by the single-tenant v1 model.
The bootstrap set now also creates the `audit_events` table used by the database-backed audit store.

### Audit Capture

Admin create, update, and retire operations append immutable audit events in the `audit_events` table.

### Admin E2E

The admin end-to-end test runs the login, CRUD, logout, and audit flow through the HTTP handler stack without a socket listener.

It exercises the same entrypoints that the roadmap uses for the clean-install verification checkpoint.

### Plugin Isolation

Optional plugin routes are registered through an explicit manager object.

The default server wiring keeps that manager disabled, so core admin routes continue to work even when no plugins are enabled.
