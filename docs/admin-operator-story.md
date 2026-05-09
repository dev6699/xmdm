# Admin Operator Story

This document turns the `server/e2e` coverage into a practical operator guideline for future CLI and UI work.

The intent is not to define a new API contract. The intent is to describe the real admin workflow that the control plane must support end to end, and to call out the management tasks that are important but not yet covered by E2E.

## Operator Goal

An XMDM admin should be able to:

1. Log in and confirm their session.
2. Establish access control through users and roles.
3. Prepare policies, groups, apps, files, and certificates.
4. Enroll devices and verify they begin syncing.
5. Push configuration and content changes.
6. Issue commands and observe acknowledgements.
7. Inspect logs, device info, and audit history.
8. Retire or update anything that is no longer valid.

That is the core control-loop for managing the application.

## Lifecycle Story

### 1. Enter the control plane

The admin starts at the login screen and authenticates with a session cookie.

Expected actions:

- open the login page
- submit credentials
- verify the current user with `/api/v1/admin/me`
- logout when finished

Covered by:

- [TestAdminE2E](../server/e2e/admin_test.go)

Relevant contract:

- [Admin console session routes](../contracts/admin-console.md)

### 2. Establish access control

Before managing devices at scale, the admin defines who can do what.

Expected actions:

- create users
- create roles
- assign permissions to roles
- update user role assignments
- retire users or roles that should no longer be active

Covered by:

- [TestAdminE2E](../server/e2e/admin_test.go)

Why this matters:

- future CLI and UI flows need a stable permission model before they expose any dangerous operations
- operators should see permission failures as normal workflow outcomes, not surprising errors

### 3. Prepare fleet structure

The admin groups devices and prepares policy objects before enrollment or content rollout.

Expected actions:

- create groups for fleet segmentation
- create policies for baseline device behavior
- version policy changes explicitly
- retire policies that are obsolete

Covered by:

- [TestAdminE2E](../server/e2e/admin_test.go)
- [TestPolicySync](../server/e2e/content_test.go)

Key operator rule:

- a policy update should be treated as a new controlled release of device behavior, not an incidental edit

### 4. Prepare content and certificates

The admin uploads reusable artifacts and turns them into device-deliverable resources.

Expected actions:

- upload raw files
- create managed-file records that place content on device paths
- upload APK artifacts
- create and publish app versions
- upload certificates
- retire content that should no longer be delivered

Covered by:

- [TestAdminE2E](../server/e2e/admin_test.go)
- [TestManagedAppsAndFiles](../server/e2e/content_test.go)
- [TestManagedAppsAndFilesRemoval](../server/e2e/content_test.go)
- [TestCertificatesApplied](../server/e2e/content_test.go)

Operator guidance:

- the UI/CLI should always show the logical record and the backing artifact together
- content changes should expose checksum, artifact identity, and active/retired state
- if artifact upload fails validation, the admin should know whether the logical record was created or not

### 5. Enroll a device

Enrollment is the moment a device becomes part of the managed fleet.

Expected actions:

- create an enrollment token
- optionally validate or preview the token lifecycle
- issue a QR/bootstrap payload for device provisioning
- enroll the device with an identity policy
- receive the device secret
- fetch the first signed config snapshot
- confirm the device transitions from pending/enrolled to active

Covered by:

- [TestEnrollmentE2E](../server/e2e/enrollment_test.go)

Operator guidance:

- enrollment should be understandable as a single journey: token, bind, secret, sync
- duplicate enrollment should be surfaced as a clear conflict, not a silent rebind
- the admin should be able to tell whether a device is waiting to enroll, enrolled, active, or retired

### 6. Verify device health

After enrollment, the admin needs proof that the device is actually participating.

Expected actions:

- confirm the device appears in inventory
- confirm it fetched the signed config snapshot
- confirm it uploaded telemetry
- confirm it uploaded device info
- confirm it uploaded logs when applicable

Covered by:

- [TestEnrollmentE2E](../server/e2e/enrollment_test.go)
- [TestDeviceInfoReporting](../server/e2e/deviceinfo_test.go)
- [TestDeviceLogsUpload](../server/e2e/content_test.go)

Operator guidance:

- health should be visible as recent activity, not just row existence
- the UI/CLI should expose last sync, last telemetry, last log upload, and current status on one device view

