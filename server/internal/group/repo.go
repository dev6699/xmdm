package group

import (
	"context"

	"xmdm/server/internal/device"
	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListGroups(ctx context.Context, tenantID string, page pagination.Params) ([]Group, error)
	ListActiveGroups(ctx context.Context, tenantID string) ([]Group, error)
	GetGroup(ctx context.Context, tenantID, id string) (Group, error)
	ListGroupDevices(ctx context.Context, tenantID, groupID string, page pagination.Params) ([]device.Device, error)
	CreateGroup(ctx context.Context, tenantID string, req GroupUpsert) (Group, error)
	UpdateGroup(ctx context.Context, tenantID, id string, req GroupUpsert) (Group, error)
	RetireGroup(ctx context.Context, tenantID, id string) (Group, error)
}
