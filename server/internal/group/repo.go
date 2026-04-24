package group

import "context"

type Repository interface {
	ListGroups(ctx context.Context, tenantID string) ([]Group, error)
	CreateGroup(ctx context.Context, tenantID string, req GroupUpsert) (Group, error)
	UpdateGroup(ctx context.Context, tenantID, id string, req GroupUpsert) (Group, error)
	RetireGroup(ctx context.Context, tenantID, id string) (Group, error)
}
