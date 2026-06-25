# Capability Matrix

## Core Capabilities

| Area | Current state |
| --- | --- |
| Admin dashboard | Server-rendered dashboard for overview, users, roles, devices, groups, policies, apps, files, managed files, certificates, commands, audit, and health. |
| Admin auth and RBAC | Admin login/session handling with permissions `admin.read`, `admin.write`, `devices.read`, and `devices.write`. |
| Enrollment | Enrollment token, QR/provisioning payload, device secret issuance, and config snapshot flow. |
| Device config sync | Signed config snapshot fetch and launcher-side verification before applying state. |
| Telemetry API | Device-authenticated telemetry ingestion for heartbeat, battery, network, location, and app-state records. |
| Managed apps | App/version records, artifact storage, policy assignment, launcher download/install coordination, checksum verification, and launcher self-update support. |
| Managed files | File upload, managed-file policy assignment, device artifact download, optional variable replacement, and checksum headers. |
| Managed certificates | Certificate records, artifact delivery, policy assignment, and launcher certificate install coordination. |
| Kiosk policy | Launcher-side kiosk mode control and kiosk exit support. |
| Package rules | Launcher-side package rules controller. |
| Device commands | Dashboard command creation, device polling, MQTT push transport, command execution, and acknowledgements. |
| MQTT push | MQTT publisher and Mosquitto dynamic security provisioning. |
| Device logs | Launcher log storage/upload and server-side log ingestion/query. |
| Device info | Launcher device-info reporting and server-side storage/export surfaces. |
| Audit | Audit persistence for implemented admin mutations and dashboard/API inspection. |
| Rate limiting | HTTP rate limits for admin login, admin command creation, and enrollment. |
| Observability | Request logging, trace response headers, health page, and documented operator signals. |
| Backup and restore | Infra backup/restore drill script and runbook. |
| Release artifacts | Release workflow for server tarball, server image, signed launcher APK, checksums, and manifest. |
| Plugin boundary | Core plugin registration for route specs, device actions, command types, permissions, migrations, and root mounts. |

## Premium Boundary

| Capability | Current state |
| --- | --- |
| Remote control | Premium-only. The core dashboard can expose plugin-owned actions, but the implementation is outside this repository. |

## Support Boundaries

| Boundary | Current state |
| --- | --- |
| Endpoint platform | XMDM manages Android launcher devices in this repository. |
| Deployment model | XMDM is single-tenant self-hosted at runtime. |
| Device controls | Current controls center on launcher kiosk behavior, package rules, managed content, telemetry API records, logs, device info, commands, and audit. |
| Android coverage | Device-owner provisioning and launcher behavior depend on Android/OEM behavior. See [Support Boundaries](support-boundaries.md). |
