package auditpg

import (
	"context"
	"encoding/json"
	"time"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/pagination"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DBStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewDBStore(pool *pgxpool.Pool) *DBStore {
	return &DBStore{
		pool: pool,
		now:  time.Now,
	}
}

func (s *DBStore) SetNow(now func() time.Time) {
	s.now = now
}

func (s *DBStore) Record(ctx context.Context, tenantID, actor, action, resourceType, resourceID string, details map[string]any) (audit.Event, error) {
	createdAt := s.now()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO audit_events (tenant_id, actor, action, resource_type, resource_id, details, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, actor, action, resourceType, resourceID, details, createdAt,
	); err != nil {
		return audit.Event{}, err
	}
	return audit.Event{
		TenantID:     tenantID,
		Actor:        actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CreatedAt:    createdAt,
		Details:      cloneMap(details),
	}, nil
}

func (s *DBStore) List(ctx context.Context, tenantID string, page pagination.Params) ([]audit.Event, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, actor, action, resource_type, resource_id, created_at, details
		 FROM audit_events
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]audit.Event, 0)
	for rows.Next() {
		var event audit.Event
		var details json.RawMessage
		if err := rows.Scan(&event.ID, &event.TenantID, &event.Actor, &event.Action, &event.ResourceType, &event.ResourceID, &event.CreatedAt, &details); err != nil {
			return nil, err
		}
		if len(details) > 0 {
			if err := json.Unmarshal(details, &event.Details); err != nil {
				return nil, err
			}
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *DBStore) ListNewest(ctx context.Context, tenantID string) ([]audit.Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, actor, action, resource_type, resource_id, created_at, details
		 FROM audit_events
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]audit.Event, 0)
	for rows.Next() {
		var event audit.Event
		var details json.RawMessage
		if err := rows.Scan(&event.ID, &event.TenantID, &event.Actor, &event.Action, &event.ResourceType, &event.ResourceID, &event.CreatedAt, &details); err != nil {
			return nil, err
		}
		if len(details) > 0 {
			if err := json.Unmarshal(details, &event.Details); err != nil {
				return nil, err
			}
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *DBStore) CountSince(ctx context.Context, tenantID string, since time.Time) (int, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int
		 FROM audit_events
		 WHERE tenant_id = $1 AND created_at >= $2`,
		tenantID, since,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
