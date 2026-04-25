# Admin Console Contract

This document defines the admin console HTTP surface in `server/cmd/server`.

Path prefix:

- The live versioned admin session surface lives under `/api/v1/admin/...`
- The live versioned admin resource surface lives under `/api/v1/...`
- The same contract can be mounted under `/admin/...` by the console wrapper
- Both surfaces preserve the same semantics and payload shapes

## Session Routes

### `GET /api/v1/admin/login`

- Request: none
- Response: `200 text/html`
- Body: login form HTML

### `POST /api/v1/admin/login`

- Request: `application/x-www-form-urlencoded`
- Fields:
  - `username` `string`
  - `password` `string`
- Success response: `303 See Other`
- Headers:
  - `Set-Cookie: session=<session-id>; HttpOnly; Path=/`
  - `Location: /api/v1/admin/me`
- Errors:
  - `400` invalid form
  - `401` invalid credentials

### `POST /api/v1/admin/logout`

- Request: none
- Success response: `204 No Content`
- Headers:
  - `Set-Cookie: session=; Max-Age=-1; HttpOnly; Path=/`

### `GET /api/v1/admin/me`

- Request:
  - session cookie required
- Success response: `200 application/json`
- Body:
  - `{"user":"<username>"}`
- Errors:
  - `401` unauthorized

### `GET /api/v1/admin/commands`

- Permission: `admin.write`
- Success response: `200 text/html`
- Body: simple command creation form with target type, device ID, group ID, and payload fields

### `POST /api/v1/admin/commands`

- Permission: `admin.write`
- Request body:
  - `application/json` for API clients, or
  - `application/x-www-form-urlencoded` for the browser form
- Fields:
  - `type` `string`
  - `payload` `object` or JSON string
  - `target.type` `device`, `group`, or `broadcast`
  - `target.deviceId` required when `target.type=device`
  - `target.groupId` required when `target.type=group`
- Success response: `200 application/json`
- Body:

```json
{
  "commands": [
    {
      "id": "uuid",
      "type": "reboot",
      "status": "queued",
      "deviceId": "device-123"
    }
  ]
}
```

## Users

### `GET /api/v1/users`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "email": "user@example.com",
    "roleId": "uuid"
  }
]
```

### `POST /api/v1/users`

- Permission: `admin.write`
- Request body:

```json
{
  "email": "user@example.com",
  "passwordHash": "hash",
  "roleId": "uuid"
}
```

Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "email": "user@example.com",
  "roleId": "uuid"
}
```

### `PATCH /api/v1/users/{id}`

- Permission: `admin.write`
- Request body:

```json
{
  "email": "user@example.com",
  "passwordHash": "hash",
  "roleId": "uuid"
}
```

Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "email": "user@example.com",
  "roleId": "uuid"
}
```

### `DELETE /api/v1/users/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "email": "user@example.com",
  "roleId": "uuid"
}
```

## Roles

### `GET /api/v1/roles`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "Role name",
    "permissions": ["admin.read", "admin.write"]
  }
]
```

### `POST /api/v1/roles`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Role name",
  "permissions": ["admin.read", "admin.write"]
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Role name",
  "permissions": ["admin.read", "admin.write"]
}
```

### `PATCH /api/v1/roles/{id}`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Role name",
  "permissions": ["admin.read", "admin.write"]
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Role name",
  "permissions": ["admin.read", "admin.write"]
}
```

### `DELETE /api/v1/roles/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "Role name",
  "permissions": ["admin.read", "admin.write"]
}
```

## Apps

### `GET /api/v1/apps`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "packageName": "com.example.app",
    "name": "App name"
  }
]
```

### `POST /api/v1/apps`

- Permission: `admin.write`
- Request body:

```json
{
  "packageName": "com.example.app",
  "name": "App name"
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "packageName": "com.example.app",
  "name": "App name"
}
```

### `PATCH /api/v1/apps/{id}`

- Permission: `admin.write`
- Request body:

```json
{
  "packageName": "com.example.app",
  "name": "App name"
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "packageName": "com.example.app",
  "name": "App name"
}
```

### `DELETE /api/v1/apps/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "packageName": "com.example.app",
  "name": "App name"
}
```

### `GET /api/v1/apps/{id}/versions`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "appId": "uuid",
    "status": "published",
    "versionName": "1.0.0",
    "versionCode": 100,
    "artifactId": "artifact-1",
    "checksum": "sha256-abc",
    "publishedAt": "2026-04-23T00:00:00Z",
    "createdAt": "2026-04-23T00:00:00Z"
  }
]
```

- Errors:
  - `404` app not found or retired

