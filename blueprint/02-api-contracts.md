# API Contracts

## API Rules

- Version all external endpoints under `/api/v1`.
- Keep public, device, and admin APIs separate.
- Return machine-readable error objects with stable error codes.
- Sign all device config payloads.
- Use JSON for request and response bodies unless file transfer is required.
- Make mutation endpoints idempotent when repeat submission is possible.

## Authentication Contracts

### Admin Auth

- Session cookie for the web console.
- JWT access token for API clients and automation.
- CSRF protection for browser-mutating requests.

### Device Auth

- One-time enrollment token during provisioning.
- Per-device secret after enrollment.
- Device ID is the stable identifier, but not the only secret.

### Plugin Auth

- Plugins inherit the auth context of the calling user or device.
- Sensitive plugin settings require an admin session or scoped token.

## Public Device APIs

### Enrollment

- `POST /api/v1/public/enrollments`
- Creates or binds a device during first enrollment.
- Accepts enrollment token, device identity hints, and optional bootstrap fields.
- Returns device ID, device secret, config snapshot, and initial artifact lists.

### QR Payload

- `GET /api/v1/public/enrollments/{enrollmentId}/qr`
- Returns JSON used to generate the QR code.
- QR content includes server URL, enrollment token, device identity policy, and bootstrap extras.

### Device Config Sync

- `GET /api/v1/device/{deviceId}/config`
- `POST /api/v1/device/{deviceId}/config`
- Returns the signed policy snapshot, app list, file list, certificate list, and pending commands.
- POST is used when the client needs to send telemetry in the same round trip.

### Telemetry Upload

- `POST /api/v1/device/{deviceId}/telemetry`
- Uploads heartbeat, battery, network, location, and app state.

### Log Upload

- `POST /api/v1/device/{deviceId}/logs`
- App and device logs are batch uploaded.

### Message Polling

- `GET /api/v1/device/{deviceId}/messages`
- Used only when MQTT is not active.

### Artifact Download

- `GET /api/v1/device/{deviceId}/artifacts/{artifactId}`
- Returns an authorized download stream for app packages, files, and certificates.

## Admin APIs

### Devices

- `GET /api/v1/admin/devices`
- `POST /api/v1/admin/devices`
- `GET /api/v1/admin/devices/{deviceId}`
- `PATCH /api/v1/admin/devices/{deviceId}`
- `DELETE /api/v1/admin/devices/{deviceId}`

### Groups And Policies

- `GET /api/v1/admin/groups`
- `POST /api/v1/admin/groups`
- `GET /api/v1/admin/policies`
- `POST /api/v1/admin/policies`
- `PUT /api/v1/admin/policies/{policyId}`

### Apps

- `GET /api/v1/admin/apps`
- `POST /api/v1/admin/apps`
- `POST /api/v1/admin/apps/{appId}/versions`
- `POST /api/v1/admin/apps/{versionId}/publish`

### Files And Certificates

- `GET /api/v1/admin/files`
- `POST /api/v1/admin/files`
- `GET /api/v1/admin/certificates`
- `POST /api/v1/admin/certificates`

### Commands And Push

- `POST /api/v1/admin/push`
- `POST /api/v1/admin/commands`
- `GET /api/v1/admin/commands`
- `GET /api/v1/admin/commands/{commandId}`

### Logs, Audit, Messaging

- `GET /api/v1/admin/audit`
- `GET /api/v1/admin/device-logs`
- `GET /api/v1/admin/messages`

### Plugins

- `GET /api/v1/admin/plugins`
- `GET /api/v1/admin/plugins/{pluginId}/settings`
- `PUT /api/v1/admin/plugins/{pluginId}/settings`

## Canonical Payload Shapes

### Error

```json
{
  "error": {
    "code": "device_not_found",
    "message": "Device not found",
    "details": {}
  }
}
```

### Enrollment Response

```json
{
  "deviceId": "string",
  "deviceSecret": "string",
  "policyVersion": "string",
  "config": {},
  "apps": [],
  "files": [],
  "certificates": [],
  "commands": []
}
```

### Config Snapshot

```json
{
  "version": "string",
  "device": {},
  "policy": {},
  "apps": [],
  "files": [],
  "certificates": [],
  "commands": [],
  "signature": "string"
}
```

### Command Record

```json
{
  "id": "string",
  "type": "reboot",
  "status": "queued",
  "payload": {},
  "expiresAt": "2026-04-23T00:00:00Z"
}
```

## Contract Decisions

- Enrollment returns the initial device secret, not a long-lived admin token.
- Config snapshots are immutable records with version numbers.
- Commands are append-only until acked or expired.
- File and app artifacts are referenced by checksum and version, not by mutable URLs alone.
- Admin APIs never reuse device authentication headers.
- Device sync must tolerate empty command lists without treating that as an error.
- Download URLs are server-authorized and short-lived when object storage is exposed directly.

## Response Rules

- Unknown device: `404` with a stable error code.
- Unauthorized request: `401` without leaking account existence.
- Forbidden request: `403` with no additional privilege hints.
- Validation error: `400` with field-level detail.
- Conflict: `409` for duplicate enrollment or stale version updates.

## Versioning Rules

- Additive changes stay within `/api/v1`.
- Breaking contract changes require `/api/v2`.
- Backward-compatible payload extension is preferred over field removal.
- Device and admin APIs may evolve independently only if their data model stays compatible.
