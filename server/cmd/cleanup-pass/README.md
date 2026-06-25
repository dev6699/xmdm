# Cleanup Pass

The cleanup-pass command runs the hardening cleanup pass for one tenant.

It reports and, with `--apply`, fixes:

- stale enrollment tokens that have already expired
- queued or sent commands that have passed their expiry time
- orphaned artifact records no longer referenced by files, certificates, or app versions

Run a dry pass:

```sh
cd server
go run ./cmd/cleanup-pass
```

Apply cleanup for the default tenant:

```sh
cd server
go run ./cmd/cleanup-pass --apply
```

The command uses the same `XMDM_POSTGRES_DSN` and object storage environment variables as the main server.
