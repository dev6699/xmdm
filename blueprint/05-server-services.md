# Server Services

## Server Design

The Go backend owns admin auth, dashboard rendering, enrollment, policy/content
state, artifact metadata and storage access, device telemetry/logs/info,
command state, MQTT publishing, audit, and plugin registration.

The server must start and operate core enrollment, sync, dashboard, and command
flows when optional plugins are omitted.

## Server Stack

- Go with the standard HTTP server.
- Server-rendered dashboard using Go templates.
- pgx-backed PostgreSQL repositories.
- Goose migrations.
- AWS SDK S3-compatible artifact storage.
- Standard logging and observability middleware.
- Docker Compose for local runtime.

## Service Decisions

- Runtime configuration supplies bind address, public URL, session TTL,
  PostgreSQL, MQTT, object storage, command/config sync intervals, and seed
  admin credentials.
- Browser operations use sessions, CSRF checks, RBAC checks, and audit records
  where implemented.
- Device operations use per-device authentication after enrollment.
- Config snapshots are computed from server-owned device, policy, runtime, and
  content state.
- Command state is persisted before push delivery is attempted.
- Object storage holds binary artifacts; PostgreSQL holds artifact metadata.
- Plugin-provided behavior is registered at startup and must not be required for
  core server startup.

Operator-facing service behavior belongs in the docs hub.

## Failure Modes

- If MQTT fails, polling must still deliver commands.
- If a duplicate command is delivered, the launcher must not execute it twice.
- If object storage is slow or unavailable, metadata and diagnostic views should
  remain usable where possible.
- If optional plugins are omitted, core enrollment, sync, dashboard, and command
  behavior must continue.
