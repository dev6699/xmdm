package identity

import "context"

type Repository interface {
	ListUsers(ctx context.Context, tenantID string) ([]User, error)
	CreateUser(ctx context.Context, tenantID string, req UserUpsert) (User, error)
	UpdateUser(ctx context.Context, tenantID, id string, req UserUpsert) (User, error)
	RetireUser(ctx context.Context, tenantID, id string) (User, error)
	AuthenticateUser(ctx context.Context, tenantID, email, password string) (User, Role, error)

	ListRoles(ctx context.Context, tenantID string) ([]Role, error)
	CreateRole(ctx context.Context, tenantID string, req RoleUpsert) (Role, error)
	UpdateRole(ctx context.Context, tenantID, id string, req RoleUpsert) (Role, error)
	RetireRole(ctx context.Context, tenantID, id string) (Role, error)
}
