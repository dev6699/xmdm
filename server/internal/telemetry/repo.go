package telemetry

import "context"

type Repository interface {
	Upload(ctx context.Context, tenantID, deviceID, secret string, req UploadRequest) (Record, error)
}
