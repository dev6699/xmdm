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

### Conventions

- Keep the launcher UI in XML with ViewBinding.
- Keep Android-specific build outputs ignored by [app/.gitignore](/home/puong/xmdm/app/.gitignore).
- Keep package names aligned with the blueprint and contracts, currently `com.xmdm.launcher`.

### Current State

The scaffold already builds, local persistence is in place, bootstrap parsing now persists canonical or fallback payloads, bootstrap state can now flow into enrollment and the initial signed config snapshot, config sync now retries transient failures before caching a verified snapshot, and `M3-02 Local Persistence` has passed a physical-device reboot check.
