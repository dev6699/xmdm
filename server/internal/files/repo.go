package files

import "context"

type Repository interface {
	ListFiles(ctx context.Context, tenantID string) ([]File, error)
	CreateFile(ctx context.Context, tenantID string, req FileUpsert) (File, error)
	RetireFile(ctx context.Context, tenantID, id string) (File, error)
}
