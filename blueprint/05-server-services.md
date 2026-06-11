# Server Services

## Server Design

The Go backend is the authoritative source for auth, policies, device inventory, artifacts, push dispatch, logs, device info, audit, messaging, and plugins.

The server must be able to start, serve admin requests, and accept device sync even if some optional plugins are unavailable.

## Server Stack

- Go as the primary implementation language.
- `net/http` for the base HTTP server.
- `chi` for request routing and middleware.
- `html/template` for the server-rendered admin console.
- `pgx` for PostgreSQL access.
- `sqlc` for typed data access code.
- `goose` for schema migrations.
- `slog` for structured logging.
- Docker Compose for the first supported runtime and local developer environment.
- `encoding/json` for contract payloads.

## Backend Packages

- `cmd/server` for the HTTP entrypoint
- `internal/auth` for admin sessions and tokens
- `internal/device` for device inventory and sync
- `internal/policy` for policy resolution
- `internal/apps` for app management and versions
- `internal/files` for file and certificate handling
- `internal/push` for command fan-out and device notifications
- `internal/logs` for device log upload and search
- `internal/deviceinfo` for device inventory reporting and export
- `internal/plugins` for plugin registration and plugin settings
- `internal/audit` for audit events
- `internal/ops` for scheduled jobs and health checks

## Service Inventory

### Authentication

- Admin login and logout
- Session issuance
- JWT access tokens for API clients
- Password recovery and password policy

### Enrollment

- Enrollment token creation
- QR payload generation
- Device binding and initial secret issuance
- Duplicate enrollment protection

### Device Sync

- Policy snapshot calculation
- App, file, certificate, and command inclusion
- Device-authenticated config refresh at `GET /api/v1/devices/{deviceId}/config`
- Device state transition tracking
- Heartbeat and telemetry recording

### Device Logs

- Device-authenticated batch log upload
- Admin search across device log records
- Retention and export of log rows

### Device Info

- Device-authenticated inventory and runtime reporting
- Admin export across device info records
- Retention and export of device info rows

### Artifact Service

- APK and file upload
- Artifact checksum validation
- Signed download URLs or authorized object access
- Cleanup of stale and orphaned objects

### Push Service

- MQTT publish
- HTTP long-poll fallback
- Pending message queue
- Retry and expiry handling

### Plugin Service

- Static plugin registration
- Plugin metadata catalog
- Plugin enable/disable state
- Plugin-specific REST settings
- Plugin-provided admin device actions
- Plugin command type registration
- Plugin task registration

### Audit Service

- Admin action audit
- Device command audit
- Security event audit
- Retention and export

### Messaging Service

- Broadcast messages
- Group-targeted messages
- Device-targeted messages
- Read status and delivery tracking

### Update Service

- Check for updates
- Download updates
- Publish new app versions and manifests

## Request Handling Rules

- Never block request handlers on large downloads.
- Use background workers for heavy writes and fan-out.
- Write command and push records first, then deliver asynchronously.
- Keep the sync API deterministic and idempotent.
- Return versioned config snapshots, not ad hoc payloads.
- Let read-only admin views degrade gracefully when a plugin is disabled.

## Push Delivery Design

1. Admin creates a message or command.
2. Server persists the message.
3. Fan-out worker resolves target devices.
4. MQTT publishes the message when the device is online immediately after enqueue and marks the queue row `sent` when the broker accepts it.
5. HTTP polling returns pending messages from `GET /api/v1/devices/{deviceId}/commands` when MQTT is unavailable or when a queued row has not yet been delivered.
6. Device acks receipt and execution through `POST /api/v1/devices/{deviceId}/commands/{commandId}/ack`.
7. Server marks the delivery complete.

- The messaging and audit surface is dashboard-first for operators, with device-facing HTTP endpoints for sync and acknowledgements.

- MQTT command topics use `devices/{deviceId}/commands` for device-targeted delivery.
- The broker must enforce device-topic isolation with per-client authentication and ACLs, not topic names alone.
- Device MQTT credentials are provisioned by the server at enrollment time and retired when the device is retired.
- If broker provisioning fails, enrollment still completes and the device can fall back to polling.

## Sync Processing

1. Device authenticates.
2. Server loads the active device state and policy.
3. Server computes a signed snapshot.
4. Server includes pending artifacts and config state.
5. Device may refresh the latest signed config snapshot through `GET /api/v1/devices/{deviceId}/config` and then applies the snapshot.
6. Device reports acknowledgements and telemetry.
7. Server persists the new device state.

## Plugin Design

- Plugins are statically registered at startup by code linked into the server build.
- Plugins may contribute REST resources, persistence, and worker modules.
- Plugins may contribute server-rendered admin routes under `/admin/plugins/{pluginId}/...`.
- Plugins may contribute browser admin routes under `/admin/plugins/{pluginId}/...`.
- Plugins may contribute device-detail actions that link to plugin-owned routes.
- Plugins may register command types for validation and display while the core command queue remains authoritative for delivery, expiry, and acknowledgement.
- Plugin settings live in tenant-scoped storage.
- The plugin manager must be able to disable a plugin without breaking core sync.
- Core server functions must not depend on a plugin for basic enrollment or device sync.
- Core must not load arbitrary third-party binaries at runtime.
- Premium plugin implementation lives outside the open-core repo and uses the same static extension contracts; core only exposes the generic extension points defined in the product principles.

## Update Design

- Update manifests are server-generated.
- The server validates file hashes before publishing artifacts.
- Web app, agent, and optional secondary packages are updated independently.
- Update jobs write results to an audit-friendly record.

## Operational Jobs

- Purge stale push messages
- Purge old logs
- Purge expired temporary artifacts
- Recalculate derived indexes when needed
- Backfill or repair push delivery after transient outages
- Rebuild plugin indexes after deployment

## Server Failure Modes

- If a plugin fails, core enrollment and sync must continue.
- If a plugin is disabled, plugin routes, command types, and device actions must be unavailable while core routes continue.
- If MQTT fails, polling must still deliver commands.
- If object storage is slow, metadata reads should still work.
- If a job crashes mid-run, the next run must be able to continue from persisted state.
