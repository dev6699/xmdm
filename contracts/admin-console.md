# Admin Console Contract

This document defines the admin console HTTP surface in `server/cmd/server`.

Path prefix:

- The live versioned admin API surface lives under `/api/v1/admin/...`
- The same contract can be mounted under `/admin/...` by the console wrapper
- Both surfaces preserve the same semantics and payload shapes

## Session Routes

### `GET /admin/login`

- Request: none
- Response: `200 text/html`
- Body: login form HTML

### `POST /admin/login`

- Request: `application/x-www-form-urlencoded`
- Fields:
  - `username` `string`
  - `password` `string`
- Success response: `303 See Other`
- Headers:
  - `Set-Cookie: session=<session-id>; HttpOnly; Path=/`
  - `Location: /admin/me`
- Errors:
  - `400` invalid form
  - `401` invalid credentials

### `POST /admin/logout`

- Request: none
- Success response: `204 No Content`
- Headers:
  - `Set-Cookie: session=; Max-Age=-1; HttpOnly; Path=/`

### `GET /admin/me`

- Request:
  - session cookie required
- Success response: `200 application/json`
- Body:
  - `{"user":"<username>"}`
- Errors:
  - `401` unauthorized

## Users

### `GET /admin/users`

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

### `POST /admin/users`

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

### `PATCH /admin/users/{id}`

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

### `DELETE /admin/users/{id}`

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

### `GET /admin/roles`

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

### `POST /admin/roles`

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

### `PATCH /admin/roles/{id}`

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

### `DELETE /admin/roles/{id}`

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

### `GET /admin/groups`

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

### `POST /admin/groups`

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

### `PATCH /admin/groups/{id}`

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

### `DELETE /admin/groups/{id}`

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

### `GET /admin/policies`

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

### `POST /admin/policies`

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

### `PATCH /admin/policies/{id}`

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

### `DELETE /admin/policies/{id}`

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

### `GET /admin/devices`

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

### `POST /admin/devices`

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

### `PATCH /admin/devices/{id}`

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

### `DELETE /admin/devices/{id}`

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
