# Implementation Backlog

This document is the ordered execution backlog for XMDM.

Rules:

- each item has a stable backlog ID
- each item has one primary owner role
- each item lists its direct dependency
- each item has a completion signal
- each item stays open until the dependency chain and verification are done

Owner roles used below:

- `PM/Arch` = product or architecture owner
- `BE` = Go backend owner
- `AE` = Android agent owner
- `OPS` = infrastructure and operations owner
- `QA` = test and verification owner

## Milestone M0 - Foundation

### M0-01 Repository Layout

- Owner: `PM/Arch`
- Depends on: none
- Task: finalize the repo layout for `xmdm`, app, server, contracts, infra, and docs.
- Done when: source directories and docs directories are frozen and referenced from the index docs.

### M0-02 Glossary And Naming

- Owner: `PM/Arch`
- Depends on: M0-01
- Task: freeze the glossary and canonical naming for tenant, device, policy, app, file, command, artifact, and plugin.
- Done when: the same terms are used in every doc and contract.

### M0-03 API Versioning Rules

- Owner: `BE`
- Depends on: M0-02
- Task: define API versioning rules and the standard error object format.
- Done when: every API doc uses the same version and error model.

### M0-04 Stack Selection

- Owner: `PM/Arch`
- Depends on: M0-01
- Task: choose the exact Kotlin and Go stack components.
- Done when: the agent and server stacks are named in the blueprint and no alternatives are left unresolved.

### M0-05 CI Skeleton

- Owner: `OPS`
- Depends on: M0-01
- Task: create CI jobs for docs validation, linting, tests, and build verification.
- Done when: docs and code validation jobs are defined in the repo.

### M0-06 Local Dev Setup

- Owner: `OPS`
- Depends on: M0-01, M0-04
- Task: define local development requirements and bootstrap scripts.
- Done when: a new contributor can bring up the stack locally from documented instructions.

### M0-07 Deployment Model

- Owner: `PM/Arch`
- Depends on: M0-02
- Task: approve the single-tenant-first deployment model.
- Done when: the deployment assumption is recorded and no doc contradicts it.

### M0-08 Threat Model

- Owner: `BE`
- Depends on: M0-03, M0-07
- Task: write the initial threat model and security assumptions.
- Done when: auth, enrollment, artifact delivery, and command execution risks are documented.

## Milestone M1 - Core Backend

### M1-01 Admin Auth

- Owner: `BE`
- Depends on: M0-03, M0-08
- Task: implement admin authentication and session management.
- Done when: admin login, logout, and session expiry work end to end.

### M1-02 RBAC

- Owner: `BE`
- Depends on: M1-01
- Task: implement RBAC and permission checks.
- Done when: every privileged admin route is guarded by a permission mapping.

### M1-03 Core Schema

- Owner: `BE`
- Depends on: M0-07, M0-03
- Task: create the tenant-aware PostgreSQL schema.
- Done when: tenant, user, role, device, group, and policy tables exist with migrations.

### M1-04 Core CRUD

- Owner: `BE`
- Depends on: M1-03, M1-02
- Task: implement user, role, device, group, and policy CRUD.
- Done when: each object can be created, updated, listed, and retired.

### M1-05 Migration Tooling

- Owner: `OPS`
- Depends on: M1-03
- Task: add migration tooling and seed data.
- Done when: schema bootstrap is repeatable on a clean database.

### M1-06 Audit Capture

- Owner: `BE`
- Depends on: M1-04
- Task: add audit event capture for all admin mutations.
- Done when: writes produce immutable audit rows.

### M1-07 Admin E2E

- Owner: `QA`
- Depends on: M1-01, M1-04, M1-05, M1-06
- Task: verify admin login and CRUD flows end to end.
- Done when: a clean install can create and manage the core objects.

### M1-08 Plugin Isolation

- Owner: `BE`
- Depends on: M1-01, M1-04
- Task: verify the admin console can operate without any optional plugin enabled.
- Done when: core routes work with all optional plugins disabled.

## Milestone M2 - Enrollment And Sync

### M2-01 QR Enrollment

- Owner: `BE`
- Depends on: M1-03, M1-04
- Task: implement QR enrollment payload generation.
- Done when: the console can create a scannable bootstrap payload.

### M2-02 Enrollment Tokens

