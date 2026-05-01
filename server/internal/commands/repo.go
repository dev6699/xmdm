package commands

import "context"

type Repository interface {
	Enqueue(ctx context.Context, tenantID string, req Upsert) ([]Command, error)
	ListRecent(ctx context.Context, tenantID string, limit int) ([]Command, error)
	ListPending(ctx context.Context, tenantID, deviceID string) ([]Command, error)
	Acknowledge(ctx context.Context, tenantID, deviceID, commandID string, req Ack) (Command, error)
}
