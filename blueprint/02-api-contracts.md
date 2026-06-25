# API Contracts

## API Rules

- Version external device/admin JSON APIs under `/api/v1`.
- Keep browser dashboard routes under `/admin`.
- Use JSON for API request/response bodies unless streaming an artifact.
- Return machine-readable error objects for API errors.
- Use device authentication for device routes after enrollment.
- Use dashboard sessions, CSRF protection, and RBAC checks for browser
  mutations.
- Additive changes stay within `/api/v1`; incompatible API changes require a
  new version.

## Authentication Contracts

### Admin Auth

- The dashboard uses a session cookie.
- The built-in permission catalog currently contains `admin.read`,
  `admin.write`, `devices.read`, and `devices.write`.
- Browser mutations keep CSRF and permission checks enabled.

### Device Auth

- Provisioning starts from a one-time enrollment token.
- Enrollment issues a per-device secret.
- Device routes use the stable device identifier plus the device secret.
- Device secrets are not interchangeable across devices.

## Device Contracts

- Enrollment consumes a one-time token and returns device credentials.
- Config sync returns a signed snapshot that the launcher verifies before
  applying state.
- Telemetry ingestion, device logs, device info, command polling, command
  acknowledgement, and artifact downloads require device authentication.
- Command identifiers are idempotency keys across MQTT and polling.
- Artifact metadata includes checksums, and the launcher verifies downloaded
  content before applying it.

Operator-facing API and dashboard behavior belongs in `docs/`, not in this
blueprint.

## Plugin Contracts

- Plugins are registered statically at startup.
- Plugins may contribute admin routes, admin device actions, command types,
  permissions, and migrations.
- Plugin command types extend the catalog but still use the core command queue,
  delivery, expiry, and acknowledgement behavior.
- Core must not embed premium feature business logic.

## Response Rules

- Unknown resource: return not found without exposing unrelated tenant data.
- Unauthorized request: reject without leaking credential validity.
- Forbidden request: reject without additional privilege hints.
- Validation error: return field/request details where safe.
- Conflict: return conflict for duplicate or stale mutations.

## Config Snapshot Contract

The signed snapshot contains:

- version
- runtime settings
- device identity details
- policy state
- managed apps
- managed files
- certificates
- signature

The signature is computed over canonical JSON with the signature field blanked
and keyed by the device secret.
