# XMDM Technical Blueprint

XMDM is the project name for a new self-hosted MDM platform rebuilt in Kotlin and Go.

This folder is the source of truth for product scope, architecture, contracts, data model, operations, security, and future delivery planning for the XMDM codebase.

The design rule is simple:

- write the decision
- explain why it was chosen
- state what was rejected
- describe the resulting behavior

If a choice is not written here, it is not decided yet.

## Reading Order

1. [00-product-principles.md](00-product-principles.md)
2. [01-system-architecture.md](01-system-architecture.md)
3. [02-api-contracts.md](02-api-contracts.md)
4. [03-data-model.md](03-data-model.md)
5. [04-device-agent.md](04-device-agent.md)
6. [05-server-services.md](05-server-services.md)
7. [06-security-and-compliance.md](06-security-and-compliance.md)
8. [07-operations.md](07-operations.md)
9. [08-migration-plan.md](08-migration-plan.md)
10. [09-roadmap-checklist.md](09-roadmap-checklist.md)

## Locked Decisions

- Self-hosted, single-tenant first
- Full enterprise parity as the target feature set
- Kotlin Android agent
- Go server, workers, and admin console
- PostgreSQL as the system of record
- Object storage for all binary artifacts
- MQTT plus HTTP polling for push delivery
- Plugin-capable backend from day one
- Server-rendered admin console first, SPA later only if there is a hard need

## What These Docs Define

- the product and its scope boundaries
- the runtime architecture and service boundaries
- the API contract surface
- the data model and lifecycle states
- the Android enrollment and runtime flow
- the server-side control plane flow
- the security model and trust boundaries
- the deployment, backup, and operational model
- the phased implementation roadmap and completion gates
- the numbered implementation backlog with owners and dependencies

## Current Repo Relationship

The existing `hmdm-android` and `hmdm-server` repositories are behavior and code references only.

The XMDM docs do not inherit their architecture. They only borrow observed product behavior, feature grouping, and operational expectations.

## Delivery Standard

Every major capability must eventually have:

- a written contract
- a persisted state model
- a failure mode
- an operational path
- a test plan
- a checklist item in the roadmap
- an owner and dependency chain in the backlog
