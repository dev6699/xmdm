# App

Android agent implementation lives here.

This directory is reserved for the Kotlin device agent, its app modules, and related Android-specific build assets.

## How To Work Here

### Build

Run the Android build from this directory:

```sh
cd app
./gradlew assembleDebug
```

The wrapper is checked in, so you do not need a system `gradle` install.

### Run On Device

Pick a connected device first:

```sh
export ADB_SERIAL="$(adb devices -l | awk 'NR==2 {print $1}')"
```

Install the debug APK on that device:

```sh
adb -s "$ADB_SERIAL" install -r app/build/outputs/apk/debug/xmdm-agent-debug.apk
```

Launch the main screen:

```sh
adb -s "$ADB_SERIAL" shell am start -n com.xmdm.launcher/.MainActivity
```

Launch the recovery screen:

```sh
adb -s "$ADB_SERIAL" shell am start -n com.xmdm.launcher/.recovery.RecoveryActivity --es com.xmdm.launcher.recovery.EXTRA_STAGE bootstrap --es com.xmdm.launcher.recovery.EXTRA_MESSAGE test
```

Launch the main screen with a bootstrap payload encoded as `base64url:<payload>`:

```sh
adb -s "$ADB_SERIAL" shell am start -n com.xmdm.launcher/.MainActivity -d 'base64url:<payload>'
```

If you have the QR JSON from the server, you can turn it into the payload like this:

```sh
json='{"android.app.extra.PROVISIONING_SERVER_URL":"http://192.168.0.168:8080","android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME":"com.xmdm.launcher/.AdminReceiver","android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE":{"com.xmdm.BASE_URL":"http://192.168.0.168:8080","com.xmdm.SERVER_PROJECT":"rest","com.xmdm.ENROLLMENT_TOKEN":"<token>","com.xmdm.DEVICE_ID":"device-999","com.xmdm.DEVICE_ID_USE":"serial","CUSTOMER":"Acme"}}'
payload="$(printf '%s' "$json" | base64 -w0 | tr '+/' '-_' | tr -d '=')"
adb -s "$ADB_SERIAL" shell am start -n com.xmdm.launcher/.MainActivity -d "base64url:$payload"
```

### Toolchain

The current scaffold uses:

- Gradle `8.7`
- Android Gradle Plugin `8.5.2`
- Kotlin `1.9.24`
- Java `17`
- `compileSdk` / `targetSdk` `34`
- `minSdk` `26`

### Layout

The app uses a single-module Android project:

- `app/build.gradle.kts` for the Android module build
- `app/settings.gradle.kts` for Gradle setup
- `app/src/main/AndroidManifest.xml` for app entry points
- `app/src/main/java/com/xmdm/launcher/` for Kotlin sources
- `app/src/main/res/` for XML layouts, strings, colors, and theme resources

### Persistence

The first local state store lives in `app/src/main/java/com/xmdm/launcher/state/`.
It uses DataStore preferences to keep:

- bootstrap data
- device identity and secret
- policy snapshot cache metadata

`MainActivity` reads the stored state on startup and shows whether each piece was restored.
The store has unit coverage that verifies save, reload, and clear behavior.

### Bootstrap Parsing

Bootstrap JSON is parsed in `app/src/main/java/com/xmdm/launcher/bootstrap/`.
The parser accepts the Android provisioning payload shape from `contracts/enrollment.md` and the flat fallback form used for manual or ADB intake.

`MainActivity` accepts raw bootstrap JSON from `com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON` or `Intent.EXTRA_TEXT`, parses it, and persists the normalized bootstrap state.
For adb-driven checks, `com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON_B64` accepts the same JSON encoded with base64 so shell quoting does not mangle the payload.
`MainActivity` also accepts `data:base64url:<payload>` on the launch intent, which is the most reliable adb path for device-side validation.
Unit tests cover both the canonical Android provisioning JSON and the flat fallback JSON form.

### Enrollment And First Config

When bootstrap data is present and the device is not yet enrolled, `MainActivity` now calls the enrollment API at `/api/v1/enrollment`.
The response supplies the device secret and the initial signed config snapshot, which are persisted locally as device identity and policy cache state.

### Retry Foundation

Retry helpers live in `app/src/main/java/com/xmdm/launcher/retry/`.
The current runner provides a small exponential-backoff utility that future config fetch and sync code can reuse.

### Config Sync

Config sync lives in `app/src/main/java/com/xmdm/launcher/sync/`.
It fetches a signed config snapshot through an injectable source, verifies the snapshot signature, retries transient fetch failures, and stores the last successful policy cache locally.

### Recovery UI

The operator recovery screen lives in `app/src/main/java/com/xmdm/launcher/recovery/`.
It surfaces bootstrap and enrollment failures, shows the latest failure message, and provides buttons to retry enrollment or reset enrollment state.
`Retry enrollment` reuses the original bootstrap payload that triggered the failure, so resetting local state does not lose the retry input.
`Reset enrollment state` only wipes enrollment identity and policy cache data; bootstrap input stays available for retry.
The screen also shows whether the app is currently the device owner.

### Device Owner Test

On a fresh, unprovisioned test device you can set the app as device owner with:

```sh
adb shell dpm set-device-owner com.xmdm.launcher/.AdminReceiver
```

This only works on a device that has not already been provisioned. On a normally used phone, Android will reject device-owner provisioning unless the device is reset back to a fresh state.

### Conventions

- Keep the launcher UI in XML with ViewBinding.
- Keep Android-specific build outputs ignored by [app/.gitignore](/home/puong/xmdm/app/.gitignore).
- Keep package names aligned with the blueprint and contracts, currently `com.xmdm.launcher`.

### Current State

The scaffold already builds, local persistence is in place, bootstrap parsing now persists canonical or fallback payloads, bootstrap state can now flow into enrollment and the initial signed config snapshot, config sync now retries transient failures before caching a verified snapshot, the recovery UI can surface setup failures, and `M3-02 Local Persistence` has passed a physical-device reboot check.
The main launcher screen also shows whether the app is currently device owner.
