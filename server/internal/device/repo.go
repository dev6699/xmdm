package device

import "context"

type Repository interface {
	ListDevices(ctx context.Context, tenantID string) ([]Device, error)
	CreateDevice(ctx context.Context, tenantID string, req DeviceUpsert) (Device, error)
	UpdateDevice(ctx context.Context, tenantID, id string, req DeviceUpsert) (Device, error)
	RetireDevice(ctx context.Context, tenantID, id string) (Device, error)
	Authenticate(ctx context.Context, tenantID, deviceID, secret string) (Device, error)
}
