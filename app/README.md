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

### Conventions

- Keep the launcher UI in XML with ViewBinding.
- Keep Android-specific build outputs ignored by [app/.gitignore](/home/puong/xmdm/app/.gitignore).
- Keep package names aligned with the blueprint and contracts, currently `com.xmdm.launcher`.

### Current State

`M3-01 Kotlin Project` is the active agent milestone in this directory. The scaffold already builds, so follow-on work should extend the existing project instead of replacing it.
