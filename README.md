# XMDM Project Status

This repository is the working home for XMDM.

## Roadmap Snapshot

Roadmap source: [blueprint/09-roadmap-checklist.md](blueprint/09-roadmap-checklist.md)
Snapshot last updated: 2026-05-23

### M0 - Foundation

| Item | State |
| --- | --- |
| M0-01 Repository Layout | ☑ |
| M0-02 Glossary And Naming | ☑ |
| M0-03 API Versioning Rules | ☑ |
| M0-04 Stack Selection | ☑ |
| M0-05 CI Skeleton | ☑ |
| M0-06 Local Dev Setup | ☑ |
| M0-07 Deployment Model | ☑ |
| M0-08 Threat Model | ☑ |

### M1 - Core Backend

| Item | State |
| --- | --- |
| M1-01 Admin Auth | ☑ |
| M1-02 RBAC | ☑ |
| M1-03 Core Schema | ☑ |
| M1-04 Core CRUD | ☑ |
| M1-05 Migration Tooling | ☑ |
| M1-06 Audit Capture | ☑ |
| M1-07 Admin E2E | ☑ |
| M1-08 Plugin Isolation | ☑ |

### M2 - Enrollment And Sync

| Item | State |
| --- | --- |
| M2-01 QR Enrollment | ☑ |
| M2-02 Enrollment Tokens | ☑ |
| M2-03 Device Secret | ☑ |
| M2-04 Signed Config | ☑ |
| M2-05 Telemetry Upload | ☑ |
| M2-06 State Transitions | ☑ |
| M2-07 Enrollment E2E | ☑ |
| M2-08 Reconnect E2E | ☑ |

Note: `M2-07 Enrollment E2E` and `M2-08 Reconnect E2E` are complete on the enrolled physical device.

### M3 - Agent Foundation

| Item | State |
| --- | --- |
| M3-01 Kotlin Project | ☑ |
| M3-02 Local Persistence | ☑ |
| M3-03 Bootstrap Parsing | ☑ |
| M3-04 Retry Logic | ☑ |
| M3-05 Recovery Diagnostics | ☑ |
| M3-06 Reboot Survival | ☑ |
| M3-07 Polling Fallback | ☑ |

Note: `M3-01 Kotlin Project` is complete in [app/](app), `assembleDebug` succeeds, `M3-02 Local Persistence` has a DataStore-backed state store plus unit coverage and now survives a physical-device reboot check, `M3-03 Bootstrap Parsing` now accepts and persists canonical provisioning JSON via QR or ADB/manual `base64url:` launch data, the Android app now flows from bootstrap into enrollment so the device secret and initial signed config snapshot are fetched from the backend, `M3-04 Retry Logic` adds a contract-driven config sync engine with retry and signature verification and now refreshes the signed device config from `GET /api/v1/devices/{deviceId}/config` after provisioning, `M3-05 Recovery Diagnostics` captures bootstrap and enrollment failures in launcher diagnostics and uploaded device logs when identity is available, `M3-06 Reboot Survival` now has a physical-device reboot verification with the persisted state file checksum unchanged and the launcher UI restoring bootstrap, identity, and policy cache after reboot, and `M3-07 Polling Fallback` is verified by the sync engine falling back from the primary polling path to the secondary server URL when the primary path is unavailable.

### M4 - Content Delivery

| Item | State |
| --- | --- |
| M4-01 App Management | ☑ |
| M4-02 File Storage | ☑ |
| M4-03 Certificates | ☑ |
| M4-04 Checksum Verification | ☑ |
| M4-05 App Install Flow | ☑ |
| M4-06 File Download Flow | ☑ |
| M4-07 Content E2E | ☑ |
| M4-08 Artifact Cleanup | ☑ |

