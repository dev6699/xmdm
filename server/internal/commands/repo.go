package commands

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	Enqueue(ctx context.Context, tenantID string, req Upsert) ([]Command, error)
	ListRecent(ctx context.Context, tenantID string, page pagination.Params) ([]Command, error)
	ListRecentAll(ctx context.Context, tenantID string) ([]Command, error)
	ListPendingForDevice(ctx context.Context, tenantID, deviceID string) ([]Command, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	Get(ctx context.Context, tenantID, commandID string) (Command, error)
	ListPending(ctx context.Context, tenantID, deviceID string, page pagination.Params) ([]Command, error)
	Acknowledge(ctx context.Context, tenantID, deviceID, commandID string, req Ack) (Command, error)
}
