package logs

import (
	"context"
)

type Repository interface {
	Upload(ctx context.Context, tenantID, deviceID, secret string, req UploadRequest) ([]Record, error)
	Search(ctx context.Context, tenantID string, filter SearchFilter) ([]Record, error)
}