Note: `M4-01 App Management` is complete with app CRUD plus immutable version upload and publish support, scan-first app list/detail pages, latest published version visibility in the list, and server-assigned version metadata; `M4-02 File Storage` is complete with multipart file upload plus artifact metadata persistence, `M4-03 Certificates` is complete with certificate upload/distribution and signed config inclusion for active certificates, `M4-04 Checksum Verification` is complete with server-side content digest validation for file, certificate, and referenced app artifacts plus an Android-side artifact checksum verifier, `M4-05 App Install Flow` is complete with server-side app artifact streaming, signed snapshot app entries, Android install/uninstall coordination for managed packages, live download progress UI, and a documented reprovision runbook, `M4-06 File Download Flow` is complete with device-authenticated file artifact downloads, a separate managed-file creation flow, server-rendered file content, and persisted managed-file state on the launcher, `M4-07 Content E2E` is complete with adb-backed physical-device verification of managed app install plus server-rendered managed file delivery, and `M4-08 Artifact Cleanup` is complete with orphan artifact detection plus a dedicated cleanup command that retires and purges unreferenced artifact rows and blobs.

### M5 - Push And Commands

| Item | State |
| --- | --- |
| M5-01 MQTT Transport | ☑ |
| M5-02 Polling Fallback | ☑ |
| M5-03 Fan-Out Queue | ☑ |
| M5-04 Device Acks | ☑ |
| M5-05 Admin Targeting | ☑ |
| M5-06 Command E2E | ☑ |
| M5-07 Broker Outage Recovery | ☑ |

Note: `M5-01 MQTT Transport` is complete with the server constructing an internal MQTT publisher in [server/cmd/server/main.go](server/cmd/server/main.go) by default, using `127.0.0.1:1883` unless `XMDM_MQTT_ADDRESS` overrides it, publishing command envelopes to `devices/{deviceId}/commands` through [server/internal/push](server/internal/push), and automatically provisioning device MQTT credentials through the enrollment and retire flows with Mosquitto dynamic security in [infra/mosquitto/mqtt-security.md](infra/mosquitto/mqtt-security.md). `M5-02 Polling Fallback` is complete as of 2026-04-25 with `GET /api/v1/devices/{deviceId}/commands` returning pending commands from PostgreSQL when MQTT is unavailable. `M5-03 Fan-Out Queue` is complete as of 2026-04-25 with server-side command enqueueing that expands device, group, and broadcast targets into per-device rows, publishes each command to MQTT immediately after enqueue, and leaves the row available for polling fallback if publish fails. `M5-04 Device Acks` is complete as of 2026-04-25 with the launcher subscribing to MQTT when the signed config snapshot provides a broker address, otherwise polling pending commands, executing supported ones, and device-authenticated command acknowledgement updating terminal command state through `POST /api/v1/devices/{deviceId}/commands/{commandId}/ack`. The launcher currently supports a lightweight `ping` command plus `reboot`. `M5-05 Admin Targeting` is complete as of 2026-04-25 with the `POST /api/v1/admin/commands` contract for device, group, and broadcast targeting. `M5-06 Command E2E` is complete as of 2026-04-25 with adb-backed physical-device coverage for `ping` over both MQTT and polling plus expiry handling that transitions commands to `expired` when they outlive `expiresAt`. `M5-07 Broker Outage Recovery` is complete as of 2026-04-28 with physical-device coverage that proves the launcher falls back to polling during MQTT outage and resumes MQTT transport after broker recovery.

### M6 - Enterprise Controls

| Item | State |
| --- | --- |
| M6-01 Kiosk Enforcement | ☑ |
| M6-02 Package Rules | ☑ |
| M6-03 Foreground Enforcement | Not Planned |
| M6-04 Device Logs | ☑ |
| M6-05 Device Info | ☑ |
| M6-06 Messaging And Audit | ☑ |
| M6-07 Image Upload | Not Planned |
| M6-08 Enterprise E2E | ☑ |
| M6-09 Policy Gaps | ☑ |