### 7. Push managed behavior

The admin then shapes how the device behaves in the field.

Expected actions:

- push policy updates
- install or remove managed apps
- deliver or remove managed files
- install or retire certificates
- enforce kiosk state and package rules

Covered by:

- [TestManagedAppsAndFiles](../server/e2e/content_test.go)
- [TestManagedAppsAndFilesRemoval](../server/e2e/content_test.go)
- [TestCertificatesApplied](../server/e2e/content_test.go)
- [TestKioskModeChrome](../server/e2e/content_test.go)
- [TestKioskExitChromeLocal](../server/e2e/content_test.go)
- [TestKioskExitChromeCommand](../server/e2e/content_test.go)
- [TestKioskStayAwakeWhilePluggedIn](../server/e2e/content_test.go)
- [TestPackageRules](../server/e2e/content_test.go)
- [TestPolicySync](../server/e2e/content_test.go)

Operator guidance:

- policy changes should be reviewable before rollout
- managed apps/files/certificates should show both logical state and device delivery state
- kiosk controls need a clear exit path for operators, with the passcode or command path made obvious

### 8. Issue commands

The admin can send explicit device actions.

Expected actions:

- create a command
- target a single device
- target a group
- target a broadcast scope when supported
- see delivery status
- see acknowledgement payloads
- see fallback behavior when MQTT is unavailable

Covered by:

- [TestCommandMQTT](../server/e2e/content_test.go)
- [TestCommandMQTTSyncConfig](../server/e2e/content_test.go)
- [TestCommandPolling](../server/e2e/content_test.go)
- [TestCommandBrokerOutageRecovery](../server/e2e/content_test.go)
- [TestKioskExitChromeCommand](../server/e2e/content_test.go)

Operator guidance:

- command delivery is not complete when it is queued; it is complete when it is acknowledged
- the UI/CLI should expose the transport path when useful, but not require the operator to understand MQTT vs polling to use the product
- command expiry should be visible because it is part of the operator contract

### 9. Observe and troubleshoot

The admin needs operational visibility after the initial rollout.

Expected actions:

- search logs by device, source, level, and time range
- search device info by device and time range
- inspect audit events for admin mutations
- inspect command history and command results
- correlate operational failures with the affected device or content object

Covered by:

- [TestDeviceLogsUpload](../server/e2e/content_test.go)
- [TestDeviceInfoReporting](../server/e2e/deviceinfo_test.go)
- [TestAdminE2E](../server/e2e/admin_test.go)

Operator guidance:

- the product should surface support views, not just CRUD forms
- audit history should be treated as a first-class operational object
- the UI/CLI should make it easy to answer: what changed, who changed it, when, and what devices were affected

### 10. Retire safely

At the end of a lifecycle, the admin should be able to remove resources without breaking the control plane.

Expected actions:

- retire devices
- retire apps, files, managed files, certificates, groups, policies, roles, and users
- confirm the device reflects removals on the next sync
- keep historical records where audit or history requires it

Covered by:

- [TestAdminE2E](../server/e2e/admin_test.go)
- [TestManagedAppsAndFilesRemoval](../server/e2e/content_test.go)

Operator guidance:

- retire is preferable to hard delete for managed entities
- the admin should be able to tell which resources are active, pending, or retired
- retired resources should remain visible enough for audit and support workflows

## Future Workflows

The current E2E suite does not directly cover these, but they are important for managing the application well.

### Enrollment token management

Add explicit workflows for:

- list active tokens
- validate a token before use
- consume a token during provisioning
- revoke a token early
- show expiry and remaining TTL

Why it matters:

- enrollment is one of the most security-sensitive flows in the product
- operators need to recover from leaked or stale tokens without editing the database

### Device detail view or CLI subcommand

Add a single device-focused view that shows:

- identity and enrollment state
- policy assignment and last config revision
- last telemetry
- last logs
- device-info history
- command queue and acknowledgements
- content delivery state

Why it matters:

- support work is usually device-centric, not object-centric
- a fragmented set of list endpoints is too slow for day-to-day operations

### Command lifecycle inspection

Add workflows for:

- list queued, sent, acked, expired, and failed commands
- filter commands by target scope and type
- inspect command payloads and terminal results
- retry or reissue supported commands when appropriate

Why it matters:

- commands are the primary remote-action primitive
- operators need a complete answer when a device did not react