### `POST /api/v1/apps/{id}/versions`

- Permission: `admin.write`
- Request body:

```json
{
  "versionName": "1.0.0",
  "versionCode": 100,
  "artifactId": "artifact-1",
  "checksum": "sha256-abc",
  "publish": true
}
```

- `artifactId` is optional.
- `publish` marks the new version as published when `true`; otherwise it stays uploaded.
- If `artifactId` is provided, the referenced artifact checksum must match `checksum`.
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "appId": "uuid",
  "status": "published",
  "versionName": "1.0.0",
  "versionCode": 100,
  "artifactId": "artifact-1",
  "checksum": "sha256-abc",
  "publishedAt": "2026-04-23T00:00:00Z",
  "createdAt": "2026-04-23T00:00:00Z"
}
```

- Errors:
  - `400` invalid input or malformed JSON
  - `404` app not found or retired
  - `409` duplicate app/package/version conflict

## Files

### `GET /api/v1/files`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "launcher.apk",
    "artifactId": "uuid",
    "checksum": "sha256-file-abc",
    "mimeType": "application/vnd.android.package-archive",
    "artifact": {
      "id": "uuid",
      "tenantId": "uuid",
      "status": "active",
      "updatedAt": "2026-04-23T00:00:00Z",
      "deletedAt": null,
      "storageKey": "artifacts/launcher.apk",
      "checksum": "sha256-file-abc",
      "sizeBytes": 1024,
      "mimeType": "application/vnd.android.package-archive"
    }
  }
]
```

### `POST /api/v1/files`

- Permission: `admin.write`
- Request body:

`multipart/form-data`

- Fields:
  - `name` `string`
  - `storageKey` `string`
  - `checksum` `string`
  - `sizeBytes` `integer`
  - `mimeType` `string`
  - `file` `binary`

- `checksum` must match the SHA-256 digest of the uploaded bytes encoded as base64url without padding.
- The server streams the uploaded `file` part into configured object storage and persists both the logical file record and the backing artifact metadata.
- The server also accepts the JSON metadata-only shape for internal registration flows, but multipart upload is the primary path.
- This endpoint is the raw artifact upload path used by app versions and other reusable blobs.
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "launcher.apk",
  "artifactId": "uuid",
  "checksum": "sha256-file-abc",
  "mimeType": "application/vnd.android.package-archive",
  "artifact": {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "storageKey": "artifacts/launcher.apk",
    "checksum": "sha256-file-abc",
    "sizeBytes": 1024,
    "mimeType": "application/vnd.android.package-archive"
  }
}
```

### `DELETE /api/v1/files/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "launcher.apk",
  "artifactId": "uuid",
  "checksum": "sha256-file-abc",
  "mimeType": "application/vnd.android.package-archive",
  "artifact": {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "storageKey": "artifacts/launcher.apk",
    "checksum": "sha256-file-abc",
    "sizeBytes": 1024,
    "mimeType": "application/vnd.android.package-archive"
  }
}
```

### `GET /api/v1/managed-files`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "fileId": "uuid",
    "path": "device-config.txt",
    "replaceVariables": true,
    "file": {
      "id": "uuid",
      "tenantId": "uuid",
      "status": "active",
      "updatedAt": "2026-04-23T00:00:00Z",
      "deletedAt": null,
      "name": "device-config.txt",
      "artifactId": "uuid",
      "checksum": "sha256-file-abc",
      "mimeType": "text/plain",
      "artifact": {
        "id": "uuid",
        "tenantId": "uuid",
        "status": "active",
        "updatedAt": "2026-04-23T00:00:00Z",
        "deletedAt": null,
        "storageKey": "artifacts/device-config.txt",
        "checksum": "sha256-file-abc",
        "sizeBytes": 1024,
        "mimeType": "text/plain"
      }
    }
  }
]
```

### `POST /api/v1/managed-files`

- Permission: `admin.write`
- Request body:

```json
{
  "fileId": "uuid",
  "path": "device-config.txt",
  "replaceVariables": true
}
```

- `fileId` must reference an active file record created through `/api/v1/files`.
- The managed-file record tells the launcher where to write the content on-device and whether to render the file as a template.
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "fileId": "uuid",
  "path": "device-config.txt",
  "replaceVariables": true
}
```

### `DELETE /api/v1/managed-files/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "fileId": "uuid",
  "path": "device-config.txt",
  "replaceVariables": true
}
```

## Certificates

### `GET /api/v1/certificates`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "wifi-root-ca.pem",
    "artifactId": "uuid",
    "checksum": "sha256-cert-abc",
    "artifact": {
      "id": "uuid",
      "tenantId": "uuid",
      "status": "active",
      "updatedAt": "2026-04-23T00:00:00Z",
      "deletedAt": null,
      "storageKey": "artifacts/wifi-root-ca.pem",
      "checksum": "sha256-cert-abc",
      "sizeBytes": 512,
      "mimeType": "application/x-pem-file"
    }
  }
]
```

