# Product Principles

## Product Statement

XMDM is a corporate Android device-management platform that lets an administrator enroll devices, push configuration, install and remove apps, distribute files, enforce kiosk restrictions, collect logs and telemetry, and execute approved remote commands.

The product is a control plane, not a general-purpose endpoint management suite. Android is the only device target in v1.

## Product Goals

- Manage Android devices at scale with one control plane.
- Support kiosk and non-kiosk device modes.
- Ship enterprise-grade features in the first usable release.
- Keep the system self-hosted, understandable, and recoverable by operators.
- Make the agent resilient to network failures, reboot cycles, and partial setup state.
- Make every server-side action traceable.

## Scope Boundary

### In Scope

- Kotlin Android agent
- Go server, workers, and admin console
- PostgreSQL as the source of truth
- Object storage for artifacts
- MQTT plus HTTP polling for push delivery
- Plugin architecture for optional modules
- Single-tenant deployment in the first release
- Tenant-aware schema for future expansion

### Out of Scope

- iOS, macOS, Windows, and Linux endpoints
- Multi-tenant SaaS billing and self-service signup
- Live remote desktop or screen mirroring
- Consumer mobile app
- A separate frontend stack in v1
- Runtime plugin loading from arbitrary third-party binaries

## Technical Decisions

### Client

- Use Kotlin for the Android agent.
- Use Android Jetpack for background work, persistence, and lifecycle management.
- Use XML and ViewBinding for the launcher UI.
- Use a local state store so the device can survive offline and reboot cycles.

### Server

- Use Go for HTTP services, workers, and scheduled jobs.
- Use PostgreSQL for transactional state.
- Use object storage for large artifacts and exports.
- Use server-rendered HTML with minimal JavaScript for the admin console.

### Protocols

- Use HTTPS for all HTTP APIs.
- Use a per-device secret after enrollment.
- Sign device config snapshots.
- Verify checksums for every downloadable artifact.
- Use MQTT only for push delivery and not for the general data plane.

### Deployment

- Support Docker Compose as the baseline self-hosted deployment.
- Allow Kubernetes and systemd wrappers later, but do not optimize for them first.

## Rejected Alternatives

- Java or Kotlin Multiplatform for the agent runtime: rejected because the Android-specific surface is the dominant requirement.
- A microservices split on day one: rejected because it adds operational overhead without reducing product risk.
- A SPA-only admin console: rejected because it creates an unnecessary frontend platform dependency.
- WebSockets as the only push channel: rejected because MQTT and polling both matter for device reliability.
- No tenant separation in schema: rejected because it blocks future SaaS expansion and makes data ownership ambiguous.

## Feature Commitments

- QR enrollment
- adb/manual fallback enrollment
- Device-owner provisioning
- Kiosk and non-kiosk operation
- App install and uninstall
- File distribution
- Certificate distribution
- Configuration sync
- Device info and log collection
- Audit trail
- Messaging and commands
- Plugin-based extensions

## Priority Model

### Must Have

- Enrollment
- Authentication
- Device sync
- App and file delivery
- Push delivery
- Kiosk control
- Audit logging

### Should Have

- Messaging
- Device telemetry
- Device logs
- Update management
- Plugin settings

### Later

- Multi-tenant SaaS
- Partner integrations
- Advanced analytics
- Device recovery tooling

## Operating Principles

- Prefer explicit failure over silent partial success.
- Prefer idempotent APIs over stateful session chaining.
- Prefer deterministic sync snapshots over mutable server-side interpretation during device poll.
- Prefer local persistence on device over repeated server round-trips.
- Prefer audit records for privileged operations even when the main operation fails.
- Prefer operational clarity over abstract generality.

## Glossary

- **Tenant**: the account boundary. XMDM starts with one tenant but keeps the schema tenant-aware.
- **Device**: one managed Android endpoint.
- **Enrollment**: binding a device to the control plane and issuing device credentials.
- **Policy**: the set of rules applied to a device at sync time.
- **App**: the managed application record and its versions.
- **File**: any managed binary or document distributed to a device.
- **Command**: an admin-approved remote action.
- **Artifact**: any binary payload that must be downloaded and verified.
- **Plugin**: an optional module that extends behavior or UI.
- **Admin**: a human operator using the web console.

Use these nouns consistently across the blueprint and contracts. Prefer singular names for entity types and keep compound phrases as modifiers around the canonical noun, for example `policy snapshot`, `app version`, and `file delivery`.

## Success Criteria

XMDM is behaving as intended when:

- a clean device can enroll without manual database work
- the server can push config and content changes to devices
- the device can recover from reboot and reconnect without re-enrollment
- admin actions are auditable
- artifact delivery is checksum-verified
- kiosk restrictions are enforced from policy, not local convenience
