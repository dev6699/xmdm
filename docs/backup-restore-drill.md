# Backup And Restore Drill

The backup and restore drill verifies that the PostgreSQL-backed XMDM database
can be backed up, restored into a fresh database, and validated against the
source data.

## Prerequisites

- Docker Compose
- The local PostgreSQL service started by the repo
- Existing schema migrations applied by `infra/test-db-env.sh`

## Run

```sh
cd infra
./backup-restore-drill.sh
```

The script:

1. Boots or reuses the local test database through `infra/test-db-env.sh`.
2. Dumps the source database to a temporary SQL file.
3. Creates a temporary restore database.
4. Restores the dump into the temporary database.
5. Compares row counts across the core tables to confirm the restore matched the source.

## Verification Target

The drill succeeds when the script prints a line like:

```text
restore drill succeeded: source=xmdm_test restore=xmdm_restore_...
```
