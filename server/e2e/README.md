# Server E2E

This directory holds the root-level end-to-end tests for the Go server. The tests run against the HTTP handler stack with a real Postgres database, but they do not start a socket listener.

## What Is Covered

- `TestAdminE2E` covers the admin console flow.
- `TestEnrollmentE2E` covers the enrollment and first-sync flow.
- `TestContentE2E` covers adb-backed managed app and managed file delivery on a physical device.

## Admin Flow

The admin E2E verifies:

- admin login and session handling
- CRUD for users, roles, apps, groups, policies, and devices
- app version upload and publish
- multipart file upload and artifact metadata persistence
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
- The enrollment flow remains HTTP-driven here.
- The content flow now includes a physical-device adb-backed path.

## Content Flow

`TestContentE2E` is the physical-device content test. It does all of the following in one run:

1. Starts a real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Creates a managed file source blob with `POST /api/v1/files`.
4. Creates a managed file record with `POST /api/v1/managed-files`.
5. Uploads the Chrome APK artifact with `POST /api/v1/files`.
6. Creates and publishes a managed app version for `com.android.chrome` using that uploaded artifact.
7. Resets server-side enrollment state for the chosen device ID.
8. Builds an enrollment QR payload that points at the test server.
9. Uses adb to reinstall the launcher, clear launcher-private state, and reverse the server port onto the device.
10. Starts the launcher with the bootstrap payload on the physical device.
11. Waits for the launcher to enroll, fetch policy, render the managed file, and restore Chrome for the current user.
12. Enqueues a lightweight `ping` command through the admin API.
13. Verifies the server observes the device ACK with a `pong` response.
14. Repeats the command flow once with MQTT bootstrap extras and once without them so both transport modes stay covered.
15. Enqueues a short-lived `ping` command in polling mode, waits for it to expire, and verifies the server marks it `expired`.
16. Verifies on-device state with adb reads from the launcher sandbox and package manager.

Important details:

- The test no longer tears the device down after a successful run.
- Chrome is treated as a managed app restore path for a preloaded system package on this device.
- The managed file content is rendered into the launcher sandbox and checked after the run.
- The test logs periodic adb snapshots while it waits so a stalled run shows the current device state.

## Test Harness

- Tests use `XMDM_TEST_POSTGRES_DSN`.
- The database is reset before each test.
- The suite uses the same HTTP handlers as the real server wiring.
- Assertions must prefer API responses over direct database reads.
- Do not add raw SQL checks for behavior that is already observable through the HTTP surface.
- The adb helper reinstalls the launcher APK and clears launcher-private state before the run.

## Running

```sh
cd server
eval "$(../infra/test-db-env.sh)"
go test -p 1 -count=1 ./...
```

To run the adb troubleshooting helper directly:

```sh
XMDM_ADB_SERIAL=<serial> XMDM_ADB_BOOTSTRAP_URI=<bootstrap-uri> XMDM_ADB_DEVICE_ID=<device-id> go test -run TestADBFlow -count=1 ./e2e
```
