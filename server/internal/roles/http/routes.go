package roleshttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"
)

func Register(mux httpx.Router, svc *auth.Service, store roles.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[roles.RoleUpsert, roles.Role]{
		Kind:      "roles",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeRoleRequest,
		List: func(ctx context.Context, params pagination.Params) ([]roles.Role, error) {
			return store.ListRoles(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req roles.RoleUpsert) (roles.Role, error) {
			return store.CreateRole(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req roles.RoleUpsert) (roles.Role, error) {
			return store.UpdateRole(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (roles.Role, error) {
			return store.RetireRole(ctx, tenantID, id)
		},
		Audit: func(rec roles.Role) map[string]any {
			return map[string]any{"name": rec.Name}
		},
	})
}

func decodeRoleRequest(r *http.Request) (roles.RoleUpsert, error) {
	var payload roles.RoleUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return roles.RoleUpsert{}, err
	}
	if payload.Name == "" {
		return roles.RoleUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}
