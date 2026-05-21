# Admin Operator Story

This story describes the end-to-end admin workflow for XMDM.

The browser dashboard lives in [Admin Dashboard](admin-dashboard.md). The command-level CLI reference lives in [cli/docs](../cli/docs). Use the story here to understand the operator lifecycle, then use the dashboard or CLI pages for exact execution paths.

## Operator Goal

An XMDM admin should be able to:

1. Log in and confirm the session.
2. Establish access control through users and roles.
3. Prepare policies, groups, apps, files, and certificates.
4. Enroll devices and verify they begin syncing.
5. Push configuration and content changes.
6. Issue commands and observe acknowledgements.
7. Inspect logs, device info, and audit history.
8. Retire or update anything that is no longer valid.

## Canonical CLI Flow

The operator-facing CLI is `xmdm`.

Typical flow:

```sh
xmdm --config ~/.config/xmdm/config.yaml auth login --username admin --password admin
xmdm --config ~/.config/xmdm/config.yaml config validate
xmdm --config ~/.config/xmdm/config.yaml users list
xmdm --config ~/.config/xmdm/config.yaml roles list
xmdm --config ~/.config/xmdm/config.yaml groups list
xmdm --config ~/.config/xmdm/config.yaml policies list
xmdm --config ~/.config/xmdm/config.yaml apps list
xmdm --config ~/.config/xmdm/config.yaml files list
xmdm --config ~/.config/xmdm/config.yaml managed-files list
xmdm --config ~/.config/xmdm/config.yaml certificates list
xmdm --config ~/.config/xmdm/config.yaml devices list
xmdm --config ~/.config/xmdm/config.yaml devices inspect <device-id> --limit 5
xmdm --config ~/.config/xmdm/config.yaml commands list --device-id <device-id>
xmdm --config ~/.config/xmdm/config.yaml commands send --json '{"type":"reboot","target":{"type":"device","deviceId":"<device-id>"}}'
xmdm --config ~/.config/xmdm/config.yaml auth logout
```

## Lifecycle Story

### 1. Enter the control plane

The admin authenticates with a session cookie, confirms the current user, and logs out when done.

CLI references:
- [Auth](../cli/docs/auth.md)
- [Configuration](../cli/docs/configuration.md)

Relevant commands:

```sh
xmdm --config ~/.config/xmdm/config.yaml auth login --username admin --password admin
xmdm --config ~/.config/xmdm/config.yaml auth whoami
xmdm --config ~/.config/xmdm/config.yaml auth logout
```

### 2. Establish access control

The admin defines users and roles before managing devices at scale.

CLI reference:
- [Management](../cli/docs/management.md)

Relevant commands:

```sh
xmdm --format json users create --json '{"email":"user@example.com","passwordHash":"hash","roleId":"uuid"}'
xmdm --format json roles create --json '{"name":"Operators","permissions":["admin.read","devices.read"]}'
xmdm --format json users update <user-id> --json '{"email":"alice@example.com","passwordHash":"hash","roleId":"uuid"}'
xmdm --format json roles retire <role-id>
```

### 3. Prepare fleet structure

The admin prepares groups and policies before enrollment or rollout.

CLI reference:
- [Management](../cli/docs/management.md)
- [Resources](../cli/docs/resources.md)

Relevant commands:

```sh
xmdm --format json groups create --json '{"name":"Field Devices"}'
xmdm --format json policies create --json '{"name":"Baseline","version":1,"kioskMode":false,"restrictions":null}'
xmdm --format json groups list
xmdm --format json policies list
```

### 4. Prepare content and certificates

The admin uploads files, managed files, app versions, and certificates.

CLI reference:
- [Content](../cli/docs/content.md)

Relevant commands:

```sh
xmdm --format json files create --name launcher.apk --storage-key artifacts/launcher.apk --source ./launcher.apk --mime-type application/vnd.android.package-archive
xmdm --format json managed-files create --file-id <file-id> --path /system/app/Launcher.apk
xmdm --format json apps versions publish <app-id> --version-name 1.0.0 --version-code 100 --artifact-id <artifact-id> --checksum <checksum>
xmdm --format json certificates create --name MDM Root --storage-key certs/mdm-root.pem --source ./mdm-root.pem --mime-type application/x-pem-file
```

For dashboard QR enrollment, publish the XMDM agent APK as the managed app whose package matches `device.agentAppPackage` in the server config. The default package is `com.xmdm.launcher`.

### 5. Enroll a device

Enrollment is the moment a device becomes part of the managed fleet.

CLI reference:
- [Enrollment](../cli/docs/enrollment.md)

Relevant commands:

```sh
xmdm --format json enrollment tokens issue --ttl 2h
xmdm --format json enrollment tokens validate <token>
xmdm --format json enrollment qr json --package-url https://mdm.example/api/v1/enrollment/agent.apk --package-checksum <latest-agent-checksum>
xmdm --format json enrollment qr png --package-url https://mdm.example/api/v1/enrollment/agent.apk --package-checksum <latest-agent-checksum> --output enrollment.png
```

### 6. Verify device health

The admin confirms the device is active, syncing, and reporting telemetry.

CLI reference:
- [Resources](../cli/docs/resources.md)
- [Inspection](../cli/docs/inspection.md)

Relevant commands:

```sh
xmdm --format json devices list
xmdm --format json devices inspect <device-id> --limit 5
xmdm --format json device-info list
xmdm --format json logs list
```

### 7. Push managed behavior

The admin rolls out policy, content, and kiosk changes.

CLI reference:
- [Management](../cli/docs/management.md)
- [Content](../cli/docs/content.md)
- [Resources](../cli/docs/resources.md)

Relevant commands:

```sh
xmdm --format json policies update <policy-id> --json '{"name":"Baseline","version":2,"kioskMode":false,"restrictions":null}'
xmdm --format json managed-files create --file-id <file-id> --path /system/app/Launcher.apk
xmdm --format json certificates retire <certificate-id>
```

### 8. Issue commands

The admin sends device actions and tracks acknowledgements.

CLI reference:
- [Commands](../cli/docs/commands.md)

Relevant commands:

```sh
xmdm --format json commands list --device-id <device-id>
xmdm --format json commands send --json '{"type":"reboot","target":{"type":"device","deviceId":"<device-id>"}}'
xmdm --format json commands show <command-id>
xmdm commands ack <device-id> <command-id> --device-secret <secret> --status acked
```

### 9. Observe and troubleshoot

The admin searches logs, device info, commands, and audit history to answer support questions.

CLI reference:
- [Resources](../cli/docs/resources.md)
- [Inspection](../cli/docs/inspection.md)

Relevant commands:

```sh
xmdm --format json logs list
xmdm --format json device-info list
xmdm --format json audit list
xmdm --format json devices inspect <device-id> --limit 5
```

### 10. Retire safely

At the end of a lifecycle, the admin retires resources without breaking the control plane.

CLI reference:
- [Management](../cli/docs/management.md)

Relevant commands:

```sh
xmdm --format json devices retire <device-id>
xmdm --format json files retire <file-id>
xmdm --format json managed-files retire <managed-file-id>
xmdm --format json certificates retire <certificate-id>
xmdm --format json groups retire <group-id>
xmdm --format json policies retire <policy-id>
xmdm --format json roles retire <role-id>
xmdm --format json users retire <user-id>
```

## Supporting Docs

- [XMDM CLI Operator Guide](../cli/README.md)
- [Admin Dashboard](admin-dashboard.md)
- [Admin Console Contract](../contracts/admin-console.md)
- [Enrollment Contract](../contracts/enrollment.md)
