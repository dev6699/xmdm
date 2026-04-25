package commands

import "context"

type Repository interface {
	Enqueue(ctx context.Context, tenantID, deviceID string, req Upsert) (Command, error)
	ListPending(ctx context.Context, tenantID, deviceID string) ([]Command, error)
}