### Policy review and diff

Add workflows for:

- preview the effective device snapshot before publishing
- compare current and proposed policy versions
- highlight kiosk and package-rule changes
- reject stale edits when version numbers conflict

Why it matters:

- policy mistakes can lock down devices or change fleet behavior unexpectedly

### Search and export

Add workflows for:

- search logs with saved filters
- export device-info records for support cases
- export audit history for compliance or incident response

Why it matters:

- these are the fastest way to move from "something is wrong" to "here is what happened"

### Operational health views

Add workflows for:

- MQTT health and broker connectivity
- command queue depth and stuck deliveries
- sync backlog and last successful sync by device
- artifact cleanup status
- stale enrollment token cleanup
- job failures and recovery actions

Why it matters:

- the control plane should be operable without direct database inspection

### Recovery and remediation

Add workflows for:

- rotate a device secret
- force a fresh config sync
- re-enroll a device after loss of trust
- retire and replace a device cleanly
- recover from partial content or policy rollout failures

Why it matters:

- real operators spend a lot of time on exceptions, not only on happy-path provisioning

## CLI Tool Checklist

The CLI should be a thin, deterministic shell over the same server contracts that power the future UI.

### Command Tree

The initial command tree should stay close to the domain objects in the server:

```text
xmdm
  auth
    login
    whoami
    logout
  users
    list
    create
    update
    retire
  roles
    list
    create
    update
    retire
  groups
    list
    create
    update
    retire
  policies
    list
    create
    update
    retire
  apps
    list
    create
    update
    retire
    versions
      list
      create
      publish
  files
    list
    upload
    retire
  managed-files
    list
    create
    retire
  certificates
    list
    upload
    retire
  enrollment
    tokens
      list
      create
      validate
      consume
      revoke
    qr
      json
      png
    enroll
  devices
    list
    show
    create
    update
    retire
    sync
    logs
    info
    commands
  commands
    list
    send
    show
    ack
  logs
    list
  device-info
    list
  audit
    list
```

### Deferred Command Tree Additions

These commands are useful, but they are not part of the first main CLI path:

- `policies preview`
- `health server`
- `health mqtt`
- `health queue`
- `health jobs`
- `version`

### Output Contract

The CLI output contract should support both operators and automation.

#### Global Rules

- `stdout` carries successful command output.
- `stderr` carries progress, warnings, and error messages.
- `--format json` produces stable machine-readable output.
- `--format table` or the default human mode produces concise operator-friendly output.
- `--quiet` suppresses nonessential progress text.
- All commands should return a nonzero exit code on failure.

#### Success Envelope

JSON mode should emit a stable envelope:

```json
{
  "ok": true,
  "command": "devices list",
  "data": {},
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T00:00:00Z",
    "baseUrl": "https://mdm.example"
  }
}
```

Recommended `data` shapes:

- `list` commands: `{ "items": [...], "count": 12, "cursor": null }`
- `show` commands: `{ "item": {...} }`
- `create`/`update`/`retire` commands: `{ "resource": {...} }`
- `send` command requests: `{ "commands": [...], "target": {...} }`
- `login`/`whoami`: `{ "user": {...} }`
- `health` commands: `{ "status": "ok", "checks": [...] }`

#### Error Envelope

JSON mode should emit the same top-level error shape as the API contract:

```json
{
  "ok": false,
  "error": {
    "code": "device_not_found",
    "message": "Device not found",
    "details": {}
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T00:00:00Z",
    "baseUrl": "https://mdm.example"
  }
}
```

#### Exit Codes

Use stable exit codes so scripts can branch without parsing text:

- `0` success
- `1` validation error or bad local input
- `2` authentication failure
- `3` authorization failure
- `4` not found
- `5` conflict
- `6` transport or server failure

#### Human Output Rules

- Tables should be short and sorted by the primary object identifier.
- Long payloads should be summarized, with `--format json` used for full fidelity.
- Mutations should confirm the resource type, identifier, and new status.
- Command requests should show the target, command type, and terminal state once available.
- Health views should make the failing check obvious first.

#### Command Mapping Principles

- Each CLI command should map to a single server contract when possible.
- Commands that combine multiple server actions should be explicit in the name, such as `enrollment qr json` or `apps versions publish`.
- Read commands should remain idempotent and side-effect free.
- Mutating commands should surface the resource identifier immediately after success.
- Where the server already exposes list endpoints, the CLI should preserve the same filters and pagination behavior.