- Owner: `BE`
- Depends on: M2-01
- Task: implement one-time enrollment token issuance and validation.
- Done when: tokens can be issued, validated, consumed, expired, and revoked.

### M2-03 Device Secret

- Owner: `BE`
- Depends on: M2-02
- Task: implement device secret issuance after enrollment.
- Done when: each enrolled device receives a unique long-lived secret.

### M2-04 Signed Config

- Owner: `BE`
- Depends on: M2-03
- Task: implement signed device config snapshots.
- Done when: the agent can verify snapshot integrity before applying it.

### M2-05 Telemetry Upload

- Owner: `BE`
- Depends on: M2-04
- Task: implement device heartbeat and telemetry upload.
- Done when: the server records heartbeat and telemetry rows.

### M2-06 State Transitions

- Owner: `BE`
- Depends on: M2-03, M2-04
- Task: implement device state transitions and duplicate enrollment handling.
- Done when: duplicate enrollment attempts produce a deterministic server response.

### M2-07 Enrollment E2E

- Owner: `QA`
- Depends on: M2-01, M2-03, M2-04, M2-05, M2-06
- Task: verify fresh-device enrollment on a physical device.
- Done when: the physical device enrolls and syncs successfully.

### M2-08 Reconnect E2E

- Owner: `QA`
- Depends on: M2-05, M2-06
- Task: verify device reconnect after reboot without re-enrollment.
- Done when: reboot does not invalidate device identity.

## Milestone M3 - Agent Foundation

### M3-01 Kotlin Project

- Owner: `AE`
- Depends on: M0-04
- Task: create the Kotlin agent project structure.
- Done when: the agent builds with the selected toolchain.

### M3-02 Local Persistence

- Owner: `AE`
- Depends on: M3-01, M2-04
- Task: implement local persistence for bootstrap, device identity, and policy cache.
- Done when: the agent can reboot and restore its last known state.

### M3-03 Bootstrap Parsing

- Owner: `AE`
- Depends on: M2-01, M2-02
- Task: implement provisioning and bootstrap parsing.
- Done when: QR payloads and ADB/manual `base64url:` provisioning payloads are accepted and persisted.

### M3-04 Retry Logic

- Owner: `AE`
- Depends on: M3-02, M2-04
- Task: implement config fetch and retry logic.
- Done when: temporary network failures do not strand the device.

### M3-05 Recovery Diagnostics

- Owner: `AE`
- Depends on: M3-03
- Task: record bootstrap and enrollment failures for operator inspection.
- Done when: setup failures are captured in launcher diagnostics and uploaded device logs when identity is available.

### M3-06 Reboot Survival

- Owner: `QA`
- Depends on: M3-02, M3-04
- Task: verify the agent can survive reboot and resume sync.
- Done when: rebooted devices continue normal sync without manual repair.

### M3-07 Polling Fallback

- Owner: `QA`
- Depends on: M3-04
- Task: verify the agent can fall back to polling when push is unavailable.
- Done when: a broker outage still allows device commands and sync.

## Milestone M4 - Content Delivery

### M4-01 App Management

- Owner: `BE`
- Depends on: M1-04
- Task: implement app management and version upload.
- Done when: apps and versions can be created and published.

### M4-02 File Storage

- Owner: `BE`
- Depends on: M1-03
- Task: implement file upload and artifact metadata storage.
- Done when: file metadata and artifact records are persistent.

### M4-03 Certificates

- Owner: `BE`
- Depends on: M4-02
- Task: implement certificate upload and distribution.
- Done when: device config can include certificate artifacts.

### M4-04 Checksum Verification

- Owner: `BE`
- Depends on: M4-01, M4-02, M4-03
- Task: implement checksum verification on the server and agent.
- Done when: tampered or incomplete artifacts are rejected.

### M4-05 App Install Flow

- Owner: `AE`
- Depends on: M3-02, M4-01, M4-04
- Task: implement app download and installation flow.
- Done when: the agent installs and removes a managed app from a signed artifact.

### M4-06 File Download Flow

- Owner: `AE`
- Depends on: M3-02, M4-02, M4-04
- Task: implement file download and templated file generation.
- Done when: managed files are downloaded and validated on-device.

### M4-07 Content E2E

