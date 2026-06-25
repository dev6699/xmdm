# Support Boundaries

## Deployment Shape

XMDM targets a single-tenant, self-hosted deployment.

The local supported runtime is Docker Compose with:

- Go server
- PostgreSQL
- S3-compatible object storage
- Mosquitto MQTT broker with dynamic security

## Server Configuration

The server reads YAML config and environment overrides for:

- listen address and public URL
- session TTL
- PostgreSQL DSN
- MQTT server and dynamic-security credentials
- device command/config sync intervals
- object storage endpoint, region, bucket, and credentials
- seed admin username and password

## Android Device Boundary

The endpoint app in this repository is the Android launcher. It handles
device-owner integration, enrollment, config sync, managed app/file/certificate
coordination, kiosk and package-rule control, commands, logs, and device-info
reporting.

## Device Provisioning Boundary

The repository implements QR/provisioning payload and device secret issuance,
but actual device-owner provisioning still depends on Android provisioning
rules, OS version behavior, and OEM/device restrictions.

Android compatibility statements require current device evidence.

## Device Control Boundary

Implemented device controls are limited to the surfaces in
[Capability Matrix](capabilities.md):

- kiosk behavior
- package rules
- app, file, and certificate delivery
- device logs, telemetry API records, device info, commands, and audit

Remote-control behavior belongs to optional premium extensions.

## Release Boundary

The release workflow builds and publishes server artifacts, a server image, a
signed Android launcher APK, checksums, and a release manifest.

The operator must still publish the launcher APK into the managed app catalog.
