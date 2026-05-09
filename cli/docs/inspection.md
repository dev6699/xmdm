# Inspection

## `xmdm devices inspect <id>`

Inputs:
- `<id>` device row id
- `--limit` optional, default `5`
- `--device-secret` optional

Runtime effect:
- loads device inventory
- aggregates recent logs, device-info rows, commands, and audit events
- optionally tries `/api/v1/devices/{name}/config` if `--device-secret` is supplied, using the device name from the inspected row

Output:
- human mode: a device tree with `device`, `deviceInfo`, `commands`, `logs`, and `audit`
- empty sections print as `[]`
- JSON mode: envelope with `data` holding the same sections plus optional `config` / `configError`
- `stderr`: not found, auth, permission, or transport errors
- exit code: `0` on success

Example Input:

```sh
xmdm devices inspect 354251c9-48f8-419d-beef-c526a6f76239 --limit 5
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm devices inspect",
  "data": {
    "device": {
      "id": "354251c9-48f8-419d-beef-c526a6f76239",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "active",
      "name": "cli-device-001"
    },
    "deviceInfo": [
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
    "commands": [
      {
        "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
        "deviceId": "354251c9-48f8-419d-beef-c526a6f76239",
        "type": "reboot",
        "status": "queued"
      }
    ],
    "logs": [
      {
        "id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
        "tenantId": "00000000-0000-0000-0000-000000000000",
        "message": "device reboot requested",
        "level": "info",
        "deviceId": "354251c9-48f8-419d-beef-c526a6f76239"
      }
    ],
    "audit": [
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
    ]
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```
