package identitypg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
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
	return &Store{
		pool: pool,
		now:  time.Now,
	}
}

func (s *Store) SetNow(now func() time.Time) {
	s.now = now
}

func (s *Store) CreateUser(ctx context.Context, tenantID string, req identity.UserUpsert) (identity.User, error) {
	if req.Email == "" || req.PasswordHash == "" || req.RoleID == "" {
		return identity.User{}, httpx.ErrInvalidInput
	}
	now := s.now()
	row := s.pool.QueryRow(ctx,
		`INSERT INTO users (id, tenant_id, email, password_hash, role_id, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Email, req.PasswordHash, req.RoleID, now,
	)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context, tenantID string, page pagination.Params) ([]identity.User, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at
		 FROM users
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]identity.User, 0)
	for rows.Next() {
		rec, err := scanUser(rows)
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

func (s *Store) ListActiveUsers(ctx context.Context, tenantID string) ([]identity.User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at
		 FROM users
		 WHERE tenant_id = $1 AND status = 'active'
		 ORDER BY created_at DESC, id DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]identity.User, 0)
	for rows.Next() {
		rec, err := scanUser(rows)
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

func (s *Store) GetUser(ctx context.Context, tenantID, id string) (identity.User, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at
		 FROM users
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	rec, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, httpx.ErrNotFound
		}
		return identity.User{}, err
	}
	return rec, nil
}

func (s *Store) UpdateUser(ctx context.Context, tenantID, id string, req identity.UserUpsert) (identity.User, error) {
	if req.Email == "" || req.RoleID == "" {
		return identity.User{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE users
		 SET email = $3, password_hash = COALESCE(NULLIF($4, ''), password_hash), role_id = $5, updated_at = $6
		 WHERE tenant_id = $1 AND id = $2 AND status = 'active'
		 RETURNING id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at`,
		tenantID, id, req.Email, req.PasswordHash, req.RoleID, s.now(),
	)
	rec, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, httpx.ErrNotFound
		}
		return identity.User{}, err
	}
	return rec, nil
}

func (s *Store) RetireUser(ctx context.Context, tenantID, id string) (identity.User, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE users
		 SET status = 'retired', deleted_at = $3, updated_at = $3
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, email, role_id::text, status, created_at, updated_at, deleted_at`,
		tenantID, id, s.now(),
	)
	rec, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, httpx.ErrNotFound
		}
		return identity.User{}, err
	}
	return rec, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, tenantID, email, password string) (identity.User, identity.Role, error) {
	if tenantID == "" || email == "" || password == "" {
		return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
	}
	row := s.pool.QueryRow(ctx,
		`SELECT
			u.id::text, u.tenant_id::text, u.email, u.password_hash, u.role_id::text, u.status, u.created_at, u.updated_at, u.deleted_at,
			r.id::text, r.name, r.permissions, r.status, r.created_at, r.updated_at, r.deleted_at
		 FROM users u
		 JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
		 WHERE u.tenant_id = $1 AND u.email = $2`,
		tenantID, email,
	)

	var user identity.User
	var passwordHash string
	var role identity.Role
	var rolePermissions json.RawMessage
	var userDeletedAt pgtype.Timestamptz
	var roleDeletedAt pgtype.Timestamptz
	if err := row.Scan(&user.ID, &user.TenantID, &user.Email, &passwordHash, &user.RoleID, &user.Status, &user.CreatedAt, &user.UpdatedAt, &userDeletedAt, &role.ID, &role.Name, &rolePermissions, &role.Status, &role.CreatedAt, &role.UpdatedAt, &roleDeletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
		}
		return identity.User{}, identity.Role{}, err
	}
	if userDeletedAt.Valid {
		user.DeletedAt = &userDeletedAt.Time
	}
	if roleDeletedAt.Valid {
		role.DeletedAt = &roleDeletedAt.Time
	}
	if !identity.VerifyPassword(passwordHash, password) {
		return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
	}
	if user.Status != "active" || role.Status != "active" {
		return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
	}
	permissions, err := identity.PermissionsFromJSON(rolePermissions)
	if err != nil {
		return identity.User{}, identity.Role{}, err
	}
	role.Permissions = permissions
	return user, role, nil
}

func (s *Store) CreateRole(ctx context.Context, tenantID string, req identity.RoleUpsert) (identity.Role, error) {
	if req.Name == "" {
		return identity.Role{}, httpx.ErrInvalidInput
	}
	payload, err := json.Marshal(req.Permissions)
	if err != nil {
		return identity.Role{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5)
		 RETURNING id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Name, payload, s.now(),
	)
	return scanRole(row)
}

func (s *Store) ListRoles(ctx context.Context, tenantID string, page pagination.Params) ([]identity.Role, error) {
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

	items := make([]identity.Role, 0)
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

func (s *Store) ListActiveRoles(ctx context.Context, tenantID string) ([]identity.Role, error) {
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

	items := make([]identity.Role, 0)
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

func (s *Store) GetRole(ctx context.Context, tenantID, id string) (identity.Role, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, permissions, status, created_at, updated_at, deleted_at
		 FROM roles
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	rec, err := scanRole(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Role{}, httpx.ErrNotFound
		}
		return identity.Role{}, err
	}
	return rec, nil
}

func (s *Store) UpdateRole(ctx context.Context, tenantID, id string, req identity.RoleUpsert) (identity.Role, error) {
	if req.Name == "" {
		return identity.Role{}, httpx.ErrInvalidInput
	}
	payload, err := json.Marshal(req.Permissions)
	if err != nil {
		return identity.Role{}, err
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
			return identity.Role{}, httpx.ErrNotFound
		}
		return identity.Role{}, err
	}
	return rec, nil
}

func (s *Store) RetireRole(ctx context.Context, tenantID, id string) (identity.Role, error) {
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
			return identity.Role{}, httpx.ErrNotFound
		}
		return identity.Role{}, err
	}
	return rec, nil
}

func scanUser(scanner rowScanner) (identity.User, error) {
	var rec identity.User
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Email, &rec.RoleID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return identity.User{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanRole(scanner rowScanner) (identity.Role, error) {
	var rec identity.Role
	var deletedAt pgtype.Timestamptz
	var permissions json.RawMessage
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &permissions, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return identity.Role{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if len(permissions) > 0 {
		if err := json.Unmarshal(permissions, &rec.Permissions); err != nil {
			return identity.Role{}, err
		}
	}
	return rec, nil
}
