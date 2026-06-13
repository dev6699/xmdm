package users

import (
	"context"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"
)

type Repository interface {
	ListUsers(ctx context.Context, tenantID string, page pagination.Params) ([]User, error)
	ListActiveUsers(ctx context.Context, tenantID string) ([]User, error)
	GetUser(ctx context.Context, tenantID, id string) (User, error)
	CreateUser(ctx context.Context, tenantID string, req UserUpsert) (User, error)
	UpdateUser(ctx context.Context, tenantID, id string, req UserUpsert) (User, error)
	RetireUser(ctx context.Context, tenantID, id string) (User, error)
	AuthenticateUser(ctx context.Context, tenantID, email, password string) (User, roles.Role, error)
}
