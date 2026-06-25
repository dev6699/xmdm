# Operations

## Deployment Model

The supported implementation target is single-tenant self-hosted operation.
Runtime workflows are shaped around one active deployment tenant.

The local supported runtime uses:

- Go server
- PostgreSQL
- S3-compatible object storage
- Mosquitto MQTT broker with dynamic security

Production TLS termination, secret injection, backups, and monitoring are
operator responsibilities documented under `docs/`.

## Operations Decisions

- Docker Compose is the supported local runtime.
- Production TLS termination, secret injection, backup storage, and monitoring
  are deployment responsibilities.
- The server public URL must be reachable by enrolled devices.
- The launcher APK must be published into the managed app catalog before QR
  enrollment can install it as the managed launcher app.
- Command polling remains the operational recovery path when MQTT is degraded.

Operational procedures live in `docs/` and `infra/`.
