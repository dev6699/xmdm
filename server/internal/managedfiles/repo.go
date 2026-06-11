package managedfiles

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListManagedFiles(ctx context.Context, tenantID string, page pagination.Params) ([]ManagedFile, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	GetManagedFile(ctx context.Context, tenantID, id string) (ManagedFile, error)
	CreateManagedFile(ctx context.Context, tenantID string, req ManagedFileUpsert) (ManagedFile, error)
	RetireManagedFile(ctx context.Context, tenantID, id string) (ManagedFile, error)
}
