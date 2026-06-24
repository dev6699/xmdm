# Release Artifacts And Deployment

XMDM uses GitHub Actions to publish the build outputs that operators need.

## Open Core Outputs

The open-core release workflow publishes:

- `xmdm-server-linux-amd64.tar.gz`
- `xmdm-server-linux-amd64.tar.gz.sha256`
- `server-image.txt` with the GHCR image reference and digest
- `xmdm-launcher-<release-tag>.apk`
- `xmdm-launcher-<release-tag>.apk.sha256`
- `checksums.txt`
- `release-manifest.txt`

The server image is pushed to GHCR. The launcher APK is published as a GitHub Release asset.
For GitHub Actions releases, add these repository secrets so the workflow can sign the launcher APK:

- `XMDM_ANDROID_RELEASE_KEYSTORE_B64`
- `XMDM_ANDROID_RELEASE_STORE_PASSWORD`
- `XMDM_ANDROID_RELEASE_KEY_ALIAS`
- `XMDM_ANDROID_RELEASE_KEY_PASSWORD`

Local `assembleRelease` builds still fall back to the debug keystore unless you pass the `xmdm.release.*` Gradle properties yourself.

Set them in GitHub at:

1. Open the repository.
2. Go to `Settings`.
3. Open `Secrets and variables`.
4. Choose `Actions`.
5. Click `New repository secret`.
6. Add each secret value above.

Set them like this:

```sh
base64 -w0 .secrets/android/xmdm-release.keystore
```

Paste that output into `XMDM_ANDROID_RELEASE_KEYSTORE_B64`, then set the remaining three secrets to the matching keystore password and key alias values.

## Generate The Keystore

Generate the signing key once on a local machine and keep the `.keystore` file outside the repo:

```sh
mkdir -p .secrets/android
keytool -genkeypair \
  -alias xmdm-release \
  -keyalg RSA \
  -keysize 2048 \
  -validity 10000 \
  -keystore .secrets/android/xmdm-release.keystore
```

Then base64-encode the file for `XMDM_ANDROID_RELEASE_KEYSTORE_B64`:

```sh
base64 -w0 .secrets/android/xmdm-release.keystore
```

Use the same alias and passwords in the GitHub Secrets values you set above.

## Publish The Launcher APK

The release workflow gives you an APK file, but the server serves the launcher from its managed app catalog. The launcher app row is seeded during bootstrap and is locked to version publishing. To publish the APK for enrollment:

1. Open the admin dashboard at `/admin/apps`.
2. Open the seeded managed app whose package name is `com.xmdm.launcher`.
3. Upload the released `xmdm-launcher-<release-tag>.apk` file.
4. Set the version code for the release.
5. Open the app detail page and use `Publish new version` to add a new APK; do not rename or retire the seeded app row.
6. Confirm the app detail page shows the latest published APK.
7. Use that app's latest published version checksum in the enrollment QR or bootstrap payload.

After that, the server will serve the launcher at `/api/v1/enrollment/agent.apk`, and enrollment can use that endpoint directly.

## Local Deployment Flow

1. Start the backend stack from `infra/`:

```sh
cd infra
docker compose -f docker-compose.yml -f docker-compose.server.yml up -d --build
```

2. To stop the stack later, run:

```sh
cd infra
docker compose -f docker-compose.yml -f docker-compose.server.yml down
```

3. Open `/admin/apps`, open the seeded `com.xmdm.launcher` app row, and use `Publish new version` to upload the released `xmdm-launcher-<release-tag>.apk`.
4. Confirm the app detail page shows the new latest published APK.
5. Generate enrollment QR/bootstrap data from the server and use the published checksum for the launcher download URL.

## Release Flow

1. Create a tagged release or run the workflow manually with a release tag.
2. Download the release assets from GitHub.
3. Use the server image reference from `server-image.txt` if you want to deploy the published image instead of building locally.
4. Publish the launcher APK into the server's app catalog so the enrollment endpoint can serve it at `/api/v1/enrollment/agent.apk`.
5. Generate enrollment QR/bootstrap data from the server and enroll devices against the deployed stack.
