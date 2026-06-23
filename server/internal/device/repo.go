package device

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListDevices(ctx context.Context, tenantID string, page pagination.Params) ([]Device, error)
	ListDevicesByFilter(ctx context.Context, tenantID string, page pagination.Params, filter DeviceListFilter) ([]Device, error)
	ListActiveDevices(ctx context.Context, tenantID string) ([]Device, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	GetStatusCounts(ctx context.Context, tenantID string) (StatusCounts, error)
	GetDevice(ctx context.Context, tenantID, id string) (Device, error)
	CreateDevice(ctx context.Context, tenantID string, req DeviceUpsert) (Device, error)
	UpdateDevice(ctx context.Context, tenantID, id string, req DeviceUpsert) (Device, error)
	RetireDevice(ctx context.Context, tenantID, id string) (Device, error)
	Authenticate(ctx context.Context, tenantID, deviceID, secret string) (Device, error)
}

type HealthFilter string

type DeviceListFilter struct {
	Health    HealthFilter
	NameQuery string
}

const (
	HealthFilterAll        HealthFilter = ""
	HealthFilterLowBattery HealthFilter = "low"
	HealthFilterStale      HealthFilter = "stale"
)