Note: `M6-01 Kiosk Enforcement` now also supports a policy-defined kiosk app package, kiosk keep-awake behavior, stay-awake-while-plugged-in device-owner control, best-effort boot unlock for kiosk devices without password policy, a required kiosk exit passcode hash for kiosk policies, a device-local persistent admin menu with enter/exit/sync actions plus the existing `exit_kiosk` command path, and a physical-device e2e check for the stay-on global setting, explicitly releases the launcher before handing off to the kiosk app, and keeps the kiosk enforcement item marked complete.
Note: `M6-06 Messaging And Audit` is complete as of 2026-05-01 with API endpoints for command listing, command creation, and audit event listing; a separate admin UI is still future work.
Note: `M6-08 Enterprise E2E` is complete as of 2026-05-01 with device-backed coverage for kiosk enforcement, package rules, managed files and apps, device logs, device info, and API-backed messaging/audit flows; `M6-07 Image Upload` remains not planned.
Note: `M6-09 Policy Gaps` is complete as of 2026-05-01 with the enterprise enforcement matrix documented in [docs/agent-app-lifecycle.md](docs/agent-app-lifecycle.md).

### M7 - Hardening

| Item | State |
| --- | --- |
| M7-01 Rate Limiting | ☑ |
| M7-02 Security Tests | ☑ |
| M7-03 Load Tests | ☑ |
| M7-04 Backup And Restore | ☑ |
| M7-05 Observability | ☑ |
| M7-06 DR And Rollback Docs | ☑ |
| M7-07 Release Candidate | ☑ |
| M7-08 Cleanup Pass | ☑ |

Note: `M7-01 Rate Limiting` is complete as of 2026-05-12 with server-side token-bucket protection for repeated admin login, enrollment, and admin command fan-out traffic, returning `429 Too Many Requests` with `Retry-After` when a bucket is exhausted.
Note: `M7-02 Security Tests` is complete as of 2026-05-12 with coverage for invalid admin credentials, browser-form CSRF protection on admin session routes, signed config snapshot tampering, and admin command authorization failures.
Note: `M7-03 Load Tests` is complete as of 2026-05-12 with concurrent HTTP load coverage for device config sync, admin command push, command polling and acknowledgements, managed-file, app, and certificate artifact downloads, telemetry upload, and a mixed workload pass in `server/internal/api/v1/load_test.go`.
Note: `M7-04 Backup And Restore` is complete as of 2026-05-12 with the documented restore drill in `infra/backup-restore-drill.sh` and `docs/backup-restore-drill.md`, which backs up the local test database, restores it into a fresh database, and compares core table counts.
Note: `M7-05 Observability` is complete as of 2026-05-12 with request logging, request IDs, trace headers, Prometheus-style `/metrics`, and route normalization in `server/internal/observability` plus `docs/observability.md`.
Note: `M7-06 DR And Rollback Docs` is complete as of 2026-05-12 with incident recovery and release rollback guidance in `docs/disaster-recovery-and-rollback.md`, including restore order, object storage recovery, rollback rules, and verification checks.
Note: `M7-07 Release Candidate` is complete as of 2026-05-12 with the staging-device release-candidate checklist in `docs/release-candidate-checklist.md` and the related e2e coverage map in `server/e2e/README.md`.
Note: `M7-08 Cleanup Pass` is complete as of 2026-05-12 with the hardening cleanup pass in `server/cmd/cleanup-pass`, which expires stale enrollment tokens and commands and purges orphan artifact records, plus the operator runbook in `docs/cleanup-pass.md`.

### M8 - CLI Tool

| Item | State |
| --- | --- |
| M8-01 CLI Foundation And Configuration | ☑ |
| M8-02 Authentication And Session Management | ☑ |
| M8-03 Resource Listing And Inspection | ☑ |
| M8-04 Core Resource Management | ☑ |
| M8-05 Content Management | ☑ |
| M8-06 Enrollment And Bootstrap | ☑ |
| M8-07 Device, Log, And Audit Inspection | ☑ |
| M8-08 Command Operations | ☑ |
| M8-09 Output Formats And Error Handling | ☑ |
| M8-10 Packaging And Release Documentation | ☑ |

