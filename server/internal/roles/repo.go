package roles

import (
	"context"
	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListRoles(ctx context.Context, tenantID string, page pagination.Params) ([]Role, error)
	ListActiveRoles(ctx context.Context, tenantID string) ([]Role, error)
	GetRole(ctx context.Context, tenantID, id string) (Role, error)
	CreateRole(ctx context.Context, tenantID string, req RoleUpsert) (Role, error)
	UpdateRole(ctx context.Context, tenantID, id string, req RoleUpsert) (Role, error)
	RetireRole(ctx context.Context, tenantID, id string) (Role, error)
}