- Owner: `QA`
- Depends on: M4-05, M4-06
- Task: verify install and uninstall behavior on a managed device.
- Done when: content distribution works on at least one emulator and one device.

### M4-08 Artifact Cleanup

- Owner: `OPS`
- Depends on: M4-02, M4-04
- Task: verify artifact cleanup and orphan detection.
- Done when: stale artifacts can be identified and removed safely.

## Milestone M5 - Push And Commands

### M5-01 MQTT Transport

- Owner: `BE`
- Depends on: M2-04
- Task: implement MQTT push transport.
- Done when: the server can publish a command message to a device topic.

### M5-02 Polling Fallback

- Owner: `BE`
- Depends on: M5-01
- Task: implement HTTP polling fallback.
- Done when: the same command can be retrieved without MQTT.

### M5-03 Fan-Out Queue

- Owner: `BE`
- Depends on: M5-01, M5-02
- Task: implement server-side push fan-out and message queueing.
- Done when: commands can target one device, a group, or a broadcast set.

### M5-04 Device Acks

- Owner: `AE`
- Depends on: M5-01, M5-02
- Task: implement device command handling and acknowledgement.
- Done when: a device can confirm success or failure for a command.

### M5-05 Admin Targeting

- Owner: `BE`
- Depends on: M5-03
- Task: implement admin broadcast, group targeting, and device targeting.
- Done when: the console can create all three targeting modes.

### M5-06 Command E2E

- Owner: `QA`
- Depends on: M5-04, M5-05
- Task: verify command delivery, ack, and expiry handling.
- Done when: commands expire, ack, and retry as specified.

### M5-07 Broker Outage Recovery

- Owner: `QA`
- Depends on: M5-01, M5-02
- Task: verify push recovery after broker outage.
- Done when: polling keeps the system usable during MQTT downtime.

## Milestone M6 - Enterprise Controls

### M6-01 Kiosk Enforcement

- Owner: `AE`
- Depends on: M2-04, M3-02
- Task: implement kiosk mode and lock screen enforcement.
- Done when: policy can force the device into kiosk state.

### M6-02 Package Rules

- Owner: `AE`
- Depends on: M6-01
- Task: implement app allow/block and package suspension rules.
- Done when: disallowed apps are blocked according to policy.

### M6-03 Foreground Enforcement

- Owner: `AE`
- Depends on: M6-01
- State: Not Planned
- Task: implement foreground-app enforcement.
- Note: kiosk and package rules cover the intended control surface, so this item stays listed but is not scheduled.

### M6-04 Device Logs

- Owner: `BE`
- Depends on: M2-05
- Task: implement device log upload and search.
- Done when: the console can store and query device log records.

### M6-05 Device Info

- Owner: `BE`
- Depends on: M2-05
- Task: implement device info reporting and export.
- Done when: collected device info can be exported from the server.

### M6-06 Messaging And Audit

- Owner: `BE`
- Depends on: M1-06, M5-03
- Task: implement messaging and audit API workflows for the admin console.
- Done when: admins can view recent commands, send commands, and query audit events through the API.

### M6-07 Image Upload

- Owner: `AE`
- Depends on: M4-02
- Task: implement image upload and display.
- State: Not Planned

### M6-08 Enterprise E2E

- Owner: `QA`
- Depends on: M6-01, M6-02, M6-04, M6-05, M6-06
- Task: verify each planned enterprise feature with at least one integration test.
- Done when: each planned enterprise control has one proven device-side path.

### M6-09 Policy Gaps

- Owner: `PM/Arch`
- Depends on: M6-08
- Task: document which enterprise features remain policy-only versus fully enforced.
- Done when: no enterprise claim is left ambiguous.

## Milestone M7 - Hardening

### M7-01 Rate Limiting

- Owner: `BE`
- Depends on: M1-01, M5-03
- Task: add rate limiting and abuse protection.
- Done when: repeated abusive traffic is throttled safely.

### M7-02 Security Tests

- Owner: `QA`
- Depends on: M1-01, M2-04, M5-04
- Task: add security tests for auth, signatures, and command authorization.
- Done when: security-sensitive routes have negative and positive tests.

### M7-03 Load Tests

- Owner: `QA`
- Depends on: M2-05, M4-04, M5-03
- Task: add load tests for sync, push, and artifact downloads.
- Done when: the system can demonstrate expected scale margins.

