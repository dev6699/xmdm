# XMDM Technical Blueprint

XMDM is a self-hosted Android MDM control plane implemented in Kotlin and Go.
The blueprint records durable product and architecture decisions. Product docs,
operator procedures, roadmaps, and status tracking live outside the blueprint.

Product and operator documentation starts at [../docs/README.md](../docs/README.md).
If a blueprint decision conflicts with source code, tests, config, migrations, or
current runbooks, verify the implementation and update or remove the stale
blueprint text.

The design rule is simple:

- write the decision
- explain why it was chosen
- state what was rejected
- describe the resulting behavior

Present a choice as supported only when it is written here and implemented in
the current repo.

## Reading Order

1. [00-product-principles.md](00-product-principles.md)
2. [01-system-architecture.md](01-system-architecture.md)
3. [02-api-contracts.md](02-api-contracts.md)
4. [03-data-model.md](03-data-model.md)
5. [04-android-launcher.md](04-android-launcher.md)
6. [05-server-services.md](05-server-services.md)
7. [06-security-and-compliance.md](06-security-and-compliance.md)
8. [07-operations.md](07-operations.md)

## Locked Decisions

- Self-hosted, single-tenant first
- Kotlin Android launcher
- Go server and admin dashboard
- PostgreSQL as the system of record
- Object storage for all binary artifacts
- MQTT plus HTTP polling for push delivery
- Plugin-capable backend from day one
- Server-rendered admin dashboard first, SPA later only if there is a hard need

## What These Docs Define

- the product and its scope boundaries
- the runtime architecture and service boundaries
- the API contract surface
- the data model and lifecycle states
- the Android enrollment and runtime flow
- the server-side control plane flow
- the security model and trust boundaries
- the deployment, backup, and operational model

## Documentation Standard

Every documented capability must have:

- a written contract
- a persisted state model
- a failure mode
- an operational path
- a test plan
- implementation evidence or an explicit limitation