## Deferred CLI Implementation Rules

These are the rules the first CLI implementation should follow so it remains safe for operators and reliable for automation.

### Configuration Precedence

- Resolve settings in this order: flags, environment variables, config file, defaults.
- Keep the config file format simple and human-editable.
- Require an explicit base URL or profile before any networked command runs.

### Secret Handling

- Never print passwords, enrollment tokens, device secrets, or session cookies in normal human output.
- Redact secrets in logs, error traces, and debug output unless the user explicitly requests raw values.
- Treat QR/bootstrap payloads as sensitive because they can contain enrollment material.

### Destructive Action Safety

- Require confirmation before retire, revoke, delete, or replace operations when running interactively.
- Allow `--yes` or `--force` to bypass confirmation for automation.
- Show the exact object being affected before confirming the action.

### Automation Mode

- Support `--format json` for machine consumption.
- Support `--quiet` or `--no-prompt` so scripts never block on interaction.
- Keep JSON output stable across releases once the command is public.

### Errors And Correlation

- Preserve server error `code` and `details` in CLI failures.
- Map server failures to stable exit codes.
- Surface `requestId` on failure so operators can correlate CLI output with server logs and audit history.

### Retry And Idempotency

- Retry only read-only or explicitly idempotent commands automatically.
- Never auto-retry a mutation unless the server contract guarantees idempotency.
- Make retry behavior visible so operators know when a command was reissued.

### Preview And Dry Run

- Provide preview or dry-run mode for policy changes, command sends, and bootstrap generation where it reduces operator risk.
- Show what would change before mutating state whenever the underlying workflow is likely to have fleet-wide impact.

### Help And Completion

- Keep `--help` output consistent across the tree.
- Make command names and subcommands discoverable from top-level help.
- Add shell completion support early if the command tree grows beyond a handful of subcommands.

## Deferred CLI Additional Rules

These rules close the remaining gaps for implementation and long-term maintenance.

### Test Matrix

- Cover command parsing, request building, and output formatting with unit tests.
- Cover API interaction with integration tests against the server contract.
- Cover the stable JSON schema with golden-file or snapshot tests.
- Cover destructive commands with explicit safety tests.

### Auth Modes

- Support session-cookie auth for interactive use.
- Support token-based auth for automation when the server exposes it.
- Make the active auth mode obvious in `whoami` or profile output.

### Profile Management

- Support named profiles even in the single-tenant first release.
- Let profiles hold base URL, auth mode, and default output format.
- Make switching profiles explicit and non-destructive.

### Non-Interactive Guarantees

- Commands that may prompt must have a `--yes` or `--force` override.
- Commands used by scripts must have a documented non-interactive path.
- Fail fast instead of waiting for hidden user input when `stdin` is not a TTY.

### Timeouts And Pagination

- Use explicit timeout defaults for network calls and long-running operations.
- Support limit and cursor-style pagination on every list command that the server exposes.
- Keep the default page size small enough for terminals but large enough to be useful.

### Resource Selectors

- Use consistent selectors for `deviceId`, `groupId`, `policyId`, `appId`, `fileId`, and `certificateId`.
- Allow either direct identifiers or lookup-by-name only where the server contract already supports it.
- Print the resolved identifier when a user selects by name.

### Naming Conventions

- Prefer verb-first commands: `list`, `show`, `create`, `update`, `retire`, `send`, `preview`.
- Keep noun phrases aligned with the server domain objects.
- Avoid aliases that hide the underlying API shape.

### Audit Visibility

- Print the resulting audit or request correlation ID after every mutating command when available.
- Make it easy to copy that identifier into support tickets or logs.
- Treat audit visibility as part of the operator contract, not just an internal detail.

## Implementation Guidance For CLI/UI

- Keep the admin model object-centric: user, role, group, policy, app, file, certificate, device, command, audit event.
- Make the happy path short, but keep the supporting detail visible for troubleshooting.
- Show status and lifecycle explicitly; do not hide `pending`, `enrolled`, `active`, or `retired`.
- Prefer actions that map cleanly to existing server contracts.
- Treat unsupported or missing workflows as first-class TODOs rather than hidden gaps.
