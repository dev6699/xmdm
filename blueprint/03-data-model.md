# Data Model

## Data Model Decisions

- PostgreSQL is the transactional store.
- The schema is tenant-aware while the current runtime is single-tenant.
- Admin-managed records use active/retired style state rather than hard deletion
  where history matters.
- Binary payloads are represented as artifacts and stored outside PostgreSQL.
- Device-facing state is derived from persisted records into signed config
  snapshots.

## Core Entities

| Entity | Purpose |
| --- | --- |
| Tenant | Account boundary for the single active deployment tenant. |
| User | Admin identity. |
| Role | Permission bundle for admin users. |
| Device | Managed Android endpoint with a device secret hash and policy assignment. |
| Group | Device grouping. |
| Policy | Runtime policy, kiosk settings, restrictions, and content assignments. |
| App | Managed Android package record. |
| App Version | Immutable app package version tied to an artifact. |
| File | Uploaded file metadata tied to an artifact. |
| Managed File | Device-path assignment for a file, with optional variable replacement. |
| Certificate | Managed certificate payload tied to an artifact. |
| Artifact | Object-storage metadata, checksum, and size. |
| Command | Admin-issued device command and terminal result. |
| Audit Event | Immutable admin/system event record. |
| Enrollment Token | One-time bootstrap credential. |
| Device Telemetry | Device-authenticated heartbeat and operational metrics. |
| Device Log | Structured launcher log entry. |
| Device Info | Device inventory and runtime snapshot. |

## Relationship Rules

- A tenant owns users, roles, devices, groups, policies, apps, files,
  certificates, commands, audit events, and uploaded device records.
- A device belongs to one tenant and may belong to multiple groups.
- A device may reference one active policy.
- A policy may reference managed apps, managed files, and certificates.
- An app may have multiple immutable versions.
- A managed file references one uploaded file and one target device path.
- A command belongs to one device.
- Device telemetry records, logs, and device info records belong to one device.

## Lifecycle Rules

- Enrollment tokens move through issued, consumed, expired, or revoked states.
- Admin-managed records are generally active or retired.
- App versions are published or retired.
- Commands start queued and end acked, failed, or expired.
- Device status is persisted and used to control authentication eligibility.

## Invariants

- Device secrets are stored as hashes, not plaintext.
- Enrollment tokens are stored as hashes, not plaintext.
- Config snapshots are signed before the launcher applies them.
- Published artifacts are referenced by checksum and metadata.
- Commands cannot be acknowledged by a different device.
- Tenant-scoped reads and writes must not cross tenant boundaries.
