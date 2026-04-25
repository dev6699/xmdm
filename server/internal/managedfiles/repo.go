package managedfiles

import "context"

type Repository interface {
	ListManagedFiles(ctx context.Context, tenantID string) ([]ManagedFile, error)
	GetManagedFile(ctx context.Context, tenantID, id string) (ManagedFile, error)
	CreateManagedFile(ctx context.Context, tenantID string, req ManagedFileUpsert) (ManagedFile, error)
	RetireManagedFile(ctx context.Context, tenantID, id string) (ManagedFile, error)
}
