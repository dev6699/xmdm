# Server

Go control plane implementation lives here.

Use this directory for backend development, server tests, and operator-facing API behavior.

## Canonical Docs

- [Admin Dashboard](../docs/admin-dashboard.md)
- [Observability](../docs/observability.md)
- [Disaster Recovery And Rollback](../docs/disaster-recovery-and-rollback.md)
- [Server E2E](e2e/README.md)
- [Release Artifacts And Deployment](../docs/release-artifacts-and-deployment.md)

## Common Commands

```sh
cd server
go run ./cmd/server --config config.yaml
```

- The checked-in starting point is [config.yaml](config.yaml).
- Server tests require `XMDM_TEST_POSTGRES_DSN`. Use a separate database name, not the runtime database.
- `go run ./cmd/server --config config.yaml --migrate-only` applies the embedded database migrations and exits.

For a full test run against a local test database:

```sh
cd server
eval "$(../infra/test-db-env.sh)"
go test -p 1 -count=1 $(go list ./... | grep -v "/e2e")
```
