package policypg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"xmdm/server/internal/httpx"
	policy "xmdm/server/internal/policy"

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

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (s *Store) SetNow(now func() time.Time) { s.now = now }

func (s *Store) CreatePolicy(ctx context.Context, tenantID string, req policy.PolicyUpsert) (policy.Policy, error) {
	if req.Name == "" {
		return policy.Policy{}, httpx.ErrInvalidInput
	}
	restrictions := req.Restrictions
	if len(restrictions) == 0 || string(restrictions) == "null" {
		restrictions = []byte(`{}`)
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO policies (id, tenant_id, name, version, kiosk_mode, restrictions_json, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, restrictions_json, status, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Name, req.Version, req.KioskMode, restrictions, s.now(),
	)
	return scanPolicy(row)
}

func (s *Store) ListPolicies(ctx context.Context, tenantID string) ([]policy.Policy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, version, kiosk_mode, restrictions_json, status, updated_at, deleted_at
		 FROM policies
		 WHERE tenant_id = $1
		 ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.Policy, 0)
	for rows.Next() {
		rec, err := scanPolicy(rows)
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

func (s *Store) UpdatePolicy(ctx context.Context, tenantID, id string, req policy.PolicyUpsert) (policy.Policy, error) {
	if req.Name == "" {
		return policy.Policy{}, httpx.ErrInvalidInput
	}
	restrictions := req.Restrictions
	if len(restrictions) == 0 || string(restrictions) == "null" {
		restrictions = []byte(`{}`)
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE policies
		 SET name = $3, version = $4, kiosk_mode = $5, restrictions_json = $6::jsonb, updated_at = $7
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, restrictions_json, status, updated_at, deleted_at`,
		tenantID, id, req.Name, req.Version, req.KioskMode, restrictions, s.now(),
	)
	rec, err := scanPolicy(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.Policy{}, httpx.ErrNotFound
		}
		return policy.Policy{}, err
	}
	return rec, nil
}

func (s *Store) RetirePolicy(ctx context.Context, tenantID, id string) (policy.Policy, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE policies
		 SET status = 'retired', deleted_at = $3, updated_at = $3
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, restrictions_json, status, updated_at, deleted_at`,
		tenantID, id, s.now(),
	)
	rec, err := scanPolicy(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.Policy{}, httpx.ErrNotFound
		}
		return policy.Policy{}, err
	}
	return rec, nil
}

func scanPolicy(scanner rowScanner) (policy.Policy, error) {
	var rec policy.Policy
	var deletedAt pgtype.Timestamptz
	var restrictions json.RawMessage
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Version, &rec.KioskMode, &restrictions, &rec.Status, &rec.UpdatedAt, &deletedAt); err != nil {
		return policy.Policy{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if len(restrictions) > 0 {
		rec.Restrictions = append(json.RawMessage(nil), restrictions...)
	} else {
		rec.Restrictions = json.RawMessage(`{}`)
	}
	return rec, nil
}
