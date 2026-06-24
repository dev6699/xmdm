package apps

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListApps(ctx context.Context, tenantID string, page pagination.Params) ([]App, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	GetApp(ctx context.Context, tenantID, id string) (App, error)
	GetAppByPackageName(ctx context.Context, tenantID, packageName string) (App, error)
	UpsertSystemOwnedApp(ctx context.Context, tenantID string, req AppUpsert) (App, error)
	CreateApp(ctx context.Context, tenantID string, req AppUpsert) (App, error)
	UpdateApp(ctx context.Context, tenantID, id string, req AppUpsert) (App, error)
	RetireApp(ctx context.Context, tenantID, id string) (App, error)
	ListVersions(ctx context.Context, tenantID, appID string, page pagination.Params) ([]Version, error)
	GetVersionByCode(ctx context.Context, tenantID, appID string, versionCode int64) (Version, error)
	GetLatestPublishedVersion(ctx context.Context, tenantID, appID string) (Version, error)
	GetVersion(ctx context.Context, tenantID, appID, versionID string) (Version, error)
	CreateVersion(ctx context.Context, tenantID, appID string, req VersionUpsert) (Version, error)
}
