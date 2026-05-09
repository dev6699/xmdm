# Resources

This page covers the read-only resource groups:

- `users`
- `roles`
- `groups`
- `policies`
- `apps`
- `files`
- `managed-files`
- `certificates`
- `devices`
- `logs`
- `device-info`
- `audit`

All resource groups support `list` and `show`.

## Common Behavior

### `list`

Inputs:
- no positional arguments
- runtime target from config/profile/base-url
- active admin session for all resources except `config` and `auth`

Runtime effect:
- sends `GET` to the matching collection endpoint

Output:
- human mode: fixed-width table with columns `id`, `name`, `status`, `type`, `deviceId`
- JSON mode: envelope with `data.items`, `data.count`, and `data.cursor`
- `stderr`: validation, auth, permission, or transport failures
- exit codes: `2` unauthorized, `3` forbidden, `4` not found, `5` conflict, `6` transport/server failures

### `show`

Inputs:
- `<id>` object id
- runtime target and active admin session

Runtime effect:
- sends `GET` to the matching collection endpoint and selects the matching item by `id`

Output:
- human mode: key/value tree for the item
- JSON mode: envelope with `data.item`
- `stderr`: not found, auth, permission, or transport errors
- exit codes: `4` for not found, `2` for unauthorized, `3` for forbidden, `5` for conflict, `6` for transport/server failures

## Per-Resource Examples

### `users`

Example Input:
```sh
xmdm --format json users list
xmdm --format json users show 33333333-3333-3333-3333-333333333333
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm users list",
  "data": {
    "items": [
      {
        "id": "33333333-3333-3333-3333-333333333333",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "email": "alice@example.com",
        "roleId": "22222222-2222-2222-2222-222222222222"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm users show",
  "data": {
    "item": {
      "id": "33333333-3333-3333-3333-333333333333",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "email": "alice@example.com",
      "roleId": "22222222-2222-2222-2222-222222222222"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `roles`

Example Input:
```sh
xmdm --format json roles list
xmdm --format json roles show 22222222-2222-2222-2222-222222222222
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm roles list",
  "data": {
    "items": [
      {
        "id": "22222222-2222-2222-2222-222222222222",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "Operators",
        "permissions": ["admin.read", "devices.read"]
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm roles show",
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

### `groups`

Example Input:
```sh
xmdm --format json groups list
xmdm --format json groups show 44444444-4444-4444-4444-444444444444
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm groups list",
  "data": {
    "items": [
      {
        "id": "44444444-4444-4444-4444-444444444444",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "Field Devices"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm groups show",
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

### `policies`

Example Input:
```sh
xmdm --format json policies list
xmdm --format json policies show 55555555-5555-5555-5555-555555555555
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm policies list",
  "data": {
    "items": [
      {
        "id": "55555555-5555-5555-5555-555555555555",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "Baseline",
        "version": 1,
        "kioskMode": false
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm policies show",
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

### `apps`

Example Input:
```sh
xmdm --format json apps list
xmdm --format json apps show 66666666-6666-6666-6666-666666666666
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm apps list",
  "data": {
    "items": [
      {
        "id": "66666666-6666-6666-6666-666666666666",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "Launcher"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm apps show",
  "data": {
    "item": {
      "id": "66666666-6666-6666-6666-666666666666",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "Launcher",
      "versions": [
        {
          "versionName": "1.0.0",
          "versionCode": 100
        }
      ]
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `files`

Example Input:
```sh
xmdm --format json files list
xmdm --format json files show 77777777-7777-7777-7777-777777777777
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm files list",
  "data": {
    "items": [
      {
        "id": "77777777-7777-7777-7777-777777777777",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "launcher.apk",
        "storageKey": "artifacts/launcher.apk"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm files show",
  "data": {
    "item": {
      "id": "77777777-7777-7777-7777-777777777777",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "launcher.apk",
      "storageKey": "artifacts/launcher.apk"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `managed-files`

Example Input:
```sh
xmdm --format json managed-files list
xmdm --format json managed-files show 88888888-8888-8888-8888-888888888888
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm managed-files list",
  "data": {
    "items": [
      {
        "id": "88888888-8888-8888-8888-888888888888",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "path": "/system/app/Launcher.apk",
        "fileId": "77777777-7777-7777-7777-777777777777",
        "replaceVariables": true
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm managed-files show",
  "data": {
    "item": {
      "id": "88888888-8888-8888-8888-888888888888",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "path": "/system/app/Launcher.apk",
      "fileId": "77777777-7777-7777-7777-777777777777",
      "replaceVariables": true
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `certificates`

Example Input:
```sh
xmdm --format json certificates list
xmdm --format json certificates show 99999999-9999-9999-9999-999999999999
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm certificates list",
  "data": {
    "items": [
      {
        "id": "99999999-9999-9999-9999-999999999999",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "MDM Root",
        "storageKey": "certs/mdm-root.pem"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm certificates show",
  "data": {
    "item": {
      "id": "99999999-9999-9999-9999-999999999999",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "MDM Root",
      "storageKey": "certs/mdm-root.pem"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `devices`

Example Input:
```sh
xmdm --format json devices list
xmdm --format json devices show 354251c9-48f8-419d-beef-c526a6f76239
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm devices list",
  "data": {
    "items": [
      {
        "id": "354251c9-48f8-419d-beef-c526a6f76239",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "status": "active",
        "name": "cli-device-001"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm devices show",
  "data": {
    "item": {
      "id": "354251c9-48f8-419d-beef-c526a6f76239",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "cli-device-001",
      "updatedAt": "2026-05-09T09:00:00.123456789Z"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `logs`

Example Input:
```sh
xmdm --format json logs list
xmdm --format json logs show aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm logs list",
  "data": {
    "items": [
      {
        "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "message": "device reboot requested",
        "level": "info",
        "deviceId": "354251c9-48f8-419d-beef-c526a6f76239"
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm logs show",
  "data": {
    "item": {
      "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "message": "device reboot requested",
      "level": "info",
      "deviceId": "354251c9-48f8-419d-beef-c526a6f76239"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `device-info`

Example Input:
```sh
xmdm --format json device-info list
xmdm --format json device-info show bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm device-info list",
  "data": {
    "items": [
      {
        "id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "deviceId": "354251c9-48f8-419d-beef-c526a6f76239",
        "payload": {
          "model": "Pixel 8",
          "serial": "ABC123"
        }
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm device-info show",
  "data": {
    "item": {
      "id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "deviceId": "354251c9-48f8-419d-beef-c526a6f76239",
      "payload": {
        "model": "Pixel 8",
        "serial": "ABC123"
      }
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

### `audit`

Example Input:
```sh
xmdm --format json audit list
xmdm --format json audit show cccccccc-cccc-cccc-cccc-cccccccccccc
```

Example Output:
```json
{
  "ok": true,
  "command": "xmdm audit list",
  "data": {
    "items": [
      {
        "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "actor": "admin",
        "action": "create",
        "resourceType": "devices",
        "resourceId": "354251c9-48f8-419d-beef-c526a6f76239",
        "details": {
          "name": "cli-device-001"
        }
      }
    ],
    "count": 1,
    "cursor": null
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
  "command": "xmdm audit show",
  "data": {
    "item": {
      "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "actor": "admin",
      "action": "create",
      "resourceType": "devices",
      "resourceId": "354251c9-48f8-419d-beef-c526a6f76239",
      "details": {
        "name": "cli-device-001"
      }
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```
