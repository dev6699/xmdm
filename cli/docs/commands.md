# Commands

## `commands list`

Inputs:
- `--device-id` optional
- `--type` optional
- `--status` optional
- `--limit` optional, default `25`

Runtime effect:
- `GET /api/v1/admin/commands`

Output:
- table in human mode
- `data.items`, `data.count`, and `data.cursor` in JSON mode

Example Input:

```sh
xmdm --format json commands list --device-id 354251c9-48f8-419d-beef-c526a6f76239 --limit 1
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm commands list",
  "data": {
    "items": [
      {
        "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
        "deviceId": "354251c9-48f8-419d-beef-c526a6f76239",
        "type": "reboot",
        "status": "queued"
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

## `commands show <id>`

Inputs:
- `<id>` command id
- same optional filters as `list`

Runtime effect:
- fetches the filtered command collection and selects the item by `id`

Output:
- single command record under `data.item`

Example Input:

```sh
xmdm --format json commands show cccccccc-cccc-cccc-cccc-cccccccccccc
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm commands show",
  "data": {
    "item": {
      "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
      "deviceId": "354251c9-48f8-419d-beef-c526a6f76239",
      "type": "reboot",
      "status": "queued"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `commands send`

Inputs:
- `--json` inline command body, or
- `--file` command body file

Runtime effect:
- `POST /api/v1/admin/commands`

Common body shape:

```json
{
  "type": "reboot",
  "payload": {
    "force": true
  },
  "target": {
    "type": "device",
    "deviceId": "device-123"
  }
}
```

Output:
- queued command record under `data.item`

Example Input:

```sh
xmdm --format json commands send --json '{"type":"reboot","payload":{"force":true},"target":{"type":"device","deviceId":"device-123"}}'
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm commands send",
  "data": {
    "item": {
      "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
      "type": "reboot",
      "status": "queued",
      "target": {
        "type": "device",
        "deviceId": "device-123"
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

## `commands ack <device-id> <command-id>`

Inputs:
- `<device-id>` required
- `<command-id>` required
- `--device-secret` required
- `--status` optional, default `acked`
- `--message` optional
- `--details` optional JSON object

Runtime effect:
- `POST /api/v1/devices/{device-id}/commands/{command-id}/ack`

Output:
- acknowledgement response under `data.item`

Example Input:

```sh
xmdm commands ack device-123 command-456 \
  --device-secret secret \
  --status acked \
  --message done \
  --details '{"transport":"polling"}'
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm commands ack",
  "data": {
    "item": {
      "id": "cccccccc-cccc-cccc-cccc-cccccccccccc",
      "status": "acked",
      "message": "done"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```
