# XMDM

XMDM is a self-hosted device management system for Android fleets.

It gives operators one place to:

- enroll devices
- publish the launcher app
- push apps, files, and certificates
- apply kiosk and policy rules
- review device health, logs, and audits
- send commands from the dashboard

Core features:

- Android device enrollment
- launcher app deployment
- policy-based kiosk control
- managed apps, files, and certificates
- device status, logs, and audit visibility
- dashboard operations

Premium features:

These features are not included in the core version.

- Remote control for supported devices, see [docs/admin-dashboard.md#premium-remote-control](docs/admin-dashboard.md#premium-remote-control)

## Built For Operators

XMDM keeps the day-to-day workflow simple:

1. Bring up the server.
2. Publish the launcher APK.
3. Enroll a device.
4. Manage the device from the dashboard.

For the full walkthrough, see [docs/admin-dashboard.md](docs/admin-dashboard.md).

## Dashboard Preview

![Dashboard overview](docs/assets/admin-dashboard-overview.png)

The overview shows the current fleet state, alerts, and the most common actions at a glance.

## Get Started

If you want to understand the project, start here:

1. [Blueprint index](blueprint/00-product-principles.md)
2. [Roadmap checklist](blueprint/09-roadmap-checklist.md)
3. [Project status snapshot](PROJECT_STATUS.md)
4. [Release artifacts and deployment](docs/release-artifacts-and-deployment.md)

## Local Setup

```sh
cd infra
docker compose -f docker-compose.yml -f docker-compose.server.yml up -d --build
```

To stop it:

```sh
cd infra
docker compose -f docker-compose.yml -f docker-compose.server.yml down
```

## Project Layout

- `app/`: Android launcher and agent
- `server/`: backend services and admin console
- `infra/`: deployment and local environment files
- `docs/`: guides, runbooks, and release notes
- `blueprint/`: product and architecture decisions
- `playwright/`: dashboard end-to-end coverage

## Release Flow

Release artifacts are built through GitHub Actions and published as GitHub Release assets.

- The Android launcher APK is published separately and then uploaded into the server’s managed app catalog
- Production signing keys are stored as GitHub repository secrets
- Server release details live in [docs/release-artifacts-and-deployment.md](docs/release-artifacts-and-deployment.md)

## Roadmap

The current implementation snapshot lives in [PROJECT_STATUS.md](PROJECT_STATUS.md).
