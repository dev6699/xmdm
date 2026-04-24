package enrollmentpg

import (
	"context"
	"errors"
	"time"

	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *Store) SetNow(now func() time.Time) {
	s.now = now
}

func (s *Store) IssueToken(ctx context.Context, tenantID string, expiresAt time.Time) (enrollment.IssuedToken, error) {
	if tenantID == "" || expiresAt.IsZero() {
		return enrollment.IssuedToken{}, httpx.ErrInvalidInput
	}
	now := s.now()
	if !expiresAt.After(now) {
		return enrollment.IssuedToken{}, httpx.ErrInvalidInput
	}
	secret, err := enrollment.NewTokenSecret()
	if err != nil {
		return enrollment.IssuedToken{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO enrollment_tokens (id, tenant_id, token_hash, status, expires_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, tenant_id::text, status, expires_at, consumed_at, revoked_at, created_at, updated_at`,
		uuid.NewString(), tenantID, enrollment.HashToken(secret), enrollment.TokenStatusIssued, expiresAt, now,
	)
	token, err := scanToken(row)
	if err != nil {
		return enrollment.IssuedToken{}, err
	}
	return enrollment.IssuedToken{Token: token, Secret: secret}, nil
}

func (s *Store) ValidateToken(ctx context.Context, tenantID, secret string) (enrollment.Token, error) {
	return s.inspectToken(ctx, tenantID, secret, false)
}

func (s *Store) ConsumeToken(ctx context.Context, tenantID, secret string) (enrollment.Token, error) {
	return s.inspectToken(ctx, tenantID, secret, true)
}

func (s *Store) BindDevice(ctx context.Context, tenantID, token, deviceID string) (enrollment.BoundDevice, error) {
	if tenantID == "" || token == "" || deviceID == "" {
		return enrollment.BoundDevice{}, httpx.ErrInvalidInput
	}
	now := s.now()
	secret, err := enrollment.NewTokenSecret()
	if err != nil {
		return enrollment.BoundDevice{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return enrollment.BoundDevice{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tokenRow, err := loadTokenByHashForUpdate(ctx, tx, tenantID, enrollment.HashToken(token))
	if err != nil {
		return enrollment.BoundDevice{}, err
	}
	exists, err := deviceExistsByTenantAndDeviceIDForUpdate(ctx, tx, tenantID, deviceID)
	if err != nil {
		return enrollment.BoundDevice{}, err
	}
	if exists {
		return enrollment.BoundDevice{}, enrollment.ErrDeviceConflict
	}
	if tokenRow.Status == enrollment.TokenStatusIssued && !tokenRow.ExpiresAt.After(now) {
		tokenRow.Status = enrollment.TokenStatusExpired
		tokenRow.UpdatedAt = now
		if err := updateToken(ctx, tx, tokenRow); err != nil {
			return enrollment.BoundDevice{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return enrollment.BoundDevice{}, err
		}
		return enrollment.BoundDevice{}, enrollment.ErrTokenExpired
	}
	if tokenRow.Status != enrollment.TokenStatusIssued {
		switch tokenRow.Status {
		case enrollment.TokenStatusConsumed:
			return enrollment.BoundDevice{}, enrollment.ErrTokenConsumed
		case enrollment.TokenStatusExpired:
			return enrollment.BoundDevice{}, enrollment.ErrTokenExpired
		case enrollment.TokenStatusRevoked:
			return enrollment.BoundDevice{}, enrollment.ErrTokenRevoked
		default:
			return enrollment.BoundDevice{}, enrollment.ErrTokenConflict
		}
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO devices (id, tenant_id, device_id, secret_hash, status, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING device_id, status`,
		uuid.NewString(), tenantID, deviceID, enrollment.HashToken(secret), device.StatusEnrolled, now,
	)
	var bound enrollment.BoundDevice
	if err := row.Scan(&bound.DeviceID, &bound.Status); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return enrollment.BoundDevice{}, enrollment.ErrDeviceConflict
		}
		return enrollment.BoundDevice{}, err
	}

	tokenRow.Status = enrollment.TokenStatusConsumed
	tokenRow.ConsumedAt = &now
	tokenRow.UpdatedAt = now
	if err := updateToken(ctx, tx, tokenRow); err != nil {
		return enrollment.BoundDevice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return enrollment.BoundDevice{}, err
	}
	bound.DeviceSecret = secret
	return bound, nil
}

