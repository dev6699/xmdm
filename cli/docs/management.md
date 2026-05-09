# Management

This page covers the write operations for the core admin resources:

- `users`
- `roles`
- `groups`
- `policies`
- `devices`

Each resource supports its own `create`, `update`, and `retire` actions.

## `users`

### Example Input

```sh
xmdm --format json users create --json '{"email":"user@example.com","passwordHash":"hash","roleId":"uuid"}'
xmdm --format json users update 33333333-3333-3333-3333-333333333333 --json '{"email":"alice@example.com","passwordHash":"hash","roleId":"uuid"}'
xmdm --format json users retire 33333333-3333-3333-3333-333333333333
```

### Example Output

```json
{
  "ok": true,
  "command": "xmdm users create",
  "data": {
    "item": {
      "id": "11111111-1111-1111-1111-111111111111",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "email": "user@example.com",
      "roleId": "uuid"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm users update",
  "data": {
    "item": {
      "id": "33333333-3333-3333-3333-333333333333",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "email": "alice@example.com",
      "roleId": "uuid"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm users retire",
  "data": {
    "item": {
      "id": "33333333-3333-3333-3333-333333333333",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "email": "alice@example.com"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `roles`

### Example Input

```sh
xmdm --format json roles create --json '{"name":"Operators","permissions":["admin.read","devices.read"]}'
xmdm --format json roles update 22222222-2222-2222-2222-222222222222 --json '{"name":"Operators","permissions":["admin.read","devices.read","audit.read"]}'
xmdm --format json roles retire 22222222-2222-2222-2222-222222222222
```

### Example Output

```json
{
  "ok": true,
  "command": "xmdm roles create",
  "data": {
    "item": {
      "id": "22222222-2222-2222-2222-222222222222",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Operators",
      "permissions": ["admin.read", "devices.read"]
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm roles update",
  "data": {
    "item": {
      "id": "22222222-2222-2222-2222-222222222222",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Operators",
      "permissions": ["admin.read", "devices.read", "audit.read"]
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm roles retire",
  "data": {
    "item": {
      "id": "22222222-2222-2222-2222-222222222222",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "Operators"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `groups`

### Example Input

```sh
xmdm --format json groups create --json '{"name":"Field Devices"}'
xmdm --format json groups update 44444444-4444-4444-4444-444444444444 --json '{"name":"Field Devices East"}'
xmdm --format json groups retire 44444444-4444-4444-4444-444444444444
```

### Example Output

```json
{
  "ok": true,
  "command": "xmdm groups create",
  "data": {
    "item": {
      "id": "44444444-4444-4444-4444-444444444444",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Field Devices"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm groups update",
  "data": {
    "item": {
      "id": "44444444-4444-4444-4444-444444444444",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Field Devices East"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm groups retire",
  "data": {
    "item": {
      "id": "44444444-4444-4444-4444-444444444444",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "Field Devices East"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `policies`

### Example Input

```sh
xmdm --format json policies create --json '{"name":"Baseline","version":1,"kioskMode":false,"restrictions":null}'
xmdm --format json policies update 55555555-5555-5555-5555-555555555555 --json '{"name":"Baseline","version":2,"kioskMode":false,"restrictions":null}'
xmdm --format json policies retire 55555555-5555-5555-5555-555555555555
```

### Example Output

```json
{
  "ok": true,
  "command": "xmdm policies create",
  "data": {
    "item": {
      "id": "55555555-5555-5555-5555-555555555555",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Baseline",
      "version": 1,
      "kioskMode": false
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm policies update",
  "data": {
    "item": {
      "id": "55555555-5555-5555-5555-555555555555",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Baseline",
      "version": 2,
      "kioskMode": false
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm policies retire",
  "data": {
    "item": {
      "id": "55555555-5555-5555-5555-555555555555",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "Baseline"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `devices`

### Example Input

```sh
xmdm --format json devices create --json '{"name":"device-001","secretHash":"hash"}'
xmdm --format json devices update 354251c9-48f8-419d-beef-c526a6f76239 --json '{"name":"device-001-updated","secretHash":"hash"}'
xmdm --format json devices retire 354251c9-48f8-419d-beef-c526a6f76239
```

### Example Output

```json
{
  "ok": true,
  "command": "xmdm devices create",
  "data": {
    "item": {
      "id": "354251c9-48f8-419d-beef-c526a6f76239",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "device-001"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm devices update",
  "data": {
    "item": {
      "id": "354251c9-48f8-419d-beef-c526a6f76239",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "device-001-updated"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

```json
{
  "ok": true,
  "command": "xmdm devices retire",
  "data": {
    "item": {
      "id": "354251c9-48f8-419d-beef-c526a6f76239",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "device-001-updated"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```
