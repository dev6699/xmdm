package audit

import (
	"context"
	"time"

	"xmdm/server/internal/pagination"
)

type Store interface {
	Record(ctx context.Context, tenantID, actor, action, resourceType, resourceID string, details map[string]any) (Event, error)
	List(ctx context.Context, tenantID string, page pagination.Params) ([]Event, error)
	ListNewest(ctx context.Context, tenantID string) ([]Event, error)
	CountSince(ctx context.Context, tenantID string, since time.Time) (int, error)
}
