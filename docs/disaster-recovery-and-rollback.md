# Disaster Recovery And Rollback

Use this runbook when XMDM loses a core service, data store, or release rollout.

It assumes the single-tenant self-hosted deployment described in [blueprint/07-operations.md](../blueprint/07-operations.md).

## Recovery Order

1. Stop writes to the affected environment.
2. Capture the current state for later review.
3. Restore PostgreSQL from the latest known-good backup.
4. Restore object storage artifacts if the content bucket was lost or corrupted.
5. Restart the server and verify the liveness endpoint and dashboard health strip.
6. Verify a device can fetch config and content again.
7. Resume traffic only after the core flows pass.

## PostgreSQL Recovery

Use the documented restore drill in [backup-restore-drill.md](backup-restore-drill.md) and the matching script in `infra/backup-restore-drill.sh` as the reference procedure.

For an actual incident:

- create a fresh restore database
- restore the backup into that database
- compare core table counts before swapping traffic
- point the server back at the restored database only after verification

## Object Storage Recovery

If object storage is lost or corrupted:

- restore the artifact bucket from the last object-storage backup
- verify app, file, certificate, and managed-file downloads
- confirm checksums still match the metadata rows in PostgreSQL

If only a small set of artifacts is impacted, prefer targeted replacement over a full bucket restore when that is faster and safer.

## Release Rollback

If a release must be rolled back:

1. Revert the server and launcher release artifacts to the previous known-good version.
2. Restart the server with the previous image or binary.
3. Reinstall or redeploy the matching Android launcher build if the rollout touched launcher code.
4. Verify admin login, device sync, and command push before re-enabling traffic.

If the failed release included schema changes that require database rollback,
restore the pre-upgrade database backup first, then roll back the binaries.

## Incident Checklist

Use this checklist when the problem is still active and you need to stabilize the environment before deeper debugging.

### Database Down

- Stop write traffic if the server is still accepting requests.
- Confirm the failure is limited to PostgreSQL and not a broader host outage.
- Restore the latest known-good backup into a fresh database.
- Verify the admin dashboard health strip shows the backend checks as healthy and the backup drill passed before re-pointing the server.

### Object Storage Down

- Pause uploads and downloads that depend on the artifact bucket.
- Restore the bucket or the affected prefix from the latest backup.
- Verify app, file, certificate, and managed-file downloads.
- Confirm checksums still match the metadata rows in PostgreSQL.

### MQTT Broker Down

- Confirm whether the broker or only the broker credentials failed.
- Leave command rows in PostgreSQL as the source of truth.
- Verify the HTTP polling fallback still returns pending commands.
- Resume MQTT delivery only after the dashboard MQTT publish indicator is healthy again.

### Bad APK Published

- Replace the managed app version in the dashboard with the known-good APK.
- Confirm the latest published version is the expected one.
- Regenerate enrollment data if it referenced the wrong version.
- Verify a device can download the corrected launcher artifact.

### Migration Failed

- Stop the rollout before retrying the migration blindly.
- Inspect the migration logs and the schema state.
- Restore the pre-upgrade database backup if the migration partially applied.
- Re-run the backup-and-restore drill before retrying the release.

### Device Fleet Stopped Syncing

- Check admin auth, enrollment state, and device secrets.
- Confirm the device can still reach the server and broker.
- Use the overview page and device detail page to find the last successful sync.
- Verify command polling, config fetch, and artifact downloads one by one.

## Verification Checklist

Recovery is complete when all of these are true:

- the admin dashboard health strip returns healthy statuses for each backend dependency or explains the failing one
- `GET /api/v1/devices/{deviceId}/config` returns a signed snapshot for a known device
- app, file, certificate, and managed-file downloads work
- command polling and acknowledgements work
- the restore drill or equivalent verification has passed on the restored data set

## Related Procedures

- [Backup And Restore Drill](backup-restore-drill.md)
- [Observability](observability.md)
- [Local Development](../infra/local-dev.md)
