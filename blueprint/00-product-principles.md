# Product Principles

## Product Statement

XMDM is a self-hosted Android device-management control plane. It lets an
administrator enroll Android devices, publish the launcher app, distribute apps,
files, and certificates, apply kiosk and package policy, collect device info and
logs, store telemetry records, and send approved commands from the dashboard.

The product scope in this repository is Android device management.

## Scope Boundary

### In Scope

- Android launcher runtime
- Go server and server-rendered admin dashboard
- PostgreSQL transactional state
- S3-compatible artifact storage
- MQTT push plus HTTP polling for command delivery
- Dashboard-first operations
- Static plugin extension points for optional modules
- Single-tenant self-hosted deployment
- Tenant-aware schema while the runtime remains single-tenant

### Scope Limits

- iOS, macOS, Windows, and Linux endpoint apps
- Multi-tenant SaaS billing or self-service signup
- Core remote desktop or screen mirroring
- Consumer mobile app workflows
- SPA-only admin dashboard
- Runtime loading of arbitrary third-party plugin binaries
- Universal Android/OEM compatibility claims

## Premium Extension Boundary

Open-core XMDM owns only the generic extension surface:

- static plugin registration at server startup
- plugin metadata
- plugin-provided admin actions
- plugin command type registration
- safe behavior when optional plugins are omitted

Premium repositories own premium feature implementation, release assets, and
operator workflows. Premium code must live outside this open-core repository.
Core enrollment, sync, command delivery, and dashboard operation must continue
when no premium plugin is present.

## Operating Principles

- Prefer implementation-backed documentation over planning notes.
- Prefer explicit limitations over broad product claims.
- Prefer server-owned policy truth over device-local authority.
- Prefer idempotent device workflows where duplicate delivery can happen.
- Prefer audit records for privileged operations.
- Prefer recoverable self-hosted operation over hidden managed-service
  assumptions.

## Glossary

- **Tenant**: the account boundary. XMDM starts with one active tenant while the schema remains tenant-aware.
- **Device**: one managed Android endpoint.
- **Enrollment**: binding a device to the control plane and issuing device credentials.
- **Policy**: the runtime rules applied to a device at sync time.
- **App**: a managed application record and its versions.
- **File**: a managed payload distributed to a device.
- **Certificate**: a managed certificate payload distributed to a device.
- **Command**: an admin-approved remote action for a device.
- **Artifact**: a binary payload that must be downloaded and verified.
- **Plugin**: an optional module registered with the server at startup.
- **Admin**: a human operator using the web dashboard.

## Success Criteria

XMDM is behaving as intended when:

- a clean Android device can enroll without manual database work
- the server can deliver config and managed content to enrolled devices
- the launcher can recover from reboot and reconnect without re-enrollment
- implemented privileged admin actions are visible through audit and dashboard
  surfaces
- artifact delivery is checksum-verified
- command delivery remains recoverable through polling when MQTT is unavailable
