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
