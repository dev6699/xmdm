# Release Candidate Checklist

Use this checklist on the staging device set before promoting a release.

It assumes the server, database, object storage, and MQTT broker are already deployed in the target staging environment.

## Preflight

- [ ] Confirm the release build is the intended server and launcher pair.
- [ ] Confirm the rollback runbook is available in [Disaster Recovery And Rollback](disaster-recovery-and-rollback.md).
- [ ] Confirm the backup-and-restore drill has passed for the current schema.
- [ ] Confirm `/metrics` is reachable and request logs include request IDs and trace headers.
- [ ] Confirm the staging test database and object storage are healthy before touching devices.

## Server-Simulated Checks

Run the pure HTTP E2E coverage against the staging server and its test database.

- [ ] `TestAdminE2E`
- [ ] `TestEnrollmentE2E`

These checks confirm:

- admin login and CRUD
- enrollment token issuance and device binding
- signed config fetch after enrollment
- telemetry upload
- audit capture

## Device-Backed Checks

Run these on at least one staging device that can be provisioned as device owner.

### Content And Certificates

- [ ] `TestManagedAppsAndFiles`
- [ ] `TestManagedAppsAndFilesRemoval`
- [ ] `TestCertificatesApplied`

Confirm:

- enrollment succeeds
- the signed config snapshot is fetched
- managed files render on-device
- managed apps install and remove cleanly
- certificates download and install cleanly

### Logs And Device Info

- [ ] `TestDeviceLogsUpload`
- [ ] `TestDeviceInfoReporting`

Confirm:

- launcher logs upload and appear in the API
- device-info reports are recorded and exported

### Policy And Kiosk

- [ ] `TestKioskModeChrome`
- [ ] `TestKioskExitChromeLocal`
- [ ] `TestKioskAdminConfigSyncStatus`
- [ ] `TestKioskAdminConfigSyncTwice`
- [ ] `TestKioskExitChromeCommand`
- [ ] `TestKioskStayAwakeWhilePluggedIn`
- [ ] `TestPackageRules`
- [ ] `TestPolicySync`

Confirm:

- kiosk mode enters and exits correctly
- local passcode unlock works
- server-command unlock works
- config sync updates are reflected on the device
- stay-awake policy is applied
- package suspension and policy refresh behave as expected

### Commands

- [ ] `TestCommandMQTT`
- [ ] `TestCommandMQTTSyncConfig`
- [ ] `TestCommandPolling`
- [ ] `TestCommandBrokerOutageRecovery`

Confirm:

- MQTT command delivery works
- command-triggered config sync works
- HTTP polling fallback works
- broker outage fallback and recovery work

## Release Gate

Do not promote the release unless all of the following are true:

- [ ] No critical defect was found in the device-backed checks.
- [ ] The rollback path was reviewed and is understood by the operator on call.
- [ ] The staging device set can complete the full product path without manual intervention.
- [ ] Any deviations, retries, or environment issues are recorded before the release is cut.

## Related Docs

- [Server E2E README](../server/e2e/README.md)
- [Observability](observability.md)
- [Disaster Recovery And Rollback](disaster-recovery-and-rollback.md)
