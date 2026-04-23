# Server

Go control plane implementation lives here.

### Admin Auth

The initial backend slice provides a minimal session-based admin auth flow:

- `GET /admin/login` renders a simple form
- `POST /admin/login` creates a session cookie
- `POST /admin/logout` clears the session cookie
- `GET /admin/me` verifies the active session
- `GET /admin/devices` demonstrates a permission-gated admin route

Run it with:

```sh
cd server
go test ./...
go run ./cmd/server
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

### Audit Capture

Admin create, update, and retire operations append immutable audit events in the in-memory audit store for now.

The current backend slice verifies this behavior through the server tests before the database-backed audit table lands.

### Admin E2E

The admin end-to-end test runs the login, CRUD, logout, and audit flow through the HTTP handler stack without a socket listener.

It exercises the same entrypoints that the roadmap uses for the clean-install verification checkpoint.
