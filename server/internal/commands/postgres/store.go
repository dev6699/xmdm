package commandspg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"xmdm/server/internal/commands"
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

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (s *Store) SetNow(now func() time.Time) { s.now = now }

func (s *Store) Enqueue(ctx context.Context, tenantID, deviceID string, req commands.Upsert) (commands.Command, error) {
	if tenantID == "" || deviceID == "" || req.Type == "" {
		return commands.Command{}, httpx.ErrInvalidInput
	}
	now := s.now()
	payload := map[string]any{}
	if req.Payload != nil {
		payload = req.Payload
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return commands.Command{}, err
	}
	var expiresAt pgtype.Timestamptz
	if req.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: req.ExpiresAt.UTC(), Valid: true}
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $8)
		 RETURNING id::text, tenant_id::text, device_id, type, payload_json, status, expires_at, created_at, updated_at`,
		uuid.NewString(), tenantID, deviceID, req.Type, string(rawPayload), commands.StatusQueued, expiresAt, now,
	)
	return scanCommand(row)
}

func (s *Store) ListPending(ctx context.Context, tenantID, deviceID string) ([]commands.Command, error) {
	if tenantID == "" || deviceID == "" {
		return nil, httpx.ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, device_id, type, payload_json, status, expires_at, created_at, updated_at
		 FROM commands
		 WHERE tenant_id = $1
		   AND device_id = $2
		   AND status IN ($3, $4)
		   AND (expires_at IS NULL OR expires_at > $5)
		 ORDER BY created_at, id`,
		tenantID, deviceID, commands.StatusQueued, commands.StatusSent, s.now(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]commands.Command, 0)
	for rows.Next() {
		rec, err := scanCommand(rows)
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

func scanCommand(scanner rowScanner) (commands.Command, error) {
	var rec commands.Command
	var payloadJSON []byte
	var expiresAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &rec.Type, &payloadJSON, &rec.Status, &expiresAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return commands.Command{}, httpx.ErrNotFound
		}
		return commands.Command{}, err
	}
	if len(payloadJSON) > 0 {
		if err := json.Unmarshal(payloadJSON, &rec.Payload); err != nil {
			return commands.Command{}, err
		}
	}
	if expiresAt.Valid {
		value := expiresAt.Time
		rec.ExpiresAt = &value
	}
	return rec, nil
}
