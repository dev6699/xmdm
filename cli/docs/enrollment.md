# Enrollment

## `enrollment tokens issue`

Inputs:
- `--ttl` optional, default `24h`
- requires an admin session

Runtime effect:
- `POST /api/v1/enrollment/tokens`

Example Output:
- token record under `data.item`

Example Input:

```sh
xmdm --format json enrollment tokens issue --ttl 2h
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm enrollment tokens issue",
  "data": {
    "item": {
      "id": "tttttttt-tttt-tttt-tttt-tttttttttttt",
      "token": "enrollment-token-value",
      "expiresAt": "2026-05-09T11:00:00Z"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `enrollment tokens validate <token>`

Inputs:
- `<token>` enrollment token string

Runtime effect:
- `POST /api/v1/enrollment/tokens/validate`

Example Output:
- token record under `data.item`

Example Output:

```json
{
  "ok": true,
  "command": "xmdm enrollment tokens validate",
  "data": {
    "item": {
      "valid": true,
      "token": "enrollment-token-value"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `enrollment tokens consume <token>`

Inputs:
- `<token>` enrollment token string

Runtime effect:
- `POST /api/v1/enrollment/tokens/consume`

Example Output:
- token record under `data.item`

Example Output:

```json
{
  "ok": true,
  "command": "xmdm enrollment tokens consume",
  "data": {
    "item": {
      "consumed": true,
      "token": "enrollment-token-value"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `enrollment tokens revoke <id>`

Inputs:
- `<id>` token id
- requires an admin session

Runtime effect:
- `DELETE /api/v1/enrollment/tokens/{id}`

Example Output:
- token record under `data.item`

Example Output:

```json
{
  "ok": true,
  "command": "xmdm enrollment tokens revoke",
  "data": {
    "item": {
      "id": "tttttttt-tttt-tttt-tttt-tttttttttttt",
      "status": "revoked"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `enrollment qr json`

Inputs:
- `--server-url` optional, defaults to the resolved base URL
- `--server-project` optional
- `--enrollment-token` optional
- `--component-name` optional
- `--package-url` required
- `--package-checksum` required
- `--device-id` optional
- `--device-id-use` optional, default `serial`
- `--bootstrap-extras` optional JSON object
- `--leave-all-system-apps-enabled` optional boolean
- `--skip-encryption` optional boolean
- `--use-mobile-data` optional boolean

Runtime effect:
- `POST /api/v1/enrollment/qr/json`

Example Output:
- Android provisioning JSON under `data.item`

Example Output:

```json
{
  "ok": true,
  "command": "xmdm enrollment qr json",
  "data": {
    "item": {
      "android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME": "com.example/.AdminReceiver",
      "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION": "https://cdn.example/launcher.apk",
      "android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM": "abc123"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `enrollment qr png`

Inputs:
- same QR inputs as `qr json`
- `--output` optional file path

Runtime effect:
- `POST /api/v1/enrollment/qr`

Example Output:
- binary PNG to stdout, or to `--output`
- no JSON envelope in PNG mode

Example Input:

```sh
xmdm enrollment qr png --package-url https://cdn.example/launcher.apk --package-checksum abc123 --output enrollment.png
```

Example Inputs:

```sh
xmdm enrollment tokens issue --ttl 2h
xmdm enrollment qr json --package-url https://cdn.example/launcher.apk --package-checksum abc123
xmdm enrollment qr png --package-url https://cdn.example/launcher.apk --package-checksum abc123 --output enrollment.png
```
