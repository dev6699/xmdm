package identityhttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/pagination"
)

func Register(mux httpx.Router, svc *auth.Service, store identity.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[identity.UserUpsert, identity.User]{
		Kind:      "users",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeUserRequest,
		List: func(ctx context.Context, params pagination.Params) ([]identity.User, error) {
			return store.ListUsers(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req identity.UserUpsert) (identity.User, error) {
			return store.CreateUser(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req identity.UserUpsert) (identity.User, error) {
			return store.UpdateUser(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (identity.User, error) {
			return store.RetireUser(ctx, tenantID, id)
		},
		Audit: func(rec identity.User) map[string]any {
			return map[string]any{"email": rec.Email}
		},
	})

	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[identity.RoleUpsert, identity.Role]{
		Kind:      "roles",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeRoleRequest,
		List: func(ctx context.Context, params pagination.Params) ([]identity.Role, error) {
			return store.ListRoles(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req identity.RoleUpsert) (identity.Role, error) {
			return store.CreateRole(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req identity.RoleUpsert) (identity.Role, error) {
			return store.UpdateRole(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (identity.Role, error) {
			return store.RetireRole(ctx, tenantID, id)
		},
		Audit: func(rec identity.Role) map[string]any {
			return map[string]any{"name": rec.Name}
		},
	})
}

func decodeUserRequest(r *http.Request) (identity.UserUpsert, error) {
	var payload identity.UserUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return identity.UserUpsert{}, err
	}
	if payload.Email == "" || payload.PasswordHash == "" || payload.RoleID == "" {
		return identity.UserUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func decodeRoleRequest(r *http.Request) (identity.RoleUpsert, error) {
	var payload identity.RoleUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return identity.RoleUpsert{}, err
	}
	if payload.Name == "" {
		return identity.RoleUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}
