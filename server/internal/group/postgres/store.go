package grouppg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	device "xmdm/server/internal/device"
	group "xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type rowScanner interface {
	Scan(...any) error
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, now: time.Now}
}

func (s *Store) SetNow(now func() time.Time) { s.now = now }

func (s *Store) CreateGroup(ctx context.Context, tenantID string, req group.GroupUpsert) (group.Group, error) {
	if req.Name == "" {
		return group.Group{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO groups (id, tenant_id, name, updated_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Name, s.now(),
	)
	return scanGroup(row)
}

func (s *Store) ListGroups(ctx context.Context, tenantID string, page pagination.Params) ([]group.Group, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at
		 FROM groups
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]group.Group, 0)
	for rows.Next() {
		rec, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) ListActiveGroups(ctx context.Context, tenantID string) ([]group.Group, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at
		 FROM groups
		 WHERE tenant_id = $1 AND status = $2
		 ORDER BY created_at DESC, id DESC`,
		tenantID, group.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]group.Group, 0)
	for rows.Next() {
		rec, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetGroup(ctx context.Context, tenantID, id string) (group.Group, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at
		 FROM groups
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	rec, err := scanGroup(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return group.Group{}, httpx.ErrNotFound
		}
		return group.Group{}, err
	}
	return rec, nil
}

func (s *Store) ListGroupDevices(ctx context.Context, tenantID, groupID string, page pagination.Params) ([]device.Device, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT d.id::text, d.tenant_id::text, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id::text, d.bootstrap_extras::text,
		        COALESCE(array_agg(DISTINCT dg.group_id::text ORDER BY dg.group_id::text) FILTER (WHERE dg.group_id IS NOT NULL), '{}'::text[]) AS group_ids
		 FROM devices d
		 INNER JOIN device_groups g ON g.tenant_id = d.tenant_id AND g.device_id = d.id AND g.group_id = $2
		 LEFT JOIN device_groups dg ON dg.tenant_id = d.tenant_id AND dg.device_id = d.id
		 WHERE d.tenant_id = $1
		 GROUP BY d.id, d.tenant_id, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id, d.bootstrap_extras
		 ORDER BY d.created_at, d.id
		 LIMIT $3 OFFSET $4`,
		tenantID, groupID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]device.Device, 0)
	for rows.Next() {
		rec, err := scanGroupDevice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) UpdateGroup(ctx context.Context, tenantID, id string, req group.GroupUpsert) (group.Group, error) {
	if req.Name == "" {
		return group.Group{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE groups
		 SET name = $3, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at`,
		tenantID, id, req.Name, s.now(),
	)
	rec, err := scanGroup(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return group.Group{}, httpx.ErrNotFound
		}
		return group.Group{}, err
	}
	return rec, nil
}

func (s *Store) RetireGroup(ctx context.Context, tenantID, id string) (group.Group, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE groups
		 SET status = 'retired', deleted_at = $3, updated_at = $3
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at`,
		tenantID, id, s.now(),
	)
	rec, err := scanGroup(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return group.Group{}, httpx.ErrNotFound
		}
		return group.Group{}, err
	}
	return rec, nil
}

func scanGroup(scanner rowScanner) (group.Group, error) {
	var rec group.Group
	var createdAt pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &createdAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return group.Group{}, err
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanGroupDevice(scanner rowScanner) (device.Device, error) {
	var rec device.Device
	var createdAt pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz
	var policyID pgtype.Text
	var bootstrapExtrasJSON pgtype.Text
	var groupIDs []string
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &createdAt, &rec.UpdatedAt, &deletedAt, &policyID, &bootstrapExtrasJSON, &groupIDs); err != nil {
		return device.Device{}, err
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if policyID.Valid {
		value := policyID.String
		rec.PolicyID = &value
	}
	if bootstrapExtrasJSON.Valid && strings.TrimSpace(bootstrapExtrasJSON.String) != "" {
		var extras map[string]any
		if err := json.Unmarshal([]byte(bootstrapExtrasJSON.String), &extras); err != nil {
			return device.Device{}, err
		}
		rec.BootstrapExtras = extras
	}
	rec.GroupIDs = groupIDs
	return rec, nil
}
