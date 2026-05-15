package grouppg

import (
	"context"
	"errors"
	"time"

	group "xmdm/server/internal/group"
	"xmdm/server/internal/httpx"

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

func (s *Store) ListGroups(ctx context.Context, tenantID string) ([]group.Group, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, status, created_at, updated_at, deleted_at
		 FROM groups
		 WHERE tenant_id = $1
		 ORDER BY created_at`,
		tenantID,
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
