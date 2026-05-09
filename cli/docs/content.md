# Content

## `files create`

Inputs:
- `--name` optional
- `--storage-key` required
- `--source` required
- `--mime-type` required
- `--checksum` optional, defaults to the SHA-256 base64url digest of the source file
- `--size-bytes` optional, defaults to the source file size

Runtime effect:
- uploads a multipart artifact and creates a logical file record

Example Output:
- JSON mode: `data.item` with file and artifact metadata
- human mode: item tree

Example Input:

```sh
xmdm --format json files create \
  --name launcher.apk \
  --storage-key artifacts/launcher.apk \
  --source ./launcher.apk \
  --mime-type application/vnd.android.package-archive
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm files create",
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

## `files retire <id>`

Inputs:
- `<id>` file id

Example Output:
- retired file record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm files retire",
  "data": {
    "item": {
      "id": "77777777-7777-7777-7777-777777777777",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "launcher.apk"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `certificates create`

Inputs:
- same upload flags as `files create`

Runtime effect:
- uploads a certificate artifact and creates a certificate record

Example Output:
- certificate record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm certificates create",
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

## `certificates retire <id>`

Inputs:
- `<id>` certificate id

Example Output:
- retired certificate record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm certificates retire",
  "data": {
    "item": {
      "id": "99999999-9999-9999-9999-999999999999",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "name": "MDM Root"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `managed-files create`

Inputs:
- `--file-id` required
- `--path` required
- `--replace-variables` optional, default `true`

Runtime effect:
- creates a managed-file record that points at an existing file

Example Output:
- managed-file record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm managed-files create",
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

## `managed-files retire <id>`

Inputs:
- `<id>` managed-file id

Example Output:
- retired managed-file record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm managed-files retire",
  "data": {
    "item": {
      "id": "88888888-8888-8888-8888-888888888888",
      "tenantId": "00000000-0000-0000-0000-000000000000",
      "status": "retired",
      "path": "/system/app/Launcher.apk"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `apps versions publish <app-id>`

Inputs:
- `<app-id>` app id
- `--version-name` required
- `--version-code` required
- `--artifact-id` required
- `--checksum` required

Runtime effect:
- publishes an app version and links it to an existing artifact

Example Output:
- published app version record

Example Output:

```json
{
  "ok": true,
  "command": "xmdm apps versions publish",
  "data": {
    "item": {
      "id": "66666666-6666-6666-6666-666666666666",
      "versionName": "1.0.0",
      "versionCode": 100,
      "artifactId": "77777777-7777-7777-7777-777777777777",
      "checksum": "checksum"
    }
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

Example Input:

```sh
xmdm apps versions publish <app-id> \
  --version-name 1.0.0 \
  --version-code 100 \
  --artifact-id <artifact-id> \
  --checksum <checksum>
```