### M7-04 Backup And Restore

- Owner: `OPS`
- Depends on: M1-03, M4-02
- Task: add backup and restore drills.
- Done when: a documented restore test succeeds.

### M7-05 Observability

- Owner: `OPS`
- Depends on: M1-01, M5-03
- Task: add observability for logs, metrics, and traces.
- Done when: core flows can be inspected in production.

### M7-06 DR And Rollback Docs

- Owner: `OPS`
- Depends on: M7-04, M7-05
- Task: add disaster recovery and rollback documentation.
- Done when: a maintainer can recover the system without source code changes.

### M7-07 Release Candidate

- Owner: `QA`
- Depends on: M7-01 through M7-06
- Task: run a release-candidate checklist on staging devices.
- Done when: staging devices can run the full product path without critical defects.

### M7-08 Cleanup Pass

- Owner: `OPS`
- Depends on: M7-03, M7-07
- Task: run a cleanup pass for stale data, orphaned artifacts, and stuck commands.
- Done when: the system is free of known backlog debris before release.

## Milestone M8 - CLI Tool

### M8-01 CLI Foundation And Configuration

- Owner: `BE`
- Depends on: M0-03, M1-01, M5-05
- Task: create the CLI project scaffold, config loading, profile handling, and base HTTP client plumbing.
- Done when: the CLI can resolve its configuration, target a server, and print a versioned help tree.

### M8-02 Authentication And Session Management

- Owner: `BE`
- Depends on: M8-01, M1-01
- Task: implement login, whoami, and logout flows for interactive CLI use.
- Done when: the CLI can establish and clear an authenticated admin session.

### M8-03 Resource Listing And Inspection

- Owner: `BE`
- Depends on: M8-01, M1-04, M4-01, M4-02, M4-03, M6-04, M6-05, M6-06
- Task: implement read-only list and show commands for users, roles, groups, policies, apps, files, managed files, certificates, devices, logs, device info, commands, and audit events.
- Done when: the CLI can inspect the core admin and device objects without mutating state.

### M8-04 Core Resource Management

- Owner: `BE`
- Depends on: M8-02, M8-03
- Task: implement create, update, and retire flows for the core admin resources.
- Done when: the CLI can manage users, roles, groups, policies, and devices end to end.

### M8-05 Content Management

- Owner: `BE`
- Depends on: M8-03, M4-01, M4-02, M4-03
- Task: implement file upload, managed-file creation, app version publish, and certificate upload and retirement commands.
- Done when: the CLI can manage artifacts and content records that feed device sync.

### M8-06 Enrollment And Bootstrap

- Owner: `BE`
- Depends on: M8-02, M2-01, M2-02, M2-03, M2-04
- Task: implement enrollment token management and bootstrap payload generation.
- Done when: the CLI can prepare and issue device enrollment material safely.

### M8-07 Device, Log, And Audit Inspection

- Owner: `BE`
- Depends on: M8-03, M6-04, M6-05, M6-06
- Task: implement device status, sync, log, device-info, and audit inspection flows.
- Done when: the CLI can answer support questions from the operational record.

### M8-08 Command Operations

- Owner: `BE`
- Depends on: M8-03, M5-03, M5-04, M5-05, M5-06, M5-07
- Task: implement command listing, command creation, command detail, and acknowledgement inspection.
- Done when: the CLI can send and track device commands over the supported delivery paths.

### M8-09 Output Formats And Error Handling

- Owner: `BE`
- Depends on: M8-01, M8-02, M8-03
- Task: implement JSON output, table output, stable exit codes, and structured error mapping.
- Done when: the CLI can be used reliably by humans and automation.

### M8-10 Packaging And Release Documentation

- Owner: `OPS`
- Depends on: M8-01 through M8-09
- Task: document installation, upgrade, completion, and shell integration for the CLI tool.
- Done when: the CLI can be distributed and operated with documented procedures.

## Milestone M9 - Admin Dashboard

### M9-01 Dashboard Blueprint And Contract

- Owner: `PM/Arch`
- Depends on: M8-10
- Task: define the browser admin dashboard scope, routes, permissions, and operator workflows.
- Done when: the roadmap, README snapshot, and dashboard docs describe every dashboard page and mutation flow.

### M9-02 Console Foundation