func (s *Store) RevokeToken(ctx context.Context, tenantID, id string) (enrollment.Token, error) {
	if tenantID == "" || id == "" {
		return enrollment.Token{}, httpx.ErrInvalidInput
	}
	now := s.now()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return enrollment.Token{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	token, err := loadTokenByIDForUpdate(ctx, tx, tenantID, id)
	if err != nil {
		return enrollment.Token{}, err
	}
	switch token.Status {
	case enrollment.TokenStatusIssued:
		token.Status = enrollment.TokenStatusRevoked
		token.RevokedAt = &now
		token.UpdatedAt = now
		if err := updateToken(ctx, tx, token); err != nil {
			return enrollment.Token{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return enrollment.Token{}, err
		}
		return token, nil
	case enrollment.TokenStatusConsumed:
		return enrollment.Token{}, enrollment.ErrTokenConsumed
	case enrollment.TokenStatusExpired:
		return enrollment.Token{}, enrollment.ErrTokenExpired
	case enrollment.TokenStatusRevoked:
		return enrollment.Token{}, enrollment.ErrTokenRevoked
	default:
		return enrollment.Token{}, enrollment.ErrTokenConflict
	}
}

func (s *Store) ExpireTokens(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.pool.Exec(ctx,
		`UPDATE enrollment_tokens
		 SET status = $2, updated_at = $3
		 WHERE status = $4 AND expires_at <= $1`,
		before, enrollment.TokenStatusExpired, s.now(), enrollment.TokenStatusIssued,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) inspectToken(ctx context.Context, tenantID, secret string, consume bool) (enrollment.Token, error) {
	if tenantID == "" || secret == "" {
		return enrollment.Token{}, httpx.ErrInvalidInput
	}
	now := s.now()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return enrollment.Token{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	token, err := loadTokenByHashForUpdate(ctx, tx, tenantID, enrollment.HashToken(secret))
	if err != nil {
		return enrollment.Token{}, err
	}
	if token.Status == enrollment.TokenStatusIssued && !token.ExpiresAt.After(now) {
		token.Status = enrollment.TokenStatusExpired
		token.UpdatedAt = now
		if err := updateToken(ctx, tx, token); err != nil {
			return enrollment.Token{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return enrollment.Token{}, err
		}
		return enrollment.Token{}, enrollment.ErrTokenExpired
	}
	switch token.Status {
	case enrollment.TokenStatusIssued:
		if consume {
			token.Status = enrollment.TokenStatusConsumed
			token.ConsumedAt = &now
			token.UpdatedAt = now
			if err := updateToken(ctx, tx, token); err != nil {
				return enrollment.Token{}, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return enrollment.Token{}, err
		}
		return token, nil
	case enrollment.TokenStatusConsumed:
		return enrollment.Token{}, enrollment.ErrTokenConsumed
	case enrollment.TokenStatusExpired:
		return enrollment.Token{}, enrollment.ErrTokenExpired
	case enrollment.TokenStatusRevoked:
		return enrollment.Token{}, enrollment.ErrTokenRevoked
	default:
		return enrollment.Token{}, enrollment.ErrTokenConflict
	}
}

func loadTokenByHashForUpdate(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, tokenHash string) (enrollment.Token, error) {
	row := tx.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, status, expires_at, consumed_at, revoked_at, created_at, updated_at
		 FROM enrollment_tokens
		 WHERE tenant_id = $1 AND token_hash = $2
		 FOR UPDATE`,
		tenantID, tokenHash,
	)
	return scanToken(row)
}

func loadTokenByIDForUpdate(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, id string) (enrollment.Token, error) {
	row := tx.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, status, expires_at, consumed_at, revoked_at, created_at, updated_at
		 FROM enrollment_tokens
		 WHERE tenant_id = $1 AND id = $2
		 FOR UPDATE`,
		tenantID, id,
	)
	return scanToken(row)
}

func deviceExistsByTenantAndDeviceIDForUpdate(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, deviceID string) (bool, error) {
	row := tx.QueryRow(ctx,
		`SELECT 1
		 FROM devices
		 WHERE tenant_id = $1 AND device_id = $2
		 FOR UPDATE`,
		tenantID, deviceID,
	)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func updateToken(ctx context.Context, tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, token enrollment.Token) error {
	_, err := tx.Exec(ctx,
		`UPDATE enrollment_tokens
		 SET status = $3, consumed_at = $4, revoked_at = $5, updated_at = $6
		 WHERE tenant_id = $1 AND id = $2`,
		token.TenantID, token.ID, token.Status, token.ConsumedAt, token.RevokedAt, token.UpdatedAt,
	)
	return err
}

func scanToken(scanner rowScanner) (enrollment.Token, error) {
	var rec enrollment.Token
	var consumedAt pgtype.Timestamptz
	var revokedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Status, &rec.ExpiresAt, &consumedAt, &revokedAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return enrollment.Token{}, enrollment.ErrTokenNotFound
		}
		return enrollment.Token{}, err
	}
	if consumedAt.Valid {
		rec.ConsumedAt = &consumedAt.Time
	}
	if revokedAt.Valid {
		rec.RevokedAt = &revokedAt.Time
	}
	return rec, nil
}
