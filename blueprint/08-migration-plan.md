# Migration Plan

## Strategy

Build XMDM in vertical slices so each phase can be demoed end to end.

The phases are ordered by dependency, not by UI convenience.

## Phase 0 - Foundation

- Repository layout
- Toolchain setup
- Shared glossary
- API versioning rules
- CI skeleton
- Local development environment

Exit criteria:

- Repo builds in both languages.
- Docs and contracts are stable enough for implementation.
- The checklist roadmap can start consuming real progress.

## Phase 1 - Identity And Core Data

- Admin auth
- Tenant-aware schema
- Users, roles, and permissions
- Device model
- Group model
- Policy model

Exit criteria:

- Admin can log in.
- CRUD exists for core objects.
- Migrations are repeatable.
- Audit events are emitted for critical writes.

## Phase 2 - Device Enrollment And Sync

- QR enrollment
- Bootstrap payload
- Device secret issuance
- Signed config snapshots
- Device heartbeat
- Basic telemetry upload

Exit criteria:

- Fresh device can enroll and sync successfully.
- Device identity is stable after reboot.
- The server can recover from duplicate enrollment attempts cleanly.

## Phase 3 - Content Delivery

- App management
- App version uploads
- File delivery
- Certificate delivery
- Checksum verification
- Download retry handling

Exit criteria:

- Device downloads and installs content from server-managed artifacts.
- Server can distinguish metadata issues from transfer issues.

## Phase 4 - Push And Commands

- MQTT delivery
- HTTP polling fallback
- Command queue
- Command ack flow
- Broadcast and targeted messages

Exit criteria:

- Admin can dispatch a command and observe device ack.
- The same command works through MQTT and polling fallback.

## Phase 5 - Enterprise Controls

- Kiosk mode
- App allow/block rules
- Foreground enforcement
- Device logs
- Device info export
- Audit trail
- Messaging module
- Image upload module

Exit criteria:

- Enterprise features are visible in the console and enforced on devices.
- The server and agent expose explicit failure states for unsupported device capabilities.

## Phase 6 - Hardening

- Security review
- Performance review
- Load testing
- Backup and restore drills
- Documentation completeness review

Exit criteria:

- Production readiness sign-off.
- Operational runbooks are complete enough for a third party to operate the system.

## Porting Rule

- Do not port code line-for-line from the reference repo.
- Port behavior and contracts, then implement them with Go and Kotlin idioms.
- Keep implementation order aligned with these phases.
- If a phase depends on a missing contract, stop and document the contract before coding around it.
