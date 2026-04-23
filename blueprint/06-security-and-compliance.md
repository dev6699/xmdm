# Security And Compliance

## Security Principles

- TLS everywhere.
- Separate admin and device credentials.
- Least privilege for every role.
- Signed or checksum-verified content.
- Full auditability for admin actions and command dispatch.

## Authentication Model

### Admin

- Password-based login.
- Session cookie for the browser.
- JWT for API clients and automation.

### Device

- One-time enrollment token.
- Per-device secret after enrollment.
- Device credentials are not interchangeable across devices.

### Plugin

- Plugin settings are access-controlled.
- Sensitive plugin operations respect the calling identity.

## Authorization Model

- Role-based access control for admin users.
- Fine-grained permissions for device commands, push, audit, files, and plugins.
- Group and tenant scoping for all admin queries.
- Device identity is always resolved before allowing a device-side mutation.

## Content Integrity

- Artifact checksums are mandatory.
- Device sync responses are signed.
- Enrollment payloads are authenticated.
- Download URLs must not be trusted without server-side authorization.

## Secrets Handling

- Store admin passwords as salted hashes.
- Store device secrets hashed or encrypted at rest.
- Keep signing keys and MQTT credentials in environment-managed secrets.
- Never log secrets, tokens, or raw enrollment payloads.
- Rotate signing and broker credentials on a documented schedule.

## Compliance Features

- Immutable audit log.
- Admin-visible history for critical changes.
- Device command history.
- Retention policies for logs and push messages.
- Exportable records for device and admin activity.

## Risk Areas

- Unauthorized command execution
- Overbroad plugin access
- Artifact tampering
- Enrollment replay
- Device impersonation
- Stale sync snapshots after policy changes
- Privilege creep in admin workflows

## Required Controls

- Request signing on enrollment and sync
- Response signing on sync payloads
- CSRF protection for browser forms
- Per-route permission checks
- Audit logging for all privileged mutations
- Token expiry for enrollment and command links
- Server-side validation of device ownership before command delivery

## Failure Mode Rules

- If auth fails, return a stable auth error and do not leak existence details.
- If a device is unknown, return a device-not-found result without exposing unrelated tenant data.
- If a signature check fails, reject the request before any side effects.
- If a plugin is disabled, deny plugin calls even if the user is otherwise authenticated.
- If an artifact fails verification, the install must stop and be reported.
