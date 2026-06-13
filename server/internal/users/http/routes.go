package usershttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/users"
)

func Register(mux httpx.Router, svc *auth.Service, store users.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[users.UserUpsert, users.User]{
		Kind:      "users",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeUserRequest,
		List: func(ctx context.Context, params pagination.Params) ([]users.User, error) {
			return store.ListUsers(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req users.UserUpsert) (users.User, error) {
			return store.CreateUser(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req users.UserUpsert) (users.User, error) {
			return store.UpdateUser(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (users.User, error) {
			return store.RetireUser(ctx, tenantID, id)
		},
		Audit: func(rec users.User) map[string]any {
			return map[string]any{"email": rec.Email}
		},
	})
}

func decodeUserRequest(r *http.Request) (users.UserUpsert, error) {
	var payload users.UserUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return users.UserUpsert{}, err
	}
	if payload.Email == "" || payload.PasswordHash == "" || payload.RoleID == "" {
		return users.UserUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}
