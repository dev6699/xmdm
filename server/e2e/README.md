# Server E2E

This directory holds the root-level end-to-end tests for the Go server. The tests run against the HTTP handler stack with a real Postgres database, but they do not start a socket listener.

## What Is Covered

- `TestAdminE2E` covers the admin console flow.
- `TestEnrollmentE2E` covers server-simulated enrollment and first-sync behavior.
- `TestManagedAppsAndFiles` covers adb-backed managed app and managed file delivery on a physical device.
- `TestManagedAppsAndFilesRemoval` covers adb-backed managed app and managed file removal on a physical device.
- `TestDeviceLogsUpload` covers adb-backed device log upload and recorded-log API verification on a physical device.
- `TestDeviceInfoReporting` covers adb-backed device-info reporting and admin export on a physical device.
- `TestKioskMode` covers adb-backed kiosk enforcement on a physical device.
- `TestPackageRules` covers adb-backed package suspension enforcement on a physical device.
- `TestPolicySync` covers adb-backed policy refresh after an admin update on a physical device.
- `TestCommandMQTT` covers MQTT command transport on a physical device.
- `TestCommandMQTTSyncConfig` covers MQTT command-triggered config sync on a physical device.
- `TestCommandPolling` covers HTTP polling command transport on a physical device.
- `TestCommandBrokerOutageRecovery` covers MQTT outage fallback and recovery on a physical device.

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
- kiosk enforcement
- package suspension enforcement
- managed file rendering
- managed app install
- device log upload and API readback
- MQTT command transport
- MQTT command-triggered config sync
- HTTP polling command transport

Current coverage:

- `TestManagedAppsAndFiles`
- `TestManagedAppsAndFilesRemoval`
- `TestDeviceLogsUpload`
- `TestDeviceInfoReporting`
- `TestKioskMode`
- `TestPackageRules`
- `TestPolicySync`
- `TestCommandMQTT`
- `TestCommandPolling`
- `TestCommandBrokerOutageRecovery`

The device-log upload test covers the structured launcher events emitted by the app:

- `launcher` startup
- `bootstrap` intake and parsing
- `enrollment` attempt and result
- `sync` refresh success or failure
- `files` apply and removal
- `apps` apply and removal
- `commands` transport, polling, and command-triggered sync

### Device Info Flow

`TestDeviceInfoReporting` is the physical-device device-info test. It does all of the following in one run:

1. Starts a real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Creates the managed file and Chrome app fixtures used by the content install test.
4. Uses adb to reinstall the launcher, clear launcher-private state, and reverse the server port onto the device.
5. Starts the launcher with the bootstrap payload on the physical device.
6. Waits for the launcher to enroll, fetch the signed device config snapshot, and upload a structured device-info report.
7. Verifies the uploaded payload contains device inventory and config fields such as model, app package, config revision, and managed bucket versions.
8. Verifies the server exports the recorded device-info rows through `GET /api/v1/device-info`.

### Recommended Taxonomy

- [`TestAdminE2E`](/home/puong/xmdm/server/e2e/admin_test.go) for admin API coverage.
- [`TestEnrollmentE2E`](/home/puong/xmdm/server/e2e/enrollment_test.go) for server-simulated device enrollment and sync behavior.
- [`TestManagedAppsAndFiles`](/home/puong/xmdm/server/e2e/content_test.go) for real-device managed file and app delivery.
- [`TestManagedAppsAndFilesRemoval`](/home/puong/xmdm/server/e2e/content_test.go) for real-device managed file and app removal.
- [`TestDeviceLogsUpload`](/home/puong/xmdm/server/e2e/content_test.go) for real-device device log upload.
- [`TestDeviceInfoReporting`](/home/puong/xmdm/server/e2e/deviceinfo_test.go) for real-device device info reporting and export.
- [`TestKioskMode`](/home/puong/xmdm/server/e2e/content_test.go) for real-device kiosk enforcement.
- [`TestPackageRules`](/home/puong/xmdm/server/e2e/content_test.go) for real-device package suspension enforcement.
- [`TestCommandMQTT`](/home/puong/xmdm/server/e2e/content_test.go) for real-device MQTT command transport.
- [`TestCommandMQTTSyncConfig`](/home/puong/xmdm/server/e2e/content_test.go) for real-device MQTT command-triggered config sync.
- [`TestCommandPolling`](/home/puong/xmdm/server/e2e/content_test.go) for real-device HTTP polling command transport.
- [`TestCommandBrokerOutageRecovery`](/home/puong/xmdm/server/e2e/content_test.go) for real-device MQTT outage fallback and recovery.

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
- signed bootstrap config fetched from `/api/v1/devices/{deviceId}/config`
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
11. Waits for the launcher to enroll, fetch the signed device config snapshot, receive the rendered managed file, and restore Chrome for the current user.
12. Verifies on-device state with adb reads from the launcher sandbox and package manager.

