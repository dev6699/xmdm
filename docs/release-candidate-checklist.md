# Release Candidate Checklist

Use this checklist on the staging device set before promoting a release.

It assumes the server, database, object storage, and MQTT broker are already deployed in the target staging environment.

Record completed runs in your external release-candidate evidence log.

## Preflight

- [ ] Confirm the release build is the intended server and launcher pair.
- [ ] Confirm the rollback runbook is available in [Disaster Recovery And Rollback](disaster-recovery-and-rollback.md).
- [ ] Confirm the backup-and-restore drill has passed for the current schema.
- [ ] Confirm `/metrics` is reachable and request logs include request IDs and trace headers.
- [ ] Confirm the staging test database and object storage are healthy before touching devices.

## Automated CI Checks

Most release-candidate coverage is already automated in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml). Before promoting a release, confirm the candidate commit has passed the CI workflow for:

- docs link and lint checks
- Go unit and database-backed tests
- server build
- Android unit tests
- Android debug build
- Playwright dashboard tests

## Manual Device Checks

Run the adb-backed device E2E coverage on at least one staging device that can be provisioned as device owner. See [Server E2E README](../server/e2e/README.md) for the exact test inventory.

- [ ] adb-backed device e2e suite

These checks confirm:

- enrollment succeeds on a real device
- content, certificates, logs, device info, policy, kiosk, and command flows behave on-device
- any device-specific regressions are captured before promotion

## Release Gate

Do not promote the release unless all of the following are true:

- [ ] No critical defect was found in the device-backed checks.
- [ ] The rollback path was reviewed and is understood by the operator on call.
- [ ] The adb-backed device e2e suite passed on the staging device set.
- [ ] Any deviations, retries, or environment issues are recorded before the release is cut.

## Related Docs

- [Server E2E README](../server/e2e/README.md)
- [Observability](observability.md)
- [Disaster Recovery And Rollback](disaster-recovery-and-rollback.md)
