# Admin Dashboard Screenshot Tooling

Use this tooling to regenerate the screenshots used by `docs/admin-dashboard.md`.

It uses:

- `server/cmd/dashboard-screenshot-fixture` for a local dashboard server with deterministic sample data
- Playwright Chromium for browser screenshots

## One-Time Setup

```sh
cd docs/tools/admin-dashboard-screenshots
npm install
npm run install-browser
```

If Chromium was already installed by Playwright, setup can be skipped.

## Capture Screenshots

Terminal 1:

```sh
cd server
GOCACHE=/tmp/go-build go run ./cmd/dashboard-screenshot-fixture
```

Terminal 2:

```sh
cd docs/tools/admin-dashboard-screenshots
npm run capture
```

The screenshots are written to `docs/assets`.

## Configuration

Optional environment variables:

- `XMDM_DASHBOARD_URL`, default `http://127.0.0.1:39091`
- `XMDM_DASHBOARD_USERNAME`, default `admin`
- `XMDM_DASHBOARD_PASSWORD`, default `admin`
