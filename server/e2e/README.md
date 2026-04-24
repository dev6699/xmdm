# Server E2E

This directory holds the root-level end-to-end tests for the Go server. The tests run against the HTTP handler stack with a real Postgres database, but they do not start a socket listener.

## What Is Covered

- `TestAdminE2E` covers the admin console flow.
- `TestEnrollmentE2E` covers the enrollment and first-sync flow.

## Admin Flow

The admin E2E verifies:

- admin login and session handling
- CRUD for users, roles, apps, groups, policies, and devices
- app version upload and publish
- audit event capture for admin mutations
- logout and session invalidation

## Enrollment Flow

The enrollment E2E verifies:

- enrollment token issuance through the admin API
- device binding through `/api/v1/enrollment`
- signed bootstrap config returned from enrollment
- telemetry upload using the device secret
- device state promotion from `enrolled` to `active`
- duplicate enrollment handling for the same device ID

## Scope Note

- This suite is server-side and HTTP-driven today.
- The roadmap item for enrollment E2E is being approached emulator-first.
- Physical-device verification is deferred until the emulator flow is in place.

## Test Harness

- Tests use `XMDM_TEST_POSTGRES_DSN`.
- The database is reset before each test.
- The suite uses the same HTTP handlers as the real server wiring.
- Assertions must prefer API responses over direct database reads.
- Do not add raw SQL checks for behavior that is already observable through the HTTP surface.

## Running

```sh
cd server
eval "$(../infra/test-db-env.sh)"
go test -p 1 -count=1 ./...
```
