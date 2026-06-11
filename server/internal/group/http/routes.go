package grouphttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	group "xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func Register(mux httpx.Router, svc *auth.Service, store group.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[group.GroupUpsert, group.Group]{
		Kind:      "groups",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodeGroupRequest,
		List: func(ctx context.Context, params pagination.Params) ([]group.Group, error) {
			return store.ListGroups(ctx, tenantID, params)
		},
		Create: func(ctx context.Context, req group.GroupUpsert) (group.Group, error) {
			return store.CreateGroup(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req group.GroupUpsert) (group.Group, error) {
			return store.UpdateGroup(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (group.Group, error) {
			return store.RetireGroup(ctx, tenantID, id)
		},
		Audit: func(rec group.Group) map[string]any {
			return map[string]any{"name": rec.Name}
		},
	})
}

func decodeGroupRequest(r *http.Request) (group.GroupUpsert, error) {
	var payload group.GroupUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return group.GroupUpsert{}, err
	}
	if payload.Name == "" {
		return group.GroupUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}