- Owner: `BE`
- Depends on: M9-01, M1-01, M1-02
- Task: implement the server-rendered dashboard shell, login/logout flow, CSRF protection, shared layout, navigation, and permission checks.
- Done when: `/admin` routes render authenticated HTML pages and reject unauthorized or CSRF-invalid mutations.

### M9-03 Overview Dashboard

- Owner: `BE`
- Depends on: M9-02
- Task: implement the dashboard home page with fleet, content, command, and audit summaries.
- Done when: `/admin` shows live summary counts from the control-plane repositories.

### M9-04 Core Resource Views

- Owner: `BE`
- Depends on: M9-02, M1-04
- Task: implement browser list/detail views for users, roles, groups, policies, and devices.
- Done when: operators can inspect core resources and open a device support view from the browser.

### M9-05 Core Resource Mutations

- Owner: `BE`
- Depends on: M9-04
- Task: implement browser create, update, and retire forms for users, roles, groups, policies, and devices.
- Done when: core resource browser mutations enforce CSRF, RBAC, validation, and audit capture.

### M9-06 Content Dashboard

- Owner: `BE`
- Depends on: M9-02, M4-01, M4-02, M4-03
- Task: implement browser pages for apps, app versions, files, managed files, and certificates.
- Done when: operators can manage app metadata, publish app versions, upload file/certificate artifacts, create managed files, and retire content records.

### M9-07 Enrollment Dashboard

- Owner: `BE`
- Depends on: M9-02, M2-01, M2-02
- Task: implement browser workflows for enrollment token issuance, validation, revocation, and enrollment material inspection.
- Done when: operators can prepare enrollment material without the CLI.

### M9-08 Commands Dashboard

- Owner: `BE`
- Depends on: M9-02, M5-05, M6-06
- Task: implement browser command listing and send forms for device, group, and broadcast targets.
- Done when: operators can create and inspect command rows through `/admin/commands`.

### M9-09 Inspection Dashboard

- Owner: `BE`
- Depends on: M9-02, M6-04, M6-05, M6-06
- Task: implement browser views for logs, device info, audit events, and device support inspection.
- Done when: operators can answer support questions from browser-visible operational records.

### M9-10 Dashboard E2E And Docs

- Owner: `QA`
- Depends on: M9-03 through M9-09
- Task: verify the dashboard workflow and document browser operation.
- Done when: dashboard handler coverage, admin e2e coverage, and operator documentation prove login, resource management, content, enrollment, commands, inspection, and logout.

## Milestone M10 - Premium Add-on Extension Points

### M10-01 Premium Boundary And Blueprint

- Owner: `PM/Arch`
- Depends on: M9-10
- Task: document the open-core versus premium boundary for premium features such as remote control.
- Done when: the blueprint states that premium implementation lives outside the open-core repo and core only exposes generic extension points.

### M10-02 Plugin Registry Contract

- Owner: `BE`
- Depends on: M10-01, M1-08
- Task: replace the demo plugin stub with static plugin registration for metadata, routes, device actions, command types, and enablement state.
- Done when: core starts and operates with plugins disabled, and tests prove a registered plugin can expose metadata without breaking core routes.

### M10-03 Admin Device Action Hooks

- Owner: `BE`
- Depends on: M10-02, M9-04
- Task: let registered plugins contribute device-detail actions.
- Done when: the dashboard can render plugin-provided device actions only when enabled and permitted, without hardcoding premium feature behavior.

### M10-04 Plugin Command Type Registry

- Owner: `BE`
- Depends on: M10-02, M5-05, M9-08
- Task: let plugins register allowed command types and payload validation metadata.
- Done when: admin API, dashboard, and CLI reject unregistered command types and accept registered plugin command types through the existing command queue.

### M10-05 Launcher Companion-App Command Boundary

- Owner: `AE`
- Depends on: M10-04, M4-05, M5-04
- Task: add a signed-command path for starting an installed companion package or activity.
- Done when: the launcher can start a declared companion app from a valid command and fails safely when the app, signature, or package declaration is missing.

## Backlog Rules

- An item may only move forward if every dependency is complete.
- An item may be split if the split preserves the dependency order.
- An item must be reassigned if the owner role changes.
- A phase is considered done only when all items in that milestone are done.
- A milestone should produce at least one demoable end-to-end behavior.
