# Server E2E

This directory holds the root-level end-to-end tests for the Go server. The tests run against the HTTP handler stack with a real Postgres database, but they do not start a socket listener.

## What Is Covered

- `TestAdminE2E` covers the admin console flow.
- `TestEnrollmentE2E` covers server-simulated enrollment and first-sync behavior.
- `TestManagedAppsAndFiles` covers adb-backed managed app and managed file delivery on a physical device.
- `TestCommandMQTT` covers MQTT command transport on a physical device.
- `TestCommandPolling` covers HTTP polling command transport on a physical device.

## Test Strategy

Keep the e2e suite split by intent:

- `Admin API` tests cover pure admin CRUD and session behavior with no adb.
- `Server-simulated device` tests cover enrollment and related device API behavior through HTTP only.
- `Real-device launcher` tests cover adb-backed launcher behavior on a physical Android device.

Do not mix these intents in a single test unless the overlap is intentional and clearly documented.

## Test Plan

The e2e suite is split into two different intentions:

### Server-Simulated Flows

These tests exercise server APIs with a synthetic device actor. They do not use adb or a real launcher.

Use this bucket for:

- enrollment token issuance
- `/api/v1/enrollment` response shape
- telemetry upload with the device secret
- duplicate enrollment handling
- command enqueue, ack, and expiry behavior

Current coverage:

- `TestEnrollmentE2E`

### Device-Backed Flows

These tests run against a real Android device and verify launcher behavior end to end.

Use this bucket for:

- launcher bootstrap
- real device enrollment
- managed file rendering
- managed app install
- MQTT command transport
- HTTP polling command transport

Current coverage:

- `TestManagedAppsAndFiles`
- `TestCommandMQTT`
- `TestCommandPolling`

### Recommended Taxonomy

- [`TestAdminE2E`](/home/puong/xmdm/server/e2e/admin_test.go) for admin API coverage.
- [`TestEnrollmentE2E`](/home/puong/xmdm/server/e2e/enrollment_test.go) for server-simulated device enrollment and sync behavior.
- [`TestManagedAppsAndFiles`](/home/puong/xmdm/server/e2e/content_test.go) for real-device managed file and app delivery.
- [`TestCommandMQTT`](/home/puong/xmdm/server/e2e/content_test.go) for real-device MQTT command transport.
- [`TestCommandPolling`](/home/puong/xmdm/server/e2e/content_test.go) for real-device HTTP polling command transport.

## Admin Flow

The admin E2E verifies:

- admin login and session handling
- CRUD for users, roles, apps, groups, policies, and devices
- app version upload and publish
- multipart file upload and artifact metadata persistence
- audit event capture for admin mutations
- logout and session invalidation

## Enrollment Flow

`TestEnrollmentE2E` verifies server-side enrollment behavior:

- enrollment token issuance through the admin API
- device binding through `/api/v1/enrollment`
- signed bootstrap config returned from enrollment
- telemetry upload using the device secret
- device state promotion from `enrolled` to `active`
- duplicate enrollment handling for the same device ID

## Content Flow

`TestManagedAppsAndFiles` is the physical-device content test. It does all of the following in one run:

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
12. Verifies on-device state with adb reads from the launcher sandbox and package manager.

## Command Flows

The command transport tests share the same device bootstrap and server setup, then split by transport:

### MQTT Command Flow

`TestCommandMQTT` does the following:

1. Boots the launcher with MQTT bootstrap extras.
2. Waits for `POST /api/v1/enrollment`.
3. Verifies the server marks the device enrolled through the API.
4. Enqueues a `ping` command through `POST /api/v1/admin/commands`.
5. Verifies the device receives the command over MQTT and acknowledges it.
6. Verifies the device does not fall back to the HTTP polling command endpoint.

### HTTP Polling Command Flow

`TestCommandPolling` does the following:

1. Boots the launcher without MQTT bootstrap extras.
2. Overrides the launcher command poll interval with `COMMAND_POLL_INTERVAL_MS=1000`.
3. Waits for `POST /api/v1/enrollment`.
4. Verifies the server marks the device enrolled through the API.
5. Enqueues a `ping` command through `POST /api/v1/admin/commands`.
6. Verifies the device polls `GET /api/v1/devices/{deviceId}/commands`.
7. Verifies the device acknowledges the command with `POST /api/v1/devices/{deviceId}/commands/{commandId}/ack`.

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
- Keep server-simulated flows free of adb dependencies.
- Keep device-backed flows focused on launcher and transport behavior, not pure server API coverage.

## Running

For the adb-backed content test on a physical device:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestManagedAppsAndFiles -count=1 ./e2e
```

The test uses `XMDM_TEST_POSTGRES_DSN` from `../infra/test-db-env.sh` and requires a connected device serial in `XMDM_ADB_SERIAL`.

For the MQTT command transport test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestCommandMQTT -count=1 ./e2e
```

For the HTTP polling command transport test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestCommandPolling -count=1 ./e2e
```

`TestCommandPolling` uses a short bootstrap override for the launcher poll interval so it completes faster than the production 30-second default.

```sh
cd server
eval "$(../infra/test-db-env.sh)"
go test -p 1 -count=1 ./...
```

To run the adb troubleshooting helper directly:

```sh
XMDM_ADB_SERIAL=<serial> XMDM_ADB_BOOTSTRAP_URI=<bootstrap-uri> XMDM_ADB_DEVICE_ID=<device-id> go test -run TestADBFlow -count=1 ./e2e
```
