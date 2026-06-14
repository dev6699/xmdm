# XMDM Project Improvement Guide

This guide defines the path from the current XMDM state to a 10/10 project.

It is intentionally repo-specific. XMDM already has a strong blueprint, working control plane, Android launcher, dashboard, MQTT plus HTTP polling, audit/RBAC, release assets, and operational docs. The next gains come from making reliability, production evidence, security hardening, and operator experience unambiguous.

## Target Score

A 10/10 XMDM project means:

- The roadmap is complete and backed by current evidence.
- The command system is safe under duplicate delivery, reconnects, broker outages, retries, and partial failures.
- A new operator can clone, run, enroll, command, debug, back up, restore, and release the system using docs alone.
- Security decisions are documented, implemented, tested, and auditable.
- Release artifacts are reproducible and connected to a tested release-candidate process.
- Production operation is observable enough to answer "what happened to this device or command?" quickly.
- The open-core and premium boundary remains clean.

## Current Baseline

Strong areas:

- Product scope is written in `blueprint/00-product-principles.md`.
- Architecture and deployment assumptions are written in `blueprint/01-system-architecture.md` and `blueprint/07-operations.md`.
- API, auth, signed config, enrollment, polling, and command contracts are written in `blueprint/02-api-contracts.md`.
- The Android lifecycle is documented in `docs/agent-app-lifecycle.md`.
- The dashboard is documented in `docs/admin-dashboard.md`.
- Release, observability, rollback, backup/restore, cleanup, and RC docs exist under `docs/`.
- MQTT plus HTTP polling fallback exists and has e2e coverage listed under `server/e2e/`.

Main gaps blocking a 10/10 rating:

- MQTT command publishing currently uses QoS 0, which is weak for command delivery.
- Command deduplication and idempotent execution are not yet explicit enough.
- Reconnect/replay behavior should be specified and tested as a contract, not only as expected behavior.
- CI appears release-focused; routine validation jobs should be restored or expanded.
- Production readiness needs stronger evidence snapshots: RC result, device matrix, load result, restore result, and security result.
- Docs should include a single "run the full system" operator path that ties together blueprint, local dev, dashboard, e2e, release, and rollback.

## Scorecard

Use this scorecard to judge progress.

| Area | Current Direction | 10/10 Gate |
| --- | --- | --- |
| Product and scope | Strong | Scope boundaries are stable, documented, and reflected in README, blueprint, and status. |
| Architecture | Strong | Runtime diagrams, failure domains, and extension boundaries match code. |
| Command reliability | Good, not finished | MQTT, polling, ack, expiry, dedup, replay, and idempotency are specified and tested. |
| Security | Good baseline | Threat model, device trust, token lifecycle, revocation, rotation, audit, and negative tests are complete. |
| Operations | Good baseline | Local, staging, backup, restore, rollback, observability, and incident workflows are executable from docs. |
| CI and release | Partial | Every PR validates docs, server tests, Android tests, Playwright, build, and release packaging where appropriate. |
| Developer onboarding | Decent | A new contributor can run a full local path in 30 minutes with documented prerequisites. |
| Enterprise evidence | Partial | Device matrix, load results, and release-candidate evidence are current and linked. |
| Open-core boundary | Strong | Core remains generic and premium implementation stays outside this repo. |

## Stage 1 - Command Reliability Hardening

Goal: make command delivery safe when MQTT and polling overlap.

Why this comes first:

- MDM reliability is judged by whether commands execute exactly as intended.
- MQTT plus polling is the correct architecture, but it creates duplicate-delivery risk.
- A command that installs, uninstalls, exits kiosk, launches a companion app, or syncs config must be retry-safe.

Tasks:

- Change server command publish from MQTT QoS 0 to QoS 1.
- Wait for PUBACK before treating MQTT publish as accepted by the broker.
- Keep the server database as the command source of truth.
- Define command transition rules in `blueprint/02-api-contracts.md` and `blueprint/05-server-services.md`.
- Define whether the state model is:
  - `queued -> sent -> acked`
  - `queued -> sent -> failed`
  - `queued -> expired`
  - optional `delivered` only if the device explicitly reports receipt
- Add an explicit dedup rule: the same `command_id` must not execute twice on one device.
- Add a device-side executed-command cache with retention, or define a server-side idempotency window that fully prevents duplicate execution.
- Add command result rules for repeated acks, late acks, expired commands, and unknown command IDs.
- Add tests proving duplicate MQTT and polling delivery cannot execute the same command twice.

Verification:

- `go test ./internal/push ./internal/commands/...`
- Device-backed command tests for MQTT, polling, broker outage, and duplicate delivery.
- A doc section that explains how an operator diagnoses a stuck command.

Done when:

- A command sent through MQTT and rediscovered through polling executes once.
- A device reboot or reconnect does not cause duplicate execution.
- A broker outage falls back to polling and then returns to MQTT without re-enrollment.
- The dashboard can show enough command state to explain what happened.

