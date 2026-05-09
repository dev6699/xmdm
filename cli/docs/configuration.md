# Configuration

The CLI resolves configuration in this order:

1. flags
2. environment variables
3. config file
4. defaults

## Config Path

- Default: `~/.config/xmdm/config.yaml`
- Override with `--config`
- Override with `XMDM_CONFIG`

## Session Path

- Default: `~/.config/xmdm/session.json`
- Override with `XMDM_SESSION_FILE`

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `XMDM_CONFIG` | Config file path |
| `XMDM_PROFILE` | Named profile to use |
| `XMDM_BASE_URL` | Explicit server base URL |
| `XMDM_AUTH_MODE` | Authentication mode placeholder |
| `XMDM_OUTPUT_FORMAT` | Default output format |
| `XMDM_TIMEOUT` | Request timeout duration |
| `XMDM_SESSION_FILE` | Session file path |

## Example Config

```yaml
defaultProfile: local
defaultFormat: table
defaultTimeout: 30s

profiles:
  local:
    baseUrl: "http://127.0.0.1:8080/api/v1"
    authMode: session
    outputFormat: table
    timeout: 30s
  lab:
    baseUrl: "https://mdm.example/api/v1"
    authMode: session
    outputFormat: json
    timeout: 15s
```

## Resolved Session File

`auth login` saves a JSON session record. The shape is:

```json
{
  "baseUrl": "http://127.0.0.1:8080/api/v1",
  "profile": "local",
  "username": "admin",
  "cookieName": "xmdm_session",
  "cookieValue": "session-cookie-value",
  "expiresAt": "2026-05-09T10:00:00Z",
  "savedAt": "2026-05-09T09:00:00Z"
}
```

## `xmdm config show`

Inputs:
- resolved config state from flags, environment, and config file

Example Output:
- human mode: key/value tree with `configPath`, `profile`, `baseUrl`, `authMode`, `outputFormat`, and `timeout`
- JSON mode: envelope with `data` containing the same resolved fields
- `stderr`: empty on success
- exit code: `0`

Example Input:

```sh
xmdm config show --config config.yaml
```

Example Output:

```json
{
  "ok": true,
  "command": "xmdm config show",
  "data": {
    "configPath": "/home/me/.config/xmdm/config.yaml",
    "profile": "local",
    "baseUrl": "http://127.0.0.1:8080/api/v1",
    "authMode": "session",
    "outputFormat": "table",
    "timeout": "30s"
  },
  "meta": {
    "requestId": "uuid",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

## `xmdm config validate`

Inputs:
- requires a reachable target from `--base-url` or an active profile

Example Output:
- `stdout`: `target reachable: <url> (status: <code>)`
- `stderr`: server or validation errors
- exit codes: `0` on success, `1` for local validation problems, `2-6` for HTTP/transport failures

Example Input:

```sh
xmdm config validate --config config.yaml
```

Example Output:

```text
target reachable: http://127.0.0.1:8080/api/v1 (status: 200)
```
