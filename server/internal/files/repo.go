package files

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListFiles(ctx context.Context, tenantID string, page pagination.Params) ([]File, error)
	GetFile(ctx context.Context, tenantID, id string) (File, error)
	CreateFile(ctx context.Context, tenantID string, req FileUpsert) (File, error)
	RetireFile(ctx context.Context, tenantID, id string) (File, error)
}
