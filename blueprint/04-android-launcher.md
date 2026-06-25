# Android Launcher

## Launcher Design

The Android launcher is the device-side runtime. It enrolls the device, persists
local state, fetches signed config snapshots, applies policy/content assignments,
handles kiosk behavior, executes supported commands, and uploads telemetry,
device info, and logs.

The server owns policy truth. The launcher owns local application of the latest
accepted snapshot.

## Implementation Decisions

- Kotlin Android app.
- XML layouts with ViewBinding.
- AndroidX AppCompat, Core, Lifecycle, and DataStore.
- Kotlin coroutines for asynchronous work.
- Gson for JSON handling.
- Android framework APIs for device admin, package install, certificate install,
  reboot, and kiosk-related behavior.
- Launcher-owned HTTP and MQTT transport code.

## Provisioning

- QR and manual/ADB provisioning use the Android managed-provisioning payload.
- The payload carries the server base URL, enrollment token, and optional device
  identifier.
- The launcher persists bootstrap state locally.
- Enrollment returns device credentials.
- The launcher fetches the signed config snapshot after enrollment.

The detailed launcher lifecycle lives in
[../docs/launcher-lifecycle.md](../docs/launcher-lifecycle.md).

## Runtime State

The launcher persists:

- server/bootstrap details
- device identity and secret
- latest accepted config state
- command execution results used for duplicate-delivery handling
- device logs pending upload
- sync and enrollment diagnostics

## Policy And Content Application

- Verify the signed config snapshot before applying it.
- Download managed artifacts through server-authorized routes.
- Verify checksums before applying downloaded content.
- Apply managed files, certificates, managed apps, package rules, and kiosk
  settings from the latest accepted snapshot.
- Preserve the last valid local state when config fetch fails.

## Kiosk And Commands

- Kiosk behavior is launcher-owned and policy-driven.
- Kiosk exit can be triggered by local passcode support or the supported
  `exit_kiosk` command.
- Built-in commands are `ping`, `reboot`, `sync_config`, `exit_kiosk`, and
  `launch_companion_app`.
- The launcher treats command IDs as idempotency keys across MQTT and HTTP
  polling.
- Duplicate command delivery returns the cached terminal result instead of
  re-running the command.

## Observability

- The launcher records lifecycle logs around startup, bootstrap, enrollment,
  config sync, policy changes, command handling, kiosk handling, managed
  content, device info, and log-drop summaries.
- Logs are uploaded in batches with client-generated IDs so retries remain
  idempotent.
- Device-info uploads provide inventory/runtime snapshots for operator support.
