# Auth Commands

`auth` manages the admin session cookie used by the CLI.

## `xmdm auth login`

Inputs:
- `--username` required
- `--password` required
- resolved target from config/profile/base-url

Runtime effect:
- sends `POST /api/v1/admin/login` as `application/x-www-form-urlencoded`
- stores the returned session cookie in `~/.config/xmdm/session.json` or `XMDM_SESSION_FILE`

Example Output:
- `stdout`: `logged in as <username>`
- `stderr`: auth or transport errors
- exit code: `0` on success, `2` for unauthorized, `6` for transport/server failures

Example Input:

```sh
xmdm auth login --username admin --password admin
```

Example Output:

```text
logged in as admin
```

## `xmdm auth whoami`

Inputs:
- active session file

Runtime effect:
- sends `GET /api/v1/admin/me`

Example Output:
- `stdout`:
  - `session user: <username>`
  - `base URL: <base-url>`
- `stderr`: session or transport errors
- exit code: `0` on success

Example Input:

```sh
xmdm auth whoami
```

Example Output:

```text
session user: admin
base URL: http://127.0.0.1:8080/api/v1
```

## `xmdm auth logout`

Inputs:
- active session file

Runtime effect:
- sends `POST /api/v1/admin/logout`
- deletes the local session file

Example Output:
- `stdout`: `logged out`
- `stderr`: session or transport errors
- exit code: `0` on success

Example Input:

```sh
xmdm auth logout
```

Example Output:

```text
logged out
```
