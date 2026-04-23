# Server Services

## Server Design

The Go backend is the authoritative source for auth, policies, device inventory, artifacts, push dispatch, logs, audit, messaging, and plugins.

The server must be able to start, serve admin requests, and accept device sync even if some optional plugins are unavailable.

## Backend Packages

- `cmd/server` for the HTTP entrypoint
- `internal/auth` for admin sessions and tokens
- `internal/device` for device inventory and sync
- `internal/policy` for policy resolution
- `internal/apps` for app catalog and versions
- `internal/files` for file and certificate handling
- `internal/push` for command fan-out and device notifications
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
- Device state transition tracking
- Heartbeat and telemetry recording

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

- Plugin discovery
- Plugin enable/disable state
- Plugin-specific REST settings
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
4. MQTT publishes the message when the device is online.
5. HTTP polling returns pending messages when MQTT is unavailable.
6. Device acks receipt and execution.
7. Server marks the delivery complete.

## Sync Processing

1. Device authenticates.
2. Server loads the active device state and policy.
3. Server computes a signed snapshot.
4. Server includes pending artifacts and commands.
5. Device applies the snapshot and reports acknowledgements.
6. Server persists the new device state and telemetry.

## Plugin Design

- Plugins are discovered at startup.
- Plugins may contribute REST resources, persistence, and worker modules.
- Plugin settings live in tenant-scoped storage.
- The plugin manager must be able to disable a plugin without breaking core sync.
- Core server functions must not depend on a plugin for basic enrollment or device sync.

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
- If MQTT fails, polling must still deliver commands.
- If object storage is slow, metadata reads should still work.
- If a job crashes mid-run, the next run must be able to continue from persisted state.
