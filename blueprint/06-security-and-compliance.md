# Security And Compliance

## Security Principles

- TLS everywhere.
- Separate admin and device credentials.
- Least privilege for every role.
- Signed or checksum-verified content.
- Full auditability for admin actions and command dispatch.

## Threat Model

### Assumptions

- The control plane is reachable from untrusted networks.
- The admin console is accessed by authenticated operators, but operator accounts can still be phished or misused.
- A managed device may be stolen, rooted, offline, or partially compromised.
- MQTT, object storage, and database infrastructure are trusted only when authenticated and authorized by the server.
- Optional plugins are part of the trusted computing base only after explicit enablement and review.

### Trust Boundaries

- Browser session to admin console.
- Public enrollment and device sync endpoints.
- Device credential boundary after enrollment.
- Plugin execution boundary inside the Go server.
- Object storage access boundary for downloadable artifacts.
- MQTT broker boundary for command delivery and polling fallback.

### Primary Attack Paths

- Unauthorized admin login or session hijack leading to policy or command abuse.
- Enrollment replay or token theft leading to rogue device binding.
- Device impersonation using stolen or reused secrets.
- Artifact tampering or checksum bypass causing malicious installs.
- Command forgery or stale command replay against a device.
- Plugin overreach into data or actions outside its intended scope.
- Stale config application after policy changes or device reconnect.

### Risk Priorities

- Authentication and session compromise on the admin side.
- Enrollment replay and device impersonation on the device side.
- Artifact integrity and download authorization.
- Command authenticity, acknowledgment, and expiry handling.
- Plugin privilege boundaries and isolation.

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
- Store kiosk exit passcodes in the policy record for admin visibility, and derive the hash only when building the signed policy snapshot for the device.
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