### `POST /api/v1/certificates`

- Permission: `admin.write`
- Request body:

`multipart/form-data`

- Fields:
  - `name` `string`
  - `storageKey` `string`
  - `checksum` `string`
  - `sizeBytes` `integer`
  - `mimeType` `string`
  - `file` `binary`

- `checksum` must match the SHA-256 digest of the uploaded bytes encoded as base64url without padding.
- The server streams the uploaded `file` part into configured object storage and persists both the logical certificate record and the backing artifact metadata.
- The server also accepts the JSON metadata-only shape for internal registration flows, but multipart upload is the primary path.
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "wifi-root-ca.pem",
  "artifactId": "uuid",
  "checksum": "sha256-cert-abc",
  "artifact": {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "storageKey": "artifacts/wifi-root-ca.pem",
    "checksum": "sha256-cert-abc",
    "sizeBytes": 512,
    "mimeType": "application/x-pem-file"
  }
}
```

### `DELETE /api/v1/certificates/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "wifi-root-ca.pem",
  "artifactId": "uuid",
  "checksum": "sha256-cert-abc",
  "artifact": {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "storageKey": "artifacts/wifi-root-ca.pem",
    "checksum": "sha256-cert-abc",
    "sizeBytes": 512,
    "mimeType": "application/x-pem-file"
  }
}
```

## Groups

### `GET /api/v1/groups`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "Group name"
  }
]
```

### `POST /api/v1/groups`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Group name"
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Group name"
}
```

### `PATCH /api/v1/groups/{id}`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Group name"
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Group name"
}
```

### `DELETE /api/v1/groups/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "Group name"
}
```

## Policies

### `GET /api/v1/policies`

- Permission: `admin.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "active",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "Policy name",
    "version": 1,
    "kioskMode": false,
    "restrictions": {}
  }
]
```

### `POST /api/v1/policies`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Policy name",
  "version": 1,
  "kioskMode": false,
  "restrictions": {}
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Policy name",
  "version": 1,
  "kioskMode": false,
  "restrictions": {}
}
```

### `PATCH /api/v1/policies/{id}`

- Permission: `admin.write`
- Request body:

```json
{
  "name": "Policy name",
  "version": 1,
  "kioskMode": false,
  "restrictions": {}
}
```

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "active",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Policy name",
  "version": 1,
  "kioskMode": false,
  "restrictions": {}
}
```

### `DELETE /api/v1/policies/{id}`

- Permission: `admin.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "Policy name",
  "version": 1,
  "kioskMode": false,
  "restrictions": {}
}
```

## Devices

### `GET /api/v1/devices`

- Permission: `devices.read`
- Success response: `200 application/json`
- Body:

```json
[
  {
    "id": "uuid",
    "tenantId": "uuid",
    "status": "pending",
    "updatedAt": "2026-04-23T00:00:00Z",
    "deletedAt": null,
    "name": "Device ID",
    "policyId": "uuid"
  }
]
```

### `POST /api/v1/devices`

- Permission: `devices.write`
- Request body:

```json
{
  "name": "Device ID",
  "secretHash": "hash",
  "policyId": "uuid"
}
```

`policyId` is optional.

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "pending",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Device ID",
  "policyId": "uuid"
}
```

### `PATCH /api/v1/devices/{id}`

- Permission: `devices.write`
- Request body:

```json
{
  "name": "Device ID",
  "secretHash": "hash",
  "policyId": "uuid"
}
```

`policyId` is optional.

- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "pending",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": null,
  "name": "Device ID",
  "policyId": "uuid"
}
```

### `DELETE /api/v1/devices/{id}`

- Permission: `devices.write`
- Response body:

```json
{
  "id": "uuid",
  "tenantId": "uuid",
  "status": "retired",
  "updatedAt": "2026-04-23T00:00:00Z",
  "deletedAt": "2026-04-23T00:00:00Z",
  "name": "Device ID",
  "policyId": "uuid"
}
```

## Common Error Behavior

- `401` unauthorized when the session cookie is missing or invalid
- `403` forbidden when the session lacks the required permission
- `404` not found when the record does not exist
- `400` invalid input for malformed or incomplete request payloads
- `500` internal error for unexpected persistence failures

## Response Types

The response body for each endpoint is the same JSON object shown above for that route.
