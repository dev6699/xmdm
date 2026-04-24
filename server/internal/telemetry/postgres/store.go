package telemetrypg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/telemetry"
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

func (s *Store) Upload(ctx context.Context, tenantID, deviceID, secret string, req telemetry.UploadRequest) (telemetry.Record, error) {
	if tenantID == "" || deviceID == "" || secret == "" {
		return telemetry.Record{}, httpx.ErrInvalidInput
	}
	payload, err := buildPayload(req)
	if err != nil {
		return telemetry.Record{}, err
	}
	now := s.now()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return telemetry.Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	deviceRow, err := loadDeviceForSecret(ctx, tx, tenantID, deviceID, secret)
	if err != nil {
		return telemetry.Record{}, err
	}
	if req.ObservedAt.IsZero() {
		req.ObservedAt = now
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO device_telemetry (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 RETURNING id::text, tenant_id::text, device_id::text, observed_at, payload_json`,
		uuid.NewString(), tenantID, deviceRow.ID, req.ObservedAt, payload, now,
	)
	rec, err := scanTelemetry(row)
	if err != nil {
		return telemetry.Record{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE devices SET updated_at = $2 WHERE id = $1`, deviceRow.ID, now); err != nil {
		return telemetry.Record{}, err
	}
	if deviceRow.Status == device.StatusEnrolled {
		if _, err := tx.Exec(ctx, `UPDATE devices SET status = $2, updated_at = $3 WHERE id = $1`, deviceRow.ID, device.StatusActive, now); err != nil {
			return telemetry.Record{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return telemetry.Record{}, err
	}
	return rec, nil
}

type deviceRow struct {
	ID         string
	SecretHash string
	Status     string
}

func loadDeviceForSecret(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, deviceID, secret string) (deviceRow, error) {
	row := tx.QueryRow(ctx,
		`SELECT id::text, secret_hash, status
		 FROM devices
		 WHERE tenant_id = $1 AND device_id = $2 AND deleted_at IS NULL
		 FOR UPDATE`,
		tenantID, deviceID,
	)
	var rec deviceRow
	if err := row.Scan(&rec.ID, &rec.SecretHash, &rec.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return deviceRow{}, telemetry.ErrDeviceNotFound
		}
		return deviceRow{}, err
	}
	if rec.Status == device.StatusRetired || rec.Status == device.StatusWiped {
		return deviceRow{}, telemetry.ErrDeviceUnauthorized
	}
	if rec.SecretHash != enrollment.HashToken(secret) {
		return deviceRow{}, telemetry.ErrDeviceUnauthorized
	}
	return rec, nil
}

func buildPayload(req telemetry.UploadRequest) (map[string]any, error) {
	if req.Heartbeat == nil && req.Battery == nil && req.Network == nil && req.Location == nil && req.AppState == nil {
		return nil, telemetry.ErrTelemetryInvalid
	}
	payload := make(map[string]any)
	if req.Heartbeat != nil {
		payload["heartbeat"] = req.Heartbeat
	}
	if req.Battery != nil {
		payload["battery"] = req.Battery
	}
	if req.Network != nil {
		payload["network"] = req.Network
	}
	if req.Location != nil {
		payload["location"] = req.Location
	}
	if req.AppState != nil {
		payload["appState"] = req.AppState
	}
	return payload, nil
}

func scanTelemetry(scanner rowScanner) (telemetry.Record, error) {
	var rec telemetry.Record
	var payload []byte
	var observedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &observedAt, &payload); err != nil {
		return telemetry.Record{}, err
	}
	if observedAt.Valid {
		rec.ObservedAt = observedAt.Time
	}
	if err := json.Unmarshal(payload, &rec.Payload); err != nil {
		return telemetry.Record{}, err
	}
	return rec, nil
}