Note: the detailed CLI command tree and output contract live in [docs/admin-operator-story.md](docs/admin-operator-story.md).
Note: `M8-01 CLI Foundation And Configuration` is complete as of 2026-05-09 with a dedicated `cli/` module, Cobra-based command scaffolding, config/profile resolution, a reusable HTTP client wrapper, and versioned help and config validation commands.
Note: `M8-02 Authentication And Session Management` is complete as of 2026-05-09 with `auth login`, `auth whoami`, and `auth logout` session-cookie flows, a local session file, and server-backed login/me/logout requests.
Note: `M8-03 Resource Listing And Inspection` is complete as of 2026-05-09 with read-only `list` and `show` commands for users, roles, groups, policies, apps, files, managed files, certificates, devices, commands, logs, device info, and audit events, plus live-server verification against the seeded Postgres-backed API.
Note: `M8-04 Core Resource Management` is complete as of 2026-05-09 with live-server-backed `create`, `update`, and `retire` flows for users, roles, groups, policies, and devices, plus command-level tests that exercise the real API and database.
Note: `M8-05 Content Management` is complete as of 2026-05-09 with live-server-backed file upload, managed-file creation, app version publish, and certificate upload and retirement commands, plus live CLI tests against the real server and Postgres-backed API.
Note: `M8-06 Enrollment And Bootstrap` is complete as of 2026-05-09 with live-server-backed enrollment token issue, validate, consume, and revoke flows plus QR JSON and PNG bootstrap generation, with command-level tests against the real server and Postgres-backed API.
Note: `M8-07 Device, Log, And Audit Inspection` is complete as of 2026-05-09 with a live-server-backed `devices inspect` support view plus log, device-info, command, and audit aggregation against the seeded Postgres-backed API.
Note: `M8-08 Command Operations` is complete as of 2026-05-09 with live-server-backed command send, list, show, and acknowledgement flows plus command lifecycle tests against the real server and Postgres-backed API.
Note: `M8-09 Output Formats And Error Handling` is complete as of 2026-05-09 with JSON envelope output, readable table/default human output, live-server-backed command tests that decode the envelope contract, and stable CLI exit-code mapping for HTTP and transport failures.
Note: `M8-10 Packaging And Release Documentation` is complete as of 2026-05-09 with install, upgrade, completion, and shell-integration guidance in [cli/README.md](cli/README.md) plus the checked-in example config and Cobra completion support.

### M9 - Admin Dashboard

| Item | State |
| --- | --- |
| M9-01 Dashboard Blueprint And Contract | ☑ |
| M9-02 Console Foundation | ☑ |
| M9-03 Overview Dashboard | ☑ |
| M9-04 Core Resource Views | ☑ |
| M9-05 Core Resource Mutations | ☑ |
| M9-06 Content Dashboard | ☑ |
| M9-07 Enrollment Dashboard | ☑ |
| M9-08 Commands Dashboard | ☑ |
| M9-09 Inspection Dashboard | ☑ |
| M9-10 Dashboard E2E And Docs | ☑ |

Note: `M9-01` through `M9-10` are implemented as of 2026-05-15 with server-rendered `/admin` pages for dashboard overview, session auth, users, roles, groups, scan-first policy list/detail pages with generated restriction inputs, policy-managed app, managed-file, and certificate toggles, identity-style device list and detail pages, device-level enrollment QR generation, scan-first app list/detail pages, scan-first managed-files and certificates pages with detail views, commands list/detail pages, audit visibility, and a root-level `playwright/` workspace that covers the real-server login, identity, device, device-detail QR, group-detail, content, command, and audit flows with inline QR JSON and PNG preview output below the generate button. Device enrollment now uses an immutable device ID separate from the operator-facing display name, managed files now upload in one dashboard step from the managed-files page, and groups now have a scan-first list with a detail page that shows member devices.

### M10 - Premium Add-on Extension Points

| Item | State |
| --- | --- |
| M10-01 Premium Boundary And Blueprint | ☑ |
| M10-02 Plugin Registry Contract | ☑ |
| M10-03 Admin Device Action Hooks | ☑ |
| M10-04 Plugin Command Type Registry | ☑ |
| M10-05 Launcher Companion-App Command Boundary | ☑ |

