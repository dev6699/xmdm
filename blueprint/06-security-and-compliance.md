# Security And Compliance

## Security Principles

- Separate admin and device credentials.
- Use least privilege for admin permissions.
- Sign config snapshots before the launcher applies them.
- Verify downloadable artifact checksums.
- Record privileged admin mutations in audit where implemented.
- Treat MQTT as a delivery channel, not policy authority.
- Keep deployment secrets outside committed defaults.

## Trust Boundaries

- Browser session to admin dashboard.
- Public enrollment-token flow before device trust exists.
- Device-secret boundary after enrollment.
- PostgreSQL as authoritative control-plane state.
- Object storage as artifact storage accessed through server authorization.
- MQTT broker as command push transport.
- Optional plugin boundary inside the trusted server process after explicit
  registration.

## Non-Negotiable Invariants

- Browser mutations must keep CSRF and RBAC checks enabled.
- Device routes must authenticate enrolled devices before accepting state or
  serving protected artifacts.
- Config snapshots must be verified by the launcher before local application.
- Command acknowledgements must be accepted only from the device that owns the
  command.
- Production TLS termination and secret injection are deployment
  responsibilities.

The operator-facing security summary lives in
[../docs/security-overview.md](../docs/security-overview.md).
