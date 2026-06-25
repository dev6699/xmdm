# Security Overview

## Admin Trust Boundary

Admin access uses the server-rendered dashboard and the auth service.
The built-in permission catalog currently contains:

- `admin.read`
- `admin.write`
- `devices.read`
- `devices.write`

Dashboard mutations keep CSRF and permission checks enabled.

## Device Trust Boundary

Enrollment starts with an enrollment token. After enrollment, the server issues
device identity material and device routes authenticate the device before
accepting telemetry, logs, device info, command polling, or artifact access.

Device credentials are scoped to a device and are stored server-side as hashes.

## Config And Artifact Integrity

The server generates config snapshots for devices. The launcher verifies config
snapshots before applying them.

Artifacts use checksums in server metadata and device download flows. Managed
file artifact responses include checksum headers.

## MQTT Command Delivery Boundary

MQTT is used for push delivery. The server remains the command source of truth,
and device polling remains the recovery path.

The local Mosquitto setup uses dynamic security. Device clients are provisioned
with per-device broker access, and the server publisher uses the `xmdm-server`
client.

## Audit And Mutation Visibility

Admin mutations record audit events for implemented dashboard/API flows that
call the audit store.

Audit coverage statements should stay limited to flows that call the audit
store.

## Rate Limits

The router installs rate limits for:

- `POST /admin/login`
- `POST /admin/commands/create`
- `POST /api/v1/enrollment`

## TLS And Secret Handling Boundary

The repository contains runtime config and local Docker Compose defaults.
Production TLS termination and secret injection are deployment responsibilities.