Note: `M10-01 Premium Boundary And Blueprint` is complete as of 2026-05-23 with the explicit open-core boundary in [blueprint/00-product-principles.md](blueprint/00-product-principles.md), [blueprint/05-server-services.md](blueprint/05-server-services.md), and [blueprint/06-security-and-compliance.md](blueprint/06-security-and-compliance.md).
Note: `M10-02 Plugin Registry Contract` is complete as of 2026-05-23 with the authenticated static plugin registry in [server/internal/plugins/manager.go](server/internal/plugins/manager.go), the admin router wiring in [server/internal/admin/http/routes.go](server/internal/admin/http/routes.go), and the dashboard wiring in [server/internal/admin/http/dashboard.go](server/internal/admin/http/dashboard.go).
Note: `M10-03 Admin Device Action Hooks` is complete as of 2026-05-23 with plugin-provided device actions rendered on the dashboard device detail page in [server/internal/admin/http/dashboard.go](server/internal/admin/http/dashboard.go), filtered by enablement and admin permissions through [server/internal/plugins/manager.go](server/internal/plugins/manager.go), and covered by [server/internal/admin/http/routes_test.go](server/internal/admin/http/routes_test.go) and [server/internal/plugins/manager_test.go](server/internal/plugins/manager_test.go).
Note: `M10-04 Plugin Command Type Registry` is complete as of 2026-05-23 with built-in and plugin command type validation in [server/internal/admin/http/routes.go](server/internal/admin/http/routes.go) and [server/internal/admin/http/dashboard.go](server/internal/admin/http/dashboard.go), the builtin command catalog in [server/internal/commands/catalog.go](server/internal/commands/catalog.go), the plugin registry command-type metadata in [server/internal/plugins/manager.go](server/internal/plugins/manager.go), and CLI preflight validation in [cli/internal/app/command_types.go](cli/internal/app/command_types.go).
Note: `M10-05 Launcher Companion-App Command Boundary` is complete as of 2026-05-23 with the signed companion-app launch command path in [app/src/main/java/com/xmdm/launcher/commands/CompanionAppLaunchCoordinator.kt](app/src/main/java/com/xmdm/launcher/commands/CompanionAppLaunchCoordinator.kt), the launcher command wiring in [app/src/main/java/com/xmdm/launcher/MainActivity.kt](app/src/main/java/com/xmdm/launcher/MainActivity.kt), the built-in command catalog entry in [server/internal/commands/catalog.go](server/internal/commands/catalog.go), and CLI/server command-type validation updates in [cli/internal/app/command_types.go](cli/internal/app/command_types.go) and [server/internal/admin/http/routes_test.go](server/internal/admin/http/routes_test.go).
Note: premium feature implementations and feature-specific roadmap items live outside the open-core repo. This repo tracks only the generic extension points required for those add-ons.

## Blueprint Index

1. [blueprint/00-product-principles.md](blueprint/00-product-principles.md)
2. [blueprint/01-system-architecture.md](blueprint/01-system-architecture.md)
3. [blueprint/02-api-contracts.md](blueprint/02-api-contracts.md)
4. [blueprint/03-data-model.md](blueprint/03-data-model.md)
5. [blueprint/04-device-agent.md](blueprint/04-device-agent.md)
6. [blueprint/05-server-services.md](blueprint/05-server-services.md)
7. [blueprint/06-security-and-compliance.md](blueprint/06-security-and-compliance.md)
8. [blueprint/07-operations.md](blueprint/07-operations.md)
9. [blueprint/08-migration-plan.md](blueprint/08-migration-plan.md)
10. [blueprint/09-roadmap-checklist.md](blueprint/09-roadmap-checklist.md)

## Repo Layout

The repository is organized into a small set of top-level homes that mirror the blueprint boundaries:

- `app/` for the Android agent implementation
- `server/` for the Go control plane, API, workers, and admin console
- `contracts/` for API contracts, payload definitions, and generated interface artifacts
- `infra/` for deployment, local environment, and operational automation
- `docs/` for repo-specific documentation, runbooks, and release-support material
- `playwright/` for browser automation and dashboard end-to-end coverage