## Stage 2 - Reconnect, Replay, And Offline Semantics

Goal: make device communication behavior deterministic across offline periods.

Tasks:

- Write a reconnect contract:
  - what the device sends after reconnect
  - what the server considers pending
  - what happens to sent-but-unacked commands
  - how expired commands are hidden or reported
- Decide whether replay is time-based, command-ID-based, or status-based.
- Make polling the recovery path for commands that were not acknowledged, not an independent command brain.
- Record command transport source in result metadata when useful for debugging.
- Add device log events for MQTT connect, subscribe, disconnect, fallback polling, command received, command executed, and ack sent.
- Add dashboard language for stale active devices and command timeout reasons.

Verification:

- Device goes offline, command is queued, device returns, command executes once.
- Device receives MQTT command but ack fails, then polling recovery does not double-execute it.
- Device receives polling command during broker outage, then broker recovers without repeating it.

Done when:

- The reconnect path is documented, implemented, and proven with tests or a manual RC checklist entry.

## Stage 3 - Security Hardening

Goal: make the device and admin trust model explicit, tested, and operational.

Tasks:

- Expand `blueprint/06-security-and-compliance.md` with:
  - device trust model
  - enrollment token lifecycle
  - per-device secret lifecycle
  - revocation behavior
  - credential rotation plan
  - signed config threat model
  - MQTT credential provisioning and retirement
  - artifact integrity guarantees
- Add a device secret rotation design, even if implementation is staged.
- Add an enrollment recovery design for lost, stolen, reset, or duplicate devices.
- Ensure every admin mutation has:
  - auth
  - RBAC
  - CSRF for browser forms
  - audit capture
  - validation tests
- Add negative tests for:
  - wrong device secret
  - revoked enrollment token
  - consumed enrollment token
  - tampered signed config
  - command ack from the wrong device
  - unauthorized admin command creation
  - plugin command type rejection

Verification:

- `go test ./internal/auth ./internal/enrollment/... ./internal/commands/... ./internal/api/v1`
- Security test cases are listed in the release-candidate checklist.

Done when:

- A maintainer can explain and test how rogue devices, stale tokens, tampered configs, and unauthorized commands are rejected.

## Stage 4 - CI, Release, And Evidence

Goal: make correctness repeatable instead of dependent on local memory.

Tasks:

- Add or restore routine CI workflows for:
  - docs link/lint checks
  - Go unit tests
  - Android unit tests
  - server build
  - Android debug build
  - Playwright dashboard tests where environment permits
- Keep release packaging in the release workflow.
- Keep the lightweight CI status section in `PROJECT_STATUS.md`.
- Record release-candidate evidence outside the repo.
- Record release-candidate runs with:
  - date
  - commit
  - server artifact
  - APK artifact
  - database migration version
  - device model and Android version
  - tests run
  - failures and waivers
- Keep `docs/release-candidate-checklist.md` as the gate, not a passive document.

Verification:

- A clean branch shows expected CI checks.
- Release workflow still produces server and APK artifacts.
- Release-candidate evidence can be traced to a commit.

Done when:

- The repo can prove that the claimed release was built and tested from the same source.

## Stage 5 - Production Operations

Goal: make the system operable by someone who did not write it.

Tasks:

- Create a single full-system runbook:
  - start local stack
  - migrate database
  - start server
  - publish launcher APK
  - enroll device
  - apply policy
  - send command
  - inspect logs and device info
  - back up and restore
  - roll back
- Expand observability docs with concrete examples:
  - request ID lookup
  - traceparent lookup
  - command latency
  - command failure rate
  - stale device detection
  - MQTT outage diagnosis
  - object storage artifact failure diagnosis
- Add dashboard troubleshooting links or text for common support cases.
- Add an incident checklist for:
  - database down
  - object storage down
  - MQTT broker down
  - bad APK published
  - migration failed
  - device fleet stopped syncing

Verification:

- A fresh operator can execute the documented local path.
- Backup/restore drill output is captured for a current schema.
- MQTT outage and recovery are documented with expected operator observations.

Done when:

- Docs answer how to run, debug, recover, and release without source-code spelunking.

## Stage 6 - Device Matrix And Android Reality

Goal: prove the launcher survives real Android constraints.

Tasks:

- Maintain a device matrix:
  - emulator API levels
  - physical device models
  - Android versions
  - OEM skins
  - device-owner provisioning method
  - tested capabilities
- Track known Android limitations:
  - silent install constraints
  - background execution
  - kiosk behavior differences
  - certificate install behavior
  - package suspension behavior
  - reboot survival
- Add manual verification scripts or checklists where automation is not realistic.
- Make app install, file download, certificate, kiosk, package rules, logs, and command checks visible in release-candidate evidence.

Verification:

