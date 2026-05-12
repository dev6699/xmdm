# Disaster Recovery And Rollback

This runbook covers the recovery steps a maintainer should use when XMDM loses a core service, data store, or release rollout.

It assumes the single-tenant self-hosted deployment described in [blueprint/07-operations.md](../blueprint/07-operations.md).

## Recovery Order

1. Stop writes to the affected environment.
2. Capture the current state for later review.
3. Restore PostgreSQL from the latest known-good backup.
4. Restore object storage artifacts if the content bucket was lost or corrupted.
5. Restart the server and verify health endpoints.
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

If the object store is lost or corrupted:

- restore the artifact bucket from the last object-storage backup
- verify app, file, certificate, and managed-file downloads
- confirm checksums still match the metadata rows in PostgreSQL

If only a small set of artifacts is impacted, prefer targeted replacement over a full bucket restore when that is faster and safer.

## Release Rollback

If a release must be rolled back:

1. Revert the server and agent release artifacts to the previous known-good version.
2. Restart the server with the previous image or binary.
3. Reinstall or redeploy the matching Android agent build if the rollout touched agent code.
4. Verify admin login, device sync, and command push before re-enabling traffic.

If the failed release included schema changes that are not backward compatible, do not attempt a binary-only rollback. Restore the pre-upgrade database backup first, then roll back the binaries.

## Verification Checklist

Recovery is complete when all of these are true:

- `GET /health` succeeds
- `GET /api/v1/devices/{deviceId}/config` returns a signed snapshot for a known device
- app, file, certificate, and managed-file downloads work
- command polling and acknowledgements work
- the restore drill or equivalent verification has passed on the restored data set

## Related Procedures

- [Backup And Restore Drill](backup-restore-drill.md)
- [Observability](observability.md)
- [Local Development](../infra/local-dev.md)
