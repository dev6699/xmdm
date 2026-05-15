package apps

import "context"

type Repository interface {
	ListApps(ctx context.Context, tenantID string) ([]App, error)
	GetApp(ctx context.Context, tenantID, id string) (App, error)
	CreateApp(ctx context.Context, tenantID string, req AppUpsert) (App, error)
	UpdateApp(ctx context.Context, tenantID, id string, req AppUpsert) (App, error)
	RetireApp(ctx context.Context, tenantID, id string) (App, error)
	ListVersions(ctx context.Context, tenantID, appID string) ([]Version, error)
	GetVersion(ctx context.Context, tenantID, appID, versionID string) (Version, error)
	CreateVersion(ctx context.Context, tenantID, appID string, req VersionUpsert) (Version, error)
}
