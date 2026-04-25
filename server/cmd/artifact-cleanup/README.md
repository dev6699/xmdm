# Artifact Cleanup

This command inspects the artifact table for rows that are no longer referenced by files, certificates, or app versions.

Run a dry pass:

```sh
cd server
go run ./cmd/artifact-cleanup
```

Apply cleanup for the default tenant:

```sh
cd server
go run ./cmd/artifact-cleanup --apply
```

The command uses the same `XMDM_POSTGRES_DSN` and object storage environment variables as the main server.
