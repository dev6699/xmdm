# XMDM Project Status

This repository is the working home for XMDM.

## Roadmap Snapshot

Roadmap source: [blueprint/09-roadmap-checklist.md](blueprint/09-roadmap-checklist.md)

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
| M1-01 Admin Auth | ☐ |
| M1-02 RBAC | ☐ |
| M1-03 Core Schema | ☐ |
| M1-04 Core CRUD | ☐ |
| M1-05 Migration Tooling | ☐ |
| M1-06 Audit Capture | ☐ |
| M1-07 Admin E2E | ☐ |
| M1-08 Plugin Isolation | ☐ |

### M2 - Enrollment And Sync

| Item | State |
| --- | --- |
| M2-01 QR Enrollment | ☐ |
| M2-02 Enrollment Tokens | ☐ |
| M2-03 Device Secret | ☐ |
| M2-04 Signed Config | ☐ |
| M2-05 Telemetry Upload | ☐ |
| M2-06 State Transitions | ☐ |
| M2-07 Enrollment E2E | ☐ |
| M2-08 Reconnect E2E | ☐ |

### M3 - Agent Foundation

| Item | State |
| --- | --- |
| M3-01 Kotlin Project | ☐ |
| M3-02 Local Persistence | ☐ |
| M3-03 Bootstrap Parsing | ☐ |
| M3-04 Retry Logic | ☐ |
| M3-05 Recovery UI | ☐ |
| M3-06 Reboot Survival | ☐ |
| M3-07 Polling Fallback | ☐ |

### M4 - Content Delivery

| Item | State |
| --- | --- |
| M4-01 App Management | ☐ |
| M4-02 File Storage | ☐ |
| M4-03 Certificates | ☐ |
| M4-04 Checksum Verification | ☐ |
| M4-05 App Install Flow | ☐ |
| M4-06 File Download Flow | ☐ |
| M4-07 Content E2E | ☐ |
| M4-08 Artifact Cleanup | ☐ |

### M5 - Push And Commands

| Item | State |
| --- | --- |
| M5-01 MQTT Transport | ☐ |
| M5-02 Polling Fallback | ☐ |
| M5-03 Fan-Out Queue | ☐ |
| M5-04 Device Acks | ☐ |
| M5-05 Admin Targeting | ☐ |
| M5-06 Command E2E | ☐ |
| M5-07 Broker Outage Recovery | ☐ |

### M6 - Enterprise Controls

| Item | State |
| --- | --- |
| M6-01 Kiosk Enforcement | ☐ |
| M6-02 Package Rules | ☐ |
| M6-03 Foreground Enforcement | ☐ |
| M6-04 Device Logs | ☐ |
| M6-05 Device Info | ☐ |
| M6-06 Messaging And Audit | ☐ |
| M6-07 Image Upload | ☐ |
| M6-08 Enterprise E2E | ☐ |
| M6-09 Policy Gaps | ☐ |

### M7 - Hardening

| Item | State |
| --- | --- |
| M7-01 Rate Limiting | ☐ |
| M7-02 Security Tests | ☐ |
| M7-03 Load Tests | ☐ |
| M7-04 Backup And Restore | ☐ |
| M7-05 Observability | ☐ |
| M7-06 DR And Rollback Docs | ☐ |
| M7-07 Release Candidate | ☐ |
| M7-08 Cleanup Pass | ☐ |

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
