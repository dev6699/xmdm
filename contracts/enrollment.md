# Enrollment Contract

This document defines the enrollment feature HTTP surface in `server/cmd/server`.

Path prefix:

- The enrollment API surface lives under `/api/v1/enrollment/...`
- It uses the same authenticated admin session semantics as the admin console

## Device Enrollment

### `POST /api/v1/enrollment`

- Request: none
- Request body:

```json
{
  "enrollmentToken": "base64url-secret",
  "deviceIdentityPolicy": {
    "deviceId": "serial-123"
  },
  "bootstrapExtras": {
    "secondaryBaseUrl": "https://backup.example"
  }
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "deviceId": "serial-123",
  "deviceSecret": "base64url-secret",
  "status": "enrolled",
  "config": {
    "version": "1",
    "device": {
      "deviceId": "serial-123"
    },
    "policy": {
      "bootstrapExtras": {
        "secondaryBaseUrl": "https://backup.example"
      }
    },
    "apps": [
      {
        "appId": "uuid",
        "packageName": "com.example.app",
        "name": "Example App",
        "versionId": "uuid",
        "versionName": "1.0.0",
        "versionCode": 100,
        "artifactId": "artifact-1",
        "checksum": "sha256-app-abc",
        "downloadPath": "/api/v1/devices/serial-123/apps/uuid/versions/uuid/artifact"
      }
    ],
    "files": [
      {
        "fileId": "uuid",
        "name": "device-config.txt",
        "path": "device-config.txt",
        "checksum": "sha256-file-abc",
        "mimeType": "text/plain",
        "downloadPath": "/api/v1/devices/serial-123/managed-files/uuid/artifact",
        "replaceVariables": true
      }
    ],
    "certificates": [],
    "commands": [],
    "signature": "hmac-sha256"
  }
}
```

- The config snapshot is signed with the device secret and uses HMAC-SHA256 over the canonical JSON body with an empty `signature` field.
- App entries in `config.apps` describe the managed package, the published version, the artifact checksum, and the device-scoped download path used by the launcher to fetch install bytes.
- File entries in `config.files` describe the managed device path, the artifact checksum, the MIME type, the template flag, and the device-scoped managed-file download path used by the launcher to fetch and render file content.
- Managed file entries are created through `/api/v1/managed-files` and point at an existing uploaded file record.

- Errors:
  - `400` invalid input or malformed JSON
  - `409` duplicate enrollment or consumed token
  - `404` unknown token
  - `500` internal error

## Enrollment Tokens

### `POST /api/v1/enrollment/tokens`

- Permission: `devices.write`
- Request body:

```json
{
  "ttlSeconds": 3600
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "issued",
  "expiresAt": "2026-04-23T00:00:00Z",
  "createdAt": "2026-04-23T00:00:00Z",
  "updatedAt": "2026-04-23T00:00:00Z",
  "token": "base64url-secret"
}
```

### `POST /api/v1/enrollment/tokens/validate`

- Request: none
- Request body:

```json
{
  "token": "base64url-secret"
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "issued",
  "expiresAt": "2026-04-23T00:00:00Z",
  "createdAt": "2026-04-23T00:00:00Z",
  "updatedAt": "2026-04-23T00:00:00Z"
}
```

### `POST /api/v1/enrollment/tokens/consume`

- Request: none
- Request body:

```json
{
  "token": "base64url-secret"
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "consumed",
  "expiresAt": "2026-04-23T00:00:00Z",
  "consumedAt": "2026-04-23T00:00:00Z",
  "createdAt": "2026-04-23T00:00:00Z",
  "updatedAt": "2026-04-23T00:00:00Z"
}
```

### `DELETE /api/v1/enrollment/tokens/{id}`

- Permission: `devices.write`
- Success response: `200 application/json`
- Body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "revoked",
  "expiresAt": "2026-04-23T00:00:00Z",
  "revokedAt": "2026-04-23T00:00:00Z",
  "createdAt": "2026-04-23T00:00:00Z",
  "updatedAt": "2026-04-23T00:00:00Z"
}
```

## QR Payload

### `POST /api/v1/enrollment/qr`

- Permission: `devices.write`
- Request body:

```json
{
  "serverUrl": "https://mdm.example",
  "enrollmentToken": "token",
  "deviceAdminPackageDownloadLocation": "https://cdn.example/launcher.apk",
  "deviceAdminPackageChecksum": "base64sha256",
  "deviceIdentityPolicy": {
    "deviceId": "serial-optional"
  },
  "bootstrapExtras": {
    "secondaryBaseUrl": "https://backup.example"
  }
}
```

- Success response: `200 image/png`
- Body:

- QR code PNG encoding the Android provisioning JSON payload below.
- The server currently emits `com.xmdm.SECONDARY_BASE_URL` with the same value as `com.xmdm.BASE_URL`.
- `bootstrapExtras.secondaryBaseUrl` is still accepted and preserved in enrollment data, but it does not change the QR payload.

### `POST /api/v1/enrollment/qr/json`

- Permission: `devices.write`
- Request body:

```json
{
  "serverUrl": "https://mdm.example",
  "enrollmentToken": "token",
  "deviceAdminPackageDownloadLocation": "https://cdn.example/launcher.apk",
  "deviceAdminPackageChecksum": "base64sha256",
  "deviceIdentityPolicy": {
    "deviceId": "serial-optional"
  },
  "bootstrapExtras": {
    "secondaryBaseUrl": "https://backup.example"
  }
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME": "com.xmdm.launcher/.AdminReceiver",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": "https://cdn.example/launcher.apk",
  "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM": "base64sha256",
  "android.app.extra.PROVISIONING_LEAVE_ALL_SYSTEM_APPS_ENABLED": true,
  "android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE": {
    "com.xmdm.BASE_URL": "https://mdm.example",
    "com.xmdm.SECONDARY_BASE_URL": "https://mdm.example",
    "com.xmdm.ENROLLMENT_TOKEN": "token",
    "com.xmdm.DEVICE_ID": "serial-optional"
  }
}
```

- The server currently emits `com.xmdm.SECONDARY_BASE_URL` with the same value as `com.xmdm.BASE_URL`.
- `bootstrapExtras.secondaryBaseUrl` is still accepted and preserved in enrollment data, but it does not change the QR payload.

- Errors:
  - `400` invalid input or malformed JSON
  - `401` unauthorized
  - `403` forbidden

## Telemetry Upload

### `POST /api/v1/devices/{deviceId}/telemetry`

- Device authentication:
  - `X-XMDM-Device-Secret: base64url-secret`
- Request body:

```json
{
  "observedAt": "2026-04-24T12:00:00Z",
  "heartbeat": {
    "online": true
  },
  "battery": {
    "level": 87
  },
  "network": {
    "connected": true
  },
  "location": {
    "latitude": 1.3521,
    "longitude": 103.8198
  },
  "appState": {
    "packageName": "com.example.app",
    "foreground": true
  }
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "deviceId": "serial-123",
  "observedAt": "2026-04-24T12:00:00Z",
  "payload": {
    "heartbeat": {
      "online": true
    },
    "battery": {
      "level": 87
    }
  }
}
```

- Errors:
  - `400` invalid input or malformed JSON
  - `401` unauthorized
  - `404` unknown device
  - `500` internal error
