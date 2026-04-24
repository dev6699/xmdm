package audit

import "context"

type Store interface {
	Record(ctx context.Context, tenantID, actor, action, resourceType, resourceID string, details map[string]any) (Event, error)
	List(ctx context.Context, tenantID string) ([]Event, error)
}
