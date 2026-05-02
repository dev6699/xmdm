# Data Model

## Data Model Decisions

- PostgreSQL is the primary transactional store.
- Every tenant-aware table includes `tenant_id`.
- Single-tenant v1 uses exactly one active tenant row.
- Use soft delete for admin-managed entities that may appear in audit or history views.
- Use immutable version rows for apps, files, certificates, and generated manifests.
- Model state transitions explicitly instead of inferring them from timestamps alone.

## Core Entities

| Entity | Purpose | Key Fields |
| --- | --- | --- |
| Tenant | Account boundary | `id`, `name`, `status` |
| User | Admin identity | `id`, `tenant_id`, `email`, `password_hash`, `role_id`, `status` |
| Role | Permission bundle | `id`, `tenant_id`, `name`, `permissions` |
| Device | Managed Android endpoint | `id`, `tenant_id`, `device_id`, `secret_hash`, `status`, `policy_id` |
| Group | Device grouping | `id`, `tenant_id`, `name`, `status` |
| Policy | Runtime device policy | `id`, `tenant_id`, `name`, `version`, `kiosk_mode`, `kiosk_app_package`, `restrictions_json` |
| App | Logical app record | `id`, `tenant_id`, `package_name`, `name`, `status` |
| AppVersion | Immutable package version | `id`, `app_id`, `version_name`, `version_code`, `artifact_id`, `checksum` |
| File | Immutable file record | `id`, `tenant_id`, `name`, `artifact_id`, `checksum`, `mime_type` |
| Certificate | Managed cert bundle | `id`, `tenant_id`, `name`, `artifact_id`, `checksum` |
| Command | Admin-issued action | `id`, `tenant_id`, `device_id`, `type`, `payload_json`, `status` |
| PushMessage | Delivery unit | `id`, `tenant_id`, `device_id`, `type`, `payload_json`, `status` |
| AuditEvent | Immutable audit trail | `id`, `tenant_id`, `actor_id`, `action`, `resource_type`, `resource_id` |
| DeviceTelemetry | Heartbeat and metrics | `id`, `device_id`, `observed_at`, `payload_json` |
| DeviceLog | Agent-side log entry | `id`, `device_id`, `level`, `message`, `created_at` |
| ImageAsset | Device-uploaded image | `id`, `tenant_id`, `device_id`, `artifact_id`, `kind`, `checksum` |
| PluginSetting | Plugin config | `id`, `tenant_id`, `plugin_id`, `scope`, `value_json` |
| EnrollmentToken | One-time bootstrap token | `id`, `tenant_id`, `token_hash`, `status`, `expires_at` |
| Artifact | Physical blob reference | `id`, `tenant_id`, `storage_key`, `checksum`, `size_bytes`, `mime_type` |
| ConfigSnapshot | Immutable sync payload | `id`, `tenant_id`, `device_id`, `version`, `signature` |

## Relationship Rules

- A tenant has many users, devices, groups, policies, apps, files, certificates, commands, and audit events.
- A device belongs to exactly one tenant and can belong to multiple groups.
- A device has one active policy snapshot at a time.
- An app has many versions.
- A policy references app versions, file assets, certificates, and command defaults.
- A command belongs to one device and is eventually acked or expired.
- A push message may map to a command or a device notification.
- An artifact may be referenced by multiple logical objects if the checksum is identical and the lifecycle rules allow it.

## Lifecycle States

### Device

- `pending`
- `enrolled`
- `active`
- `locked`
- `suspended`
- `retired`
- `wiped`

### App Version

- `draft`
- `uploaded`
- `verified`
- `published`
- `deprecated`
- `deleted`

### File Asset

- `staged`
- `verified`
- `published`
- `archived`

### Command

- `queued`
- `sent`
- `delivered`
- `acked`
- `failed`
- `expired`

### Push Message

- `queued`
- `inflight`
- `delivered`
- `acked`
- `failed`
- `expired`

### Enrollment Token

- `issued`
- `consumed`
- `expired`
- `revoked`

## Storage Boundaries

- PostgreSQL stores metadata, relationships, and state transitions.
- Object storage stores binary artifacts and exports.
- The artifact table only stores metadata and object keys.
- Checksums are stored alongside every artifact and verified before install.
- Signed manifests reference object keys, not mutable filenames.

## Indexing Rules

- Device lookup by `tenant_id + device_id`.
- Device lookup by serial, IMEI, or enrollment token.
- App lookup by `tenant_id + package_name`.
- Command lookup by `device_id + status`.
- Push lookup by `device_id + status`.
- Audit lookup by `tenant_id + created_at`.
- Artifact lookup by `tenant_id + checksum`.

## Audit Rules

- All admin mutating actions create audit events.
- All device commands create audit events.
- Enrollment and de-enrollment create audit events.
- Failed authorization attempts create audit events when they are security relevant.

## Invariants

- A device secret is never stored in plaintext.
- A config snapshot version must increase when policy-relevant content changes.
- A published app version is immutable.
- A command cannot be delivered after it expires.
- A tenant must never read another tenant's artifact metadata.
