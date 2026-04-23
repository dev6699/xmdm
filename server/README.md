# Server

Go control plane implementation lives here.

### Admin Auth

The initial backend slice provides a minimal session-based admin auth flow:

- `GET /admin/login` renders a simple form
- `POST /admin/login` creates a session cookie
- `POST /admin/logout` clears the session cookie
- `GET /admin/me` verifies the active session
- `GET /admin/devices` demonstrates a permission-gated admin route

Run it with:

```sh
cd server
go test ./...
go run ./cmd/server
```
