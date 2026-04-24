# API Contracts

## API Rules

- Version all external endpoints under `/api/v1`.
- Keep public, device, and admin APIs separate.
- Additive changes stay within `/api/v1`.
- Breaking contract changes require `/api/v2`.
- Return machine-readable error objects with a stable top-level `error` object.
- The error object must include `code`, `message`, and `details`.
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

- Creates or binds a device during first enrollment.
- Accepts an enrollment token, device identity hints, and optional bootstrap fields.
- Returns the device identity, device secret, initial config snapshot, and any initial artifact or command data needed for first sync.
- `POST /api/v1/enrollment` binds the device during first enrollment and returns the device secret.
- Enrollment token lifecycle routes live under `/api/v1/enrollment/tokens` for issuance, validation, consumption, and revocation.
- The issued token is one-time use and is stored only as a hash server-side.
- Token validation and consumption are public device-side calls that operate on the raw bootstrap token string.

### QR Payload

- Produces the Android managed-provisioning QR code image and the canonical JSON payload used to generate it.
- The QR payload carries the device admin component, APK download URL and checksum, and the admin extras bundle used by the launcher.
- The feature surface lives under `/api/v1/enrollment/qr` for PNG output and `/api/v1/enrollment/qr/json` for the raw payload.
- See the canonical payload shape below for the exact JSON object.

### Device Config Sync

- Returns the signed policy snapshot plus the app, file, certificate, and command state that the device needs to apply.
- The sync path is deterministic and idempotent.
- The POST variant is used when the device needs to send telemetry in the same round trip.

### Telemetry Upload

- Accepts device heartbeat, battery, network, location, and app-state telemetry.
- Telemetry writes feed device health tracking and operational visibility.
- The live upload surface uses `POST /api/v1/devices/{deviceId}/telemetry` with the device secret in `X-XMDM-Device-Secret`.

### Log Upload

- Accepts batch uploads of app and device logs.
- Log uploads are separate from telemetry so large log payloads do not block config sync.

### Message Polling

- Provides a fallback read path for pending device messages when MQTT is not active.

### Artifact Download

- Returns authorized download streams for app packages, files, and certificates.
- Artifact access is mediated by the server rather than exposing object storage directly.

## Admin APIs

- The admin console manages users, roles, groups, policies, devices, and the operational admin workflow.
- The console contract and payload shapes live in [../contracts/admin-console.md](../contracts/admin-console.md).
- The live versioned admin session surface uses `/api/v1/admin/...`.
- The live versioned admin resource surface uses `/api/v1/...`.
- The console wrapper can still mount the same contract under `/admin/...` when needed.
- All `/api/v1/admin/...` and `/api/v1/...` admin endpoints should follow the versioning and error rules below.

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

- `error.code` is a stable machine-readable string.
- `error.message` is a human-readable summary safe to show to operators.
- `error.details` carries field-level or request-specific context when available.
- Success responses do not wrap payloads in an `error` object.

### Enrollment Response

```json
{
  "deviceId": "string",
  "deviceSecret": "string",
  "status": "enrolled",
  "config": {
    "version": "string",
    "device": {},
    "policy": {},
    "apps": [],
    "files": [],
    "certificates": [],
    "commands": [],
    "signature": "string"
  }
}
```

- The `config` payload is a signed config snapshot.
- The signature is computed over the canonical JSON representation with `signature` blanked and keyed by the device secret.

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

### QR Enrollment Payload

```json
{
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME": "com.xmdm.launcher/.AdminReceiver",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": "https://cdn.example/launcher.apk",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM": "base64sha256",
  "android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED": true,
  "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE": {
    "com.xmdm.BASE_URL": "https://mdm.example",
    "com.xmdm.SERVER_PROJECT": "rest",
    "com.xmdm.ENROLLMENT_TOKEN": "token",
    "com.xmdm.DEVICE_ID_USE": "serial",
    "com.xmdm.CUSTOMER": "Acme",
    "com.xmdm.GROUP": "field"
  }
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
- Enrollment QR generation returns a PNG QR image; `/api/v1/enrollment/qr/json` returns the canonical Android provisioning payload for clients that need the raw JSON.
- `/api/v1/enrollment/tokens` issues a token for a given TTL; `/api/v1/enrollment/tokens/validate` and `/api/v1/enrollment/tokens/consume` operate on the one-time token string; `DELETE /api/v1/enrollment/tokens/{id}` revokes an active token.

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
