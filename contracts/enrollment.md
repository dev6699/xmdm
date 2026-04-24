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
    "deviceId": "serial-123",
    "deviceIdUse": "serial"
  },
  "bootstrapExtras": {
    "customer": "Acme"
  }
}
```

- Success response: `200 application/json`
- Body:

```json
{
  "deviceId": "serial-123",
  "deviceSecret": "base64url-secret",
  "status": "enrolled"
}
```

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
  "serverProject": "rest",
  "enrollmentToken": "token",
  "deviceAdminPackageDownloadLocation": "https://cdn.example/launcher.apk",
  "deviceAdminPackageChecksum": "base64sha256",
  "deviceIdentityPolicy": {
    "deviceId": "serial-optional",
    "deviceIdUse": "serial"
  },
  "bootstrapExtras": {
    "customer": "Acme",
    "groups": ["field"]
  }
}
```

- Success response: `200 image/png`
- Body:

- QR code PNG encoding the Android provisioning JSON payload below.

### `POST /api/v1/enrollment/qr/json`

- Permission: `devices.write`
- Request body:

```json
{
  "serverUrl": "https://mdm.example",
  "serverProject": "rest",
  "enrollmentToken": "token",
  "deviceAdminPackageDownloadLocation": "https://cdn.example/launcher.apk",
  "deviceAdminPackageChecksum": "base64sha256",
  "deviceIdentityPolicy": {
    "deviceId": "serial-optional",
    "deviceIdUse": "serial"
  },
  "bootstrapExtras": {
    "customer": "Acme",
    "groups": ["field"]
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
    "com.xmdm.SERVER_PROJECT": "rest",
    "com.xmdm.ENROLLMENT_TOKEN": "token",
    "com.xmdm.DEVICE_ID_USE": "serial",
    "com.xmdm.CUSTOMER": "Acme",
    "com.xmdm.GROUP": "field"
  }
}
```

- Errors:
  - `400` invalid input or malformed JSON
  - `401` unauthorized
  - `403` forbidden