- At least one emulator and one physical device prove the full product path.
- At least one broker outage and one reboot recovery are tested on a physical device.

Done when:

- The project can say which Android environments are supported and which are not.

## Stage 7 - Dashboard And Operator Experience

Goal: turn operational state into fast answers.

Tasks:

- Make command detail pages answer:
  - who sent it
  - target device or group
  - current status
  - MQTT publish result
  - polling fallback result
  - ack time
  - failure reason
  - retry or expiry status
- Make device detail pages answer:
  - last heartbeat
  - last config version
  - last command
  - last MQTT status if known
  - recent logs
  - recent device info
  - enrollment source
- Add filters for failed, pending, expired, and slow commands.
- Add user-facing explanations for Not Planned enterprise controls so the product does not overclaim.

Verification:

- Playwright tests cover the main dashboard workflows.
- Manual dashboard screenshots stay current when UI changes.

Done when:

- An operator can answer "why did this device not execute command X?" from the dashboard and logs.

## Stage 8 - Data Model And Policy Versioning

Goal: make policy evolution safe and auditable.

Tasks:

- Decide whether policies are mutable records, versioned records, or versioned snapshots.
- Document rollback behavior:
  - what gets rolled back
  - how devices see the change
  - how old artifacts are retained
  - what happens to commands created under old policy state
- Add policy diff visibility for operators.
- Add audit entries for policy assignment, policy content changes, app publish, file publish, certificate assignment, and command enqueue.
- Add data retention decisions for logs, device info, telemetry, commands, and artifacts.

Verification:

- An operator can change policy, see the diff, apply it, and roll back.
- Audit events connect each mutation to the responsible admin.

Done when:

- Policy changes are explainable after the fact and reversible within documented limits.

## Stage 9 - Scale And Load Evidence

Goal: prove the selected architecture has margins for its intended deployment size.

Tasks:

- Define target scale for v1:
  - devices
  - commands per minute
  - telemetry writes per minute
  - app artifact download concurrency
  - dashboard operator count
- Add synthetic load tests for:
  - config sync
  - command enqueue
  - command polling
  - command ack
  - telemetry upload
  - artifact download metadata path
- Measure:
  - p50/p95/p99 latency
  - error rate
  - database connection use
  - broker publish failures
  - CPU and memory
- Keep scale results in docs with commit and environment details.

Verification:

- Load test results are reproducible enough for release decisions.
- Observability exposes the same failure modes the load tests exercise.

Done when:

- The project has a stated supported scale and evidence for that scale.

## Stage 10 - Documentation System Polish

Goal: make docs feel like a finished product surface.

Tasks:

- Add a "Start Here" path that clearly separates:
  - product overview
  - local development
  - operator manual
  - architecture blueprint
  - release process
  - troubleshooting
- Add a full-system architecture diagram:
  - dashboard
  - API
  - PostgreSQL
  - object storage
  - MQTT broker
  - Android launcher
  - premium plugin boundary
- Add a command lifecycle diagram.
- Add a device lifecycle diagram.
- Add a security lifecycle diagram for enrollment to runtime auth.
- Keep `/blueprint` for decisions and `/docs` for operator/developer procedures.
- Remove or rewrite any generic/stale rating or evaluation notes before publishing.

Verification:

- New readers can understand the project in this order:
  - `README.md`
  - `blueprint/README.md`
  - `PROJECT_STATUS.md`
  - `docs/admin-dashboard.md`
  - full-system runbook

Done when:

- The docs no longer require tribal knowledge from previous implementation sessions.

## Do Not Do Yet

Avoid these until the current architecture hits its limits:

- Do not rename `server/` to `backend/` or `app/` to `agent/` only for aesthetics.
- Do not add Kafka, NATS, or Redis Streams before command semantics are correct.
- Do not add multi-tenant SaaS workflows before single-tenant self-hosted operation is proven.
- Do not move premium implementation into the open-core repo.
- Do not mark release readiness based only on unit tests.
- Do not treat MQTT as a general data plane; keep it focused on push delivery.

## Recommended Immediate Backlog

1. Implement MQTT QoS 1 command publish and PUBACK handling.
2. Write command dedup and idempotency rules into the blueprint.
3. Add device-side executed-command cache or an equivalent server-side dedup guarantee.
4. Add duplicate-delivery tests for MQTT plus polling.
5. Restore or add routine CI checks outside the release workflow.
6. Create a full-system runbook that covers local run, enroll, command, debug, backup, restore, and release.
7. Add release-candidate evidence files for the next verified device-backed run.

## Save Point Criteria

Suggest a save point after each of these:

- Command reliability contract and tests land.
- MQTT QoS 1 publish is implemented and verified.
- CI validation workflow is restored.
- Full-system runbook is added and walked through.
- Security hardening tests are added.
- Release-candidate evidence is captured.

Use the repo save prompt pattern before committing a meaningful checkpoint.
