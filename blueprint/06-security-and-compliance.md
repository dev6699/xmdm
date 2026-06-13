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
- Premium plugin code is reviewed and deployed separately from open-core XMDM, but it still runs inside the XMDM trust boundary once enabled.

### Trust Boundaries

- Browser session to admin console.
- Public enrollment and device sync endpoints.
- Device credential boundary after enrollment.
- Plugin execution boundary inside the Go server.
- Premium service boundary for plugin-owned services outside the core server and outside the open-core repository.
- Object storage access boundary for downloadable artifacts.
- MQTT broker boundary for command delivery and polling fallback.

### Primary Attack Paths

- Unauthorized admin login or session hijack leading to policy or command abuse.
- Enrollment replay or token theft leading to rogue device binding.
- Device impersonation using stolen or reused secrets.
- Artifact tampering or checksum bypass causing malicious installs.
- Command forgery or stale command replay against a device.
- Plugin overreach into data or actions outside its intended scope.
- Plugin-created support sessions with stale or overbroad access.
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
- Admin sessions are tenant-scoped and permission-scoped before any dashboard mutation or API write.

### Device

- One-time enrollment token.
- Per-device secret after enrollment.
- Device credentials are not interchangeable across devices.
- Device identity is provisioned by the server, bound to one tenant, and rejected if the device status is not eligible for authentication.
- A device may authenticate only after enrollment has consumed the bootstrap token and the device secret has been issued.

### Plugin

- Plugin settings are access-controlled.
- Sensitive plugin operations respect the calling identity.
- Premium plugin sessions use short-lived, auditable tokens scoped to one device and one operator action.

## Authorization Model

- Role-based access control for admin users.
- Fine-grained permissions for device commands, push, audit, files, and plugins.
- Group and tenant scoping for all admin queries.
- Device identity is always resolved before allowing a device-side mutation.
- Plugin-provided routes, device actions, and command types must declare required permissions.

## Content Integrity

- Artifact checksums are mandatory.
- Device sync responses are signed.
- Enrollment payloads are authenticated.
- Download URLs must not be trusted without server-side authorization.
- Command acknowledgements are accepted only from the authenticated device that owns the command row.

## Secrets Handling

- Store admin passwords as salted hashes.
- Store device secrets hashed or encrypted at rest.
- Store kiosk exit passcodes in the policy record for admin visibility, and derive the hash only when building the signed policy snapshot for the device.
- Keep signing keys and MQTT credentials in environment-managed secrets.
- Never log secrets, tokens, or raw enrollment payloads.
- Rotate signing and broker credentials on a documented schedule.
- Enrollment tokens are one-time use, tracked by status, and can be validated, consumed, expired, or revoked without leaking the raw token secret.
- Device secrets are rotated only through a new enrollment or an explicit future rotation flow; the old secret stops authenticating the device once replaced.
- A lost, stolen, factory-reset, or duplicate device is recovered by retiring the old binding, revoking any outstanding enrollment token, and issuing a fresh enrollment path for the replacement hardware.
- Rotation does not preserve a stale device secret for fallback use. Once a replacement secret is issued, the old secret must fail authentication and cannot continue to fetch config, commands, or telemetry.
- MQTT broker credentials are provisioned from the server runtime and retired with the same lifecycle discipline as other control-plane secrets. A broker credential leak is handled as a configuration secret incident, not as a device-side exception.

## Device Secret Rotation And Recovery

- Rotation is an explicit administrative recovery action, not a silent background mutation.
- A rotation flow must be able to invalidate the previous secret, create a new enrollment token or equivalent bootstrap path, and audit the operator who approved the change.
- Re-enrollment for lost or replaced hardware should preserve the device identity only when the previous binding is intentionally retired first.
- Duplicate device claims must resolve to a single active binding; any second binding attempt must fail until the previous binding is retired or decommissioned.
- If a device is wiped or reset, the operator should treat it as a new trust event and re-enroll it rather than trying to reuse the old secret.
- Future automated rotation can be added later, but the blueprint must already define the loss, revoke, and rebind outcome.

## Enrollment Recovery

- Enrollment recovery means revoking the old bootstrap path, not broadening trust in the damaged one.
- If a token is leaked before consumption, revoke it and issue a replacement token.
- If a token was already consumed, the recovery path must not resurrect it; bind a fresh device or a fresh secret instead.
- If a device is replaced with the same hardware identity, the old secret must be retired before the new enrollment completes.

## Compliance Features

- Immutable audit log.
- Admin-visible history for critical changes.
- Device command history.
- Premium plugin session history when a plugin creates operator-to-device sessions.
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
- Premium plugin session misuse

## Required Controls

- Request signing on enrollment and sync
- Response signing on sync payloads
- CSRF protection for browser forms
- Per-route permission checks
- Audit logging for all privileged mutations
- Audit logging for plugin session create, connect, cancel, timeout, and command dispatch actions
- Token expiry for enrollment and command links
- Token expiry for plugin-created support sessions
- Server-side validation of device ownership before command delivery
- Explicit rejection of command acknowledgements from the wrong device or wrong secret
- Explicit rejection of consumed, expired, revoked, or otherwise invalid enrollment tokens
- Admin writes for users, roles, policies, devices, apps, files, certificates, and commands must keep CSRF and RBAC checks enabled in the dashboard

## Failure Mode Rules

- If auth fails, return a stable auth error and do not leak existence details.
- If a device is unknown, return a device-not-found result without exposing unrelated tenant data.
- If a signature check fails, reject the request before any side effects.
- If a plugin is disabled, deny plugin calls even if the user is otherwise authenticated.
- If an artifact fails verification, the install must stop and be reported.
