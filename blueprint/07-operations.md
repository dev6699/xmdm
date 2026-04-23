# Operations

## Deployment Model

The first supported deployment is single-tenant self-hosted.

Recommended baseline:

- Go server binary
- PostgreSQL
- Object storage backend
- MQTT broker or embedded MQTT endpoint
- Reverse proxy with TLS termination

## Local Development

- Run PostgreSQL locally or in Docker.
- Run the Go server with a local config file.
- Point the Android agent at the local server URL.
- Use a physical device or emulator that can be device-owner provisioned.
- Keep local object storage and MQ credentials isolated from production values.

## Build Flow

### Server

- Build the Go services with the repository module graph.
- Run migrations before starting the server.
- Use Docker Compose to stand up PostgreSQL, object storage, and MQTT for local runs.
- Generate any static web assets required by the admin console.
- Verify the health endpoints before testing the agent.

### Android Agent

- Build the Kotlin app with the Android Gradle Plugin toolchain.
- Install to a provisionable device or emulator.
- Verify enrollment and sync against the development server.
- Validate that bootstrap values survive app restart and reboot.

## Configuration Domains

- Database connection
- Object storage connection
- Admin auth and password recovery
- Device enrollment and signing keys
- MQTT connection settings
- File and app download base URLs
- Email and notification settings
- Rebranding values
- Retention windows
- Feature flags for enterprise modules and plugin enablement

## Observability

- Structured logs with request IDs
- Health endpoints for HTTP, DB, object storage, and MQTT
- Metrics for enrollments, sync success, push delivery, app installs, file downloads, and job failures
- Audit-friendly operator actions
- Admin-visible job and sync failure pages

## Backups

- Back up PostgreSQL daily.
- Back up object storage buckets or directories daily.
- Keep encryption keys and secrets in a separate secure backup.
- Test restore before release milestones.
- Keep a documented point-in-time restore procedure.

## Maintenance Tasks

- Purge old logs and messages
- Expire stale enrollment tokens
- Reconcile stuck push messages
- Clean orphaned artifact records
- Rotate secrets and signing keys on a schedule
- Rebuild derived indexes and caches when the schema changes

## Release Process

- Freeze API contracts.
- Run integration and end-to-end tests.
- Tag server and agent releases together.
- Publish server artifacts, then Android agent artifacts.
- Validate upgrade and rollback on a staging device set.
- Release docs with the code so operators can match behavior to procedure.

## Troubleshooting

- If devices fail to sync, check auth, enrollment state, and response signatures.
- If push does not work, check MQTT connectivity, broker auth, and message queue state.
- If installs fail, check artifact checksums and object storage access.
- If plugin behavior is missing, check plugin registration and enablement state.
- If admin forms fail, check CSRF/session handling and permission mapping.
