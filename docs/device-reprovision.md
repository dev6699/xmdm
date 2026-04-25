# Device Reprovision Runbook

Use [`infra/reprovision-device.sh`](../infra/reprovision-device.sh) with `XMDM_ADB_SERIAL` set. That is the supported reprovision path. It generates a fresh device id unless you override `XMDM_DEVICE_ID`.

## What this runbook covers

- Clearing launcher state and Chrome before reprovisioning
- Minting a fresh enrollment token
- Generating the launcher bootstrap payload from the server's QR JSON
- Uploading `chrome.apk` and publishing Chrome if the server fixtures are missing
- Picking a fresh device id that is not already present in the server's device list

## Prerequisites

- The device-facing XMDM server URL is `http://192.168.0.168:8080`
- The host-side admin API is reachable at `http://127.0.0.1:8080`
- You can log into the admin console with the default local credentials unless your environment changed them
- `adb` can see the target device

Check the device first:

```sh
adb devices -l
```

## Helper Command

Run the reprovision helper from the repo root:

```sh
export ADB_SERIAL="$(adb devices -l | awk 'NR==2 {print $1}')"
XMDM_ADB_SERIAL="$ADB_SERIAL" ./infra/reprovision-device.sh
```

The helper handles token minting, QR JSON generation, launcher reset, and the bootstrap launch. Use the manual steps only if you are debugging the server payload.

Expected screen states:

- `managed apps: idle` right after startup
- `managed apps: verifying policy snapshot`
- `managed apps: downloading Chrome 138.0.7204.179 (1/1) ...`
- `managed apps: installing Chrome 138.0.7204.179 (1/1)`

The live banner shows downloaded bytes, total size, and percentage while the APK is streaming.

### Why Chrome gets installed

The launcher does not guess the app list locally. After enrollment, the server returns a signed config snapshot that includes an `apps` array. The helper ensures the Chrome file, app, and published version exist on the server before it starts the launcher, so the snapshot contains Chrome and the device installs it.

If you need to trace the decision, look at:

- `server/internal/enrollment/http/routes.go` for the server-side snapshot assembly
- `server/internal/enrollment/snapshot.go` for the shape of the signed snapshot, including `apps`
- `app/src/main/java/com/xmdm/launcher/apps/ManagedAppInstallCoordinator.kt` for the device-side logic that turns `apps` into install work

## Notes

- If you are retrying from the recovery screen, the `Retry enrollment` button reuses the saved bootstrap payload.
- `Reset enrollment state` only clears enrollment identity and policy. It does not wipe the saved bootstrap input.
- The helper force-stops `com.xmdm.launcher`, clears its persisted debug state, removes `com.android.chrome` from user 0, sets `com.xmdm.launcher.EXTRA_RESET_STATE=true`, and ensures the Chrome file, app, and published version records exist before it starts the launcher.
- The launcher streams APK downloads to disk, so the managed-app install path should not allocate the full Chrome APK in memory.
