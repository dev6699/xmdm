# Cleanup Pass

The cleanup pass is the last hardening maintenance sweep before release.

It addresses the known backlog debris called out in the roadmap:

- stale enrollment tokens
- stuck commands that are past their expiry time
- orphaned artifact records

## What It Does

The cleanup pass uses the existing server maintenance paths:

- expired enrollment tokens are marked expired
- expired queued or sent commands are marked expired
- orphaned artifact rows are purged after their blobs are removed from object storage

## How To Run

From the server tree:

```sh
go run ./cmd/cleanup-pass --apply
```

If you want to inspect the candidates first, omit `--apply` and review the reported counts.

## Verification

After the pass, rerun it in dry-run mode and confirm the candidate counts stay at zero for the tenant you cleaned.

If counts remain non-zero, inspect the corresponding rows before release.
