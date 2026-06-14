# Playwright Dashboard Workspace

This workspace owns dashboard browser automation for XMDM.

## Layout

- `playwright.config.ts` defines the browser projects and server startup.
- `support/` holds shared helpers for routes, auth, and server wiring.
- `tests/` will hold the Playwright specs.

## Setup

```sh
cd playwright
npm install
npm run install-browser
```

If Chromium is already installed, the browser install step can be skipped.

## Run

By default the workspace boots the local stack and starts the real dashboard server through Playwright.
Each local run begins by dropping the compose volumes and recreating the stack, so the dashboard database and local services start from a clean state.
The booted server disables per-request observability logging for test runs so the startup output stays focused on the actual bootstrap steps and failures.

```sh
cd playwright
npm test
```

To point at an already-running dashboard, set `XMDM_DASHBOARD_URL`.

```sh
XMDM_DASHBOARD_URL=http://127.0.0.1:39092 npm test
```

## Environment

- `XMDM_DASHBOARD_URL` overrides the dashboard base URL.
- `XMDM_DASHBOARD_USERNAME` sets the login username for future specs.
- `XMDM_DASHBOARD_PASSWORD` sets the login password for future specs.
- The default Playwright startup path resets the local compose stack with `docker compose down -v`, runs `infra/bootstrap-local.sh`, and then starts `server/cmd/server` with `XMDM_ADDR=:39092` and `XMDM_DISABLE_REQUEST_LOGS=1`.
