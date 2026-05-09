# XMDM CLI Operator Guide

`xmdm` is the operator-facing command-line client for the XMDM control plane.
Use it to authenticate, inspect fleet state, manage core resources, upload content, issue enrollment payloads, and drive device commands.

The CLI assumes a live XMDM server for almost every command. It is designed for both human operators and automation.

## Quick Start

1. Install or build the binary.
2. Copy the example config into `~/.config/xmdm/config.yaml`.
3. Log in with an admin session.
4. Use `list` and `show` commands to inspect resources.
5. Use `--format json` when you need machine-readable output.

Example:

```sh
cd cli
go install ./cmd/xmdm
mkdir -p ~/.config/xmdm
cp config.yaml ~/.config/xmdm/config.yaml
xmdm auth login --username admin --password admin
xmdm devices list
```

## Install

Build from source:

```sh
cd cli
go build -o ./bin/xmdm ./cmd/xmdm
```

Install into your Go bin directory:

```sh
cd cli
go install ./cmd/xmdm
```

The checked-in example config lives at [config.yaml](config.yaml).
Copy it to your user config directory:

```sh
mkdir -p ~/.config/xmdm
cp config.yaml ~/.config/xmdm/config.yaml
```

## Upgrade

Source install:

```sh
cd cli
go install ./cmd/xmdm
```

Release install:

- replace the `xmdm` binary with the newer artifact
- keep the same config file unless the release notes say otherwise
- re-run shell completion setup if your shell stores generated scripts in a custom location

## Shell Completion

Cobra exposes built-in completion generation:

```sh
xmdm completion bash
xmdm completion zsh
xmdm completion fish
xmdm completion powershell
```

Examples:

```sh
xmdm completion bash > ~/.local/share/bash-completion/completions/xmdm
```

```sh
xmdm completion zsh > "${fpath[1]}/_xmdm"
```

```sh
xmdm completion fish > ~/.config/fish/completions/xmdm.fish
```

```powershell
xmdm completion powershell | Out-String | Invoke-Expression
```

## Configuration

The CLI resolves configuration in this order:

1. flags
2. environment variables
3. config file
4. defaults

### Config Path

- Default: `~/.config/xmdm/config.yaml`
- Override with `--config`
- Override with `XMDM_CONFIG`

### Session Path

- Default: `~/.config/xmdm/session.json`
- Override with `XMDM_SESSION_FILE`

### Environment Variables

| Variable | Purpose |
| --- | --- |
| `XMDM_CONFIG` | Config file path |
| `XMDM_PROFILE` | Named profile to use |
| `XMDM_BASE_URL` | Explicit server base URL |
| `XMDM_AUTH_MODE` | Authentication mode placeholder |
| `XMDM_OUTPUT_FORMAT` | Default output format |
| `XMDM_TIMEOUT` | Request timeout duration |
| `XMDM_SESSION_FILE` | Session file path |

### Example Config

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

### Resolved Session File

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

## Output Contract

### Human Output

The default output format is `table`.

- `list` commands print a fixed-width table.
- `show`, `inspect`, and `config show` print a readable key/value tree.
- `auth login`, `auth whoami`, `auth logout`, and `config validate` print plain text.
- `completion` prints shell scripts.

### JSON Output

Use `--format json` for machine consumption.

Successful commands emit a JSON envelope:

```json
{
  "ok": true,
  "command": "xmdm users list",
  "data": {
    "items": [],
    "count": 0,
    "cursor": null
  },
  "meta": {
    "requestId": "b7d2d6f0a8a94f8f8c7d4ab8f33db7e2",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

Single-object commands use `data.item`:

```json
{
  "ok": true,
  "command": "xmdm users create",
  "data": {
    "item": {
      "id": "uuid",
      "tenantId": "uuid",
      "status": "active"
    }
  },
  "meta": {
    "requestId": "b7d2d6f0a8a94f8f8c7d4ab8f33db7e2",
    "timestamp": "2026-05-09T09:00:00.123456789Z",
    "baseUrl": "http://127.0.0.1:8080/api/v1"
  }
}
```

`devices inspect` returns a richer `data` object:

```json
{
  "device": {...},
  "deviceInfo": [...],
  "commands": [...],
  "logs": [...],
  "audit": [...],
  "config": {...},
  "configError": "optional error string"
}
```

That object is returned under `data` in the live JSON envelope.

### Exit Codes

| Exit code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Validation or local CLI error |
| `2` | Unauthorized (`401`) |
| `3` | Forbidden (`403`) |
| `4` | Not found (`404`) |
| `5` | Conflict (`409`) |
| `6` | Transport failure or server error (`5xx`) |

Errors are written to `stderr`.

## Action Pages

- [Auth](docs/auth.md)
- [Configuration](docs/configuration.md)
- [Resources](docs/resources.md)
- [Inspection](docs/inspection.md)
- [Management](docs/management.md)
- [Content](docs/content.md)
- [Enrollment](docs/enrollment.md)
- [Commands](docs/commands.md)

## Discoverability

Use help at any level:

```sh
xmdm --help
xmdm devices --help
xmdm devices inspect --help
```

## Related Docs

- [Admin Operator Story](../docs/admin-operator-story.md)
- [Admin Console Contract](../contracts/admin-console.md)
- [Enrollment Contract](../contracts/enrollment.md)