`TestManagedAppsAndFilesRemoval` is the physical-device content removal test. It does all of the following in one run:

1. Starts the same real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Creates the managed file and Chrome app fixtures used by the content install test.
4. Starts the launcher; the test server injects a short config-sync interval through the signed config snapshot runtime bucket.
5. Waits for the launcher to enroll, fetch the signed device config snapshot, receive the rendered managed file, and restore Chrome for the current user.
6. Retires the managed file record and the managed Chrome app on the server.
7. Waits for the launcher to fetch the updated device config snapshot after the retire operations.
8. Verifies the managed file has been removed from the launcher sandbox and Chrome has been uninstalled from the device.

## Kiosk Flow

`TestKioskMode` is the physical-device kiosk test. It does all of the following in one run:

1. Starts a real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Resets server-side enrollment state for the chosen device ID.
4. Creates an active kiosk policy and keeps the enrollment payload itself free of kiosk-specific extras.
5. Uses adb to reinstall the launcher, clear launcher-private state, and reverse the server port onto the device.
6. Starts the launcher with the bootstrap payload on the physical device.
7. Waits for the launcher to enroll and fetch the signed device config snapshot.
8. Verifies on-device kiosk state with `dumpsys activity activities`.

## Package Rules Flow

`TestPackageRules` is the physical-device package policy test. It does all of the following in one run:

1. Starts a real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Uploads the Chrome APK artifact and publishes it as a managed app.
4. Creates an active policy that blocks `com.android.chrome` through the policy restrictions JSON.
5. Uses adb to reinstall the launcher, clear launcher-private state, and reverse the server port onto the device.
6. Starts the launcher with the bootstrap payload on the physical device.
7. Waits for the launcher to enroll, fetch the signed device config snapshot, restore Chrome, and suspend the package on-device.
8. Verifies the package suspension state with `dumpsys package com.android.chrome`.

## Policy Sync Flow

`TestPolicySync` is the physical-device policy refresh test. It does all of the following in one run:

1. Starts a real HTTP handler stack with a real Postgres test database.
2. Uploads the launcher APK artifact to the test server so the device can reprovision itself from the same server under test.
3. Uploads the Chrome APK artifact and publishes it as a managed app.
4. Creates a benign active policy, then later patches it to block `com.android.chrome`.
5. Uses adb to reinstall the launcher, clear launcher-private state, and reverse the server port onto the device.
6. Starts the launcher; the test server injects a short config-sync interval through the signed config snapshot runtime bucket on the physical device.
7. Waits for the launcher to enroll, fetch policy, restore Chrome, and then fetch the updated device config snapshot after the policy patch.
8. Verifies the package suspension state with `dumpsys package com.android.chrome`.

## Command Flows

The command transport tests share the same device bootstrap and server setup, then split by transport:

### MQTT Command Flow

`TestCommandMQTT` does the following:

1. Boots the launcher; the test server supplies MQTT through the signed config snapshot runtime bucket.
2. Waits for `POST /api/v1/enrollment`.
3. Verifies the server marks the device enrolled through the API.
4. Enqueues a `ping` command through `POST /api/v1/admin/commands`.
5. Verifies the device receives the command over MQTT and acknowledges it.
6. Verifies the device does not fall back to the HTTP polling command endpoint.

