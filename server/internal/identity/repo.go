package identity

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListUsers(ctx context.Context, tenantID string, page pagination.Params) ([]User, error)
	ListActiveUsers(ctx context.Context, tenantID string) ([]User, error)
	GetUser(ctx context.Context, tenantID, id string) (User, error)
	CreateUser(ctx context.Context, tenantID string, req UserUpsert) (User, error)
	UpdateUser(ctx context.Context, tenantID, id string, req UserUpsert) (User, error)
	RetireUser(ctx context.Context, tenantID, id string) (User, error)
	AuthenticateUser(ctx context.Context, tenantID, email, password string) (User, Role, error)

	ListRoles(ctx context.Context, tenantID string, page pagination.Params) ([]Role, error)
	ListActiveRoles(ctx context.Context, tenantID string) ([]Role, error)
	GetRole(ctx context.Context, tenantID, id string) (Role, error)
	CreateRole(ctx context.Context, tenantID string, req RoleUpsert) (Role, error)
	UpdateRole(ctx context.Context, tenantID, id string, req RoleUpsert) (Role, error)
	RetireRole(ctx context.Context, tenantID, id string) (Role, error)
}
