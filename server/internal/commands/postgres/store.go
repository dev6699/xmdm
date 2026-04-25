package commandspg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"xmdm/server/internal/commands"
	device "xmdm/server/internal/device"
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

func (s *Store) Enqueue(ctx context.Context, tenantID string, req commands.Upsert) ([]commands.Command, error) {
	if tenantID == "" || req.Type == "" {
		return nil, httpx.ErrInvalidInput
	}
	now := s.now()
	payload := map[string]any{}
	if req.Payload != nil {
		payload = req.Payload
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var expiresAt pgtype.Timestamptz
	if req.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: req.ExpiresAt.UTC(), Valid: true}
	}
	targets, err := s.resolveTargets(ctx, tenantID, req.Target)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	commandsOut := make([]commands.Command, 0, len(targets))
	for _, deviceID := range targets {
		row := tx.QueryRow(ctx,
			`INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, expires_at, acked_at, result_json, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, NULL, '{}'::jsonb, $8, $8)
			 RETURNING id::text, tenant_id::text, device_id, type, payload_json, status, expires_at, acked_at, result_json, created_at, updated_at`,
			uuid.NewString(), tenantID, deviceID, req.Type, string(rawPayload), commands.StatusQueued, expiresAt, now,
		)
		rec, err := scanCommand(row)
		if err != nil {
			return nil, err
		}
		commandsOut = append(commandsOut, rec)
	}
	if len(commandsOut) == 0 {
		return nil, httpx.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return commandsOut, nil
}

func (s *Store) ListPending(ctx context.Context, tenantID, deviceID string) ([]commands.Command, error) {
	if tenantID == "" || deviceID == "" {
		return nil, httpx.ErrInvalidInput
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, device_id, type, payload_json, status, expires_at, acked_at, result_json, created_at, updated_at
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

func (s *Store) Acknowledge(ctx context.Context, tenantID, deviceID, commandID string, req commands.Ack) (commands.Command, error) {
	if tenantID == "" || deviceID == "" || commandID == "" || req.Status == "" {
		return commands.Command{}, httpx.ErrInvalidInput
	}
	switch req.Status {
	case commands.StatusAcked, commands.StatusFailed:
	default:
		return commands.Command{}, httpx.ErrInvalidInput
	}
	now := s.now()
	result := map[string]any{
		"status": req.Status,
	}
	if req.Message != "" {
		result["message"] = req.Message
	}
	if req.Details != nil {
		result["details"] = req.Details
	}
	rawResult, err := json.Marshal(result)
	if err != nil {
		return commands.Command{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return commands.Command{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`UPDATE commands
		 SET status = $5, acked_at = $6, result_json = $7::jsonb, updated_at = $6
		 WHERE tenant_id = $1 AND device_id = $2 AND id = $3
		   AND status IN ($8, $9)
		 RETURNING id::text, tenant_id::text, device_id, type, payload_json, status, expires_at, acked_at, result_json, created_at, updated_at`,
		tenantID, deviceID, commandID, req.Status, now, string(rawResult), commands.StatusQueued, commands.StatusSent,
	)
	rec, err := scanCommand(row)
	if err != nil {
		return commands.Command{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return commands.Command{}, err
	}
	return rec, nil
}

func (s *Store) resolveTargets(ctx context.Context, tenantID string, target commands.Target) ([]string, error) {
	switch target.Type {
	case "", commands.TargetDevice:
		if target.DeviceID == "" {
			return nil, httpx.ErrInvalidInput
		}
		return s.listTargetDeviceIDs(ctx,
			`SELECT device_id
			 FROM devices
			 WHERE tenant_id = $1 AND device_id = $2 AND status <> $3 AND status <> $4
			 ORDER BY created_at, id`,
			tenantID, target.DeviceID, device.StatusRetired, device.StatusWiped,
		)
	case commands.TargetGroup:
		if target.GroupID == "" {
			return nil, httpx.ErrInvalidInput
		}
		return s.listTargetDeviceIDs(ctx,
			`SELECT d.device_id
			 FROM device_groups dg
			 JOIN devices d ON d.tenant_id = dg.tenant_id AND d.id = dg.device_id
			 WHERE dg.tenant_id = $1 AND dg.group_id = $2 AND d.status <> $3 AND d.status <> $4
			 ORDER BY d.created_at, d.id`,
			tenantID, target.GroupID, device.StatusRetired, device.StatusWiped,
		)
	case commands.TargetBroadcast:
		return s.listTargetDeviceIDs(ctx,
			`SELECT device_id
			 FROM devices
			 WHERE tenant_id = $1 AND status <> $2 AND status <> $3
			 ORDER BY created_at, id`,
			tenantID, device.StatusRetired, device.StatusWiped,
		)
	default:
		return nil, fmt.Errorf("invalid command target type")
	}
}

func (s *Store) listTargetDeviceIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			return nil, err
		}
		items = append(items, deviceID)
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
	var ackedAt pgtype.Timestamptz
	var resultJSON []byte
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &rec.Type, &payloadJSON, &rec.Status, &expiresAt, &ackedAt, &resultJSON, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
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
	if ackedAt.Valid {
		value := ackedAt.Time
		rec.AckedAt = &value
	}
	if len(resultJSON) > 0 {
		if err := json.Unmarshal(resultJSON, &rec.Result); err != nil {
			return commands.Command{}, err
		}
	}
	return rec, nil
}