### MQTT Command-Triggered Config Sync

`TestCommandMQTTSyncConfig` does the following:

1. Boots the launcher; the test server supplies MQTT through the signed config snapshot runtime bucket.
2. Waits for `POST /api/v1/enrollment`.
3. Verifies the server marks the device enrolled through the API.
4. Enqueues a `sync_config` command through `POST /api/v1/admin/commands`.
5. Verifies the device receives the command over MQTT and acknowledges it after refreshing config.
6. Verifies the device fetches `GET /api/v1/devices/{deviceId}/config` again after the command.
7. Verifies the device does not fall back to the HTTP polling command endpoint.

### HTTP Polling Command Flow

`TestCommandPolling` does the following:

1. Boots the launcher without an MQTT runtime bucket in the signed config snapshot.
2. Uses the short command-poll interval from the signed config snapshot runtime bucket.
3. Waits for `POST /api/v1/enrollment`.
4. Verifies the server marks the device enrolled through the API.
5. Enqueues a `ping` command through `POST /api/v1/admin/commands`.
6. Verifies the device polls `GET /api/v1/devices/{deviceId}/commands`.
7. Verifies the device acknowledges the command with `POST /api/v1/devices/{deviceId}/commands/{commandId}/ack`.

Important details:

- The test no longer tears the device down after a successful run.
- Chrome is treated as a managed app restore path for a preloaded system package on this device.
- The managed file content is rendered on the server, downloaded by the launcher, and checked after the run.
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

For the adb-backed content removal test on a physical device:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestManagedAppsAndFilesRemoval -count=1 ./e2e
```

`TestManagedAppsAndFilesRemoval` uses the short config-sync interval from the signed config snapshot runtime bucket so the launcher picks up the retire operations quickly.

For the adb-backed device log upload test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestDeviceLogsUpload -count=1 ./e2e
```

`TestDeviceLogsUpload` waits for the launcher to emit structured lifecycle logs and upload them through `POST /api/v1/devices/{deviceId}/logs`.

For the adb-backed device info reporting test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestDeviceInfoReporting -count=1 ./e2e
```

`TestDeviceInfoReporting` waits for the launcher to upload a structured device inventory report and then exports the recorded rows through the admin API.

For the adb-backed kiosk enforcement test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestKioskMode -count=1 ./e2e
```

For the adb-backed package rules test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestPackageRules -count=1 ./e2e
```

For the adb-backed policy sync test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestPolicySync -count=1 ./e2e
```

`TestPolicySync` uses the short config-sync interval from the signed config snapshot runtime bucket so the launcher refreshes the signed device config snapshot quickly after the admin policy patch.

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

For the MQTT command-triggered config sync test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestCommandMQTTSyncConfig -count=1 ./e2e
```

`TestCommandPolling` uses the short poll interval from the signed config snapshot runtime bucket so it completes faster than the production 30-second default.

### MQTT Outage Recovery Flow

`TestCommandBrokerOutageRecovery` does the following:

1. Boots the launcher with MQTT enabled and a short polling interval from the signed config snapshot runtime bucket.
2. Verifies the device enrolls and receives an initial command over MQTT.
3. Stops the local MQTT broker container.
4. Verifies the next command falls back to HTTP polling and still gets acknowledged.
5. Restarts the MQTT broker container.
6. Verifies the launcher resumes MQTT command delivery without re-enrollment.

This test uses the local `infra/docker-compose.yml` broker container as the outage control point.

For the MQTT outage recovery test:

```sh
eval "$(../infra/test-db-env.sh)"
cd server
XMDM_ADB_SERIAL=<connected-device-serial> go test -run TestCommandBrokerOutageRecovery -count=1 ./e2e
```

```sh
cd server
eval "$(../infra/test-db-env.sh)"
go test -p 1 -count=1 ./...
```

To run the adb troubleshooting helper directly:

```sh
XMDM_ADB_SERIAL=<serial> XMDM_ADB_BOOTSTRAP_URI=<bootstrap-uri> XMDM_ADB_DEVICE_ID=<device-id> go test -run TestADBFlow -count=1 ./e2e
```
