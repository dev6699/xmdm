package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(...any) error
}

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, now: time.Now}
}

func (s *Store) SetNow(now func() time.Time) {
	s.now = now
}

func (s *Store) CreateRole(ctx context.Context, tenantID string, req roles.RoleUpsert) (roles.Role, error) {
	if req.Name == "" {
		return roles.Role{}, httpx.ErrInvalidInput
	}
	payload, err := json.Marshal(req.Permissions)
	if err != nil {
		return roles.Role{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5)
		 RETURNING id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Name, payload, s.now(),
	)
	return scanRole(row)
}

func (s *Store) ListRoles(ctx context.Context, tenantID string, page pagination.Params) ([]roles.Role, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at
		 FROM roles
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]roles.Role, 0)
	for rows.Next() {
		rec, err := scanRole(rows)
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

func (s *Store) ListActiveRoles(ctx context.Context, tenantID string) ([]roles.Role, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at
		 FROM roles
		 WHERE tenant_id = $1 AND status = 'active'
		 ORDER BY created_at DESC, id DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]roles.Role, 0)
	for rows.Next() {
		rec, err := scanRole(rows)
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

func (s *Store) GetRole(ctx context.Context, tenantID, id string) (roles.Role, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at
		 FROM roles
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	rec, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return roles.Role{}, httpx.ErrNotFound
		}
		return roles.Role{}, err
	}
	return rec, nil
}

func (s *Store) UpdateRole(ctx context.Context, tenantID, id string, req roles.RoleUpsert) (roles.Role, error) {
	if req.Name == "" {
		return roles.Role{}, httpx.ErrInvalidInput
	}
	payload, err := json.Marshal(req.Permissions)
	if err != nil {
		return roles.Role{}, err
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE roles
		 SET name = $3, permissions = $4::jsonb, updated_at = $5
		 WHERE tenant_id = $1 AND id = $2 AND status = 'active'
		 RETURNING id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at`,
		tenantID, id, req.Name, payload, s.now(),
	)
	rec, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return roles.Role{}, httpx.ErrNotFound
		}
		return roles.Role{}, err
	}
	return rec, nil
}

func (s *Store) RetireRole(ctx context.Context, tenantID, id string) (roles.Role, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE roles
		 SET status = 'retired', deleted_at = $3, updated_at = $3
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at`,
		tenantID, id, s.now(),
	)
	rec, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return roles.Role{}, httpx.ErrNotFound
		}
		return roles.Role{}, err
	}
	return rec, nil
}

func scanRole(scanner rowScanner) (roles.Role, error) {
	var rec roles.Role
	var deletedAt pgtype.Timestamptz
	var permissions json.RawMessage
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &permissions, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return roles.Role{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if len(permissions) > 0 {
		if err := json.Unmarshal(permissions, &rec.Permissions); err != nil {
			return roles.Role{}, err
		}
	}
	return rec, nil
}
