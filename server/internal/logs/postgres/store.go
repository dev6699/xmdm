package logspg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
	"xmdm/server/internal/pagination"
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

func (s *Store) Upload(ctx context.Context, tenantID, deviceID, secret string, req logs.UploadRequest) ([]logs.Record, error) {
	if tenantID == "" || deviceID == "" || secret == "" {
		return nil, httpx.ErrInvalidInput
	}
	if len(req.Entries) == 0 {
		return nil, logs.ErrLogsInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	deviceRow, err := loadDeviceForSecret(ctx, tx, tenantID, deviceID, secret)
	if err != nil {
		return nil, err
	}
	now := s.now()
	records := make([]logs.Record, 0, len(req.Entries))
	for _, entry := range req.Entries {
		record, err := insertLog(ctx, tx, tenantID, deviceRow.ID, now, req.ObservedAt, entry)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if _, err := tx.Exec(ctx, `UPDATE devices SET updated_at = $2 WHERE id = $1`, deviceRow.ID, now); err != nil {
		return nil, err
	}
	if deviceRow.Status == device.StatusEnrolled {
		if _, err := tx.Exec(ctx, `UPDATE devices SET status = $2, updated_at = $3 WHERE id = $1`, deviceRow.ID, device.StatusActive, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) Search(ctx context.Context, tenantID string, filter logs.SearchFilter) ([]logs.Record, error) {
	if tenantID == "" {
		return nil, httpx.ErrInvalidInput
	}
	page := filter.Pagination
	if page.Limit == 0 && page.Offset == 0 && (filter.Limit != 0 || filter.Offset != 0) {
		page = pagination.Params{Limit: filter.Limit, Offset: filter.Offset}
	}
	page = pagination.Normalize(page, 100, 500)
	args := []any{tenantID}
	conditions := []string{"l.tenant_id = $1"}
	joinDevices := true
	if filter.DeviceID != "" {
		args = append(args, filter.DeviceID)
		conditions = append(conditions, fmt.Sprintf("d.id = $%d", len(args)))
	}
	if filter.Source != "" {
		args = append(args, filter.Source)
		conditions = append(conditions, fmt.Sprintf("l.source = $%d", len(args)))
	}
	if filter.Level != "" {
		args = append(args, filter.Level)
		conditions = append(conditions, fmt.Sprintf("l.level = $%d", len(args)))
	}
	if filter.Query != "" {
		args = append(args, filter.Query)
		conditions = append(conditions, fmt.Sprintf("(l.message ILIKE '%%' || $%d || '%%' OR l.source ILIKE '%%' || $%d || '%%' OR l.payload_json::text ILIKE '%%' || $%d || '%%')", len(args), len(args), len(args)))
	}
	if filter.Since != nil {
		args = append(args, filter.Since.UTC())
		conditions = append(conditions, fmt.Sprintf("l.observed_at >= $%d", len(args)))
	}
	if filter.Until != nil {
		args = append(args, filter.Until.UTC())
		conditions = append(conditions, fmt.Sprintf("l.observed_at <= $%d", len(args)))
	}
	args = append(args, page.Limit, page.Offset)

	query := strings.Builder{}
	query.WriteString(`SELECT l.id::text, l.tenant_id::text, d.id::text, l.observed_at, l.source, l.level, l.message, l.payload_json
		FROM device_logs l`)
	if joinDevices {
		query.WriteString(` JOIN devices d ON d.id = l.device_id`)
	}
	query.WriteString(` WHERE `)
	query.WriteString(strings.Join(conditions, " AND "))
	query.WriteString(fmt.Sprintf(` ORDER BY l.observed_at DESC, l.created_at DESC, l.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)))

	rows, err := s.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]logs.Record, 0)
	for rows.Next() {
		rec, err := scanSearchLog(rows)
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
		 WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
		 FOR UPDATE`,
		tenantID, deviceID,
	)
	var rec deviceRow
	if err := row.Scan(&rec.ID, &rec.SecretHash, &rec.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return deviceRow{}, logs.ErrDeviceNotFound
		}
		return deviceRow{}, err
	}
	if rec.Status == device.StatusRetired || rec.Status == device.StatusWiped {
		return deviceRow{}, logs.ErrDeviceUnauthorized
	}
	if rec.SecretHash != enrollment.HashToken(secret) {
		return deviceRow{}, logs.ErrDeviceUnauthorized
	}
	return rec, nil
}

func insertLog(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, deviceRowID string, now, fallbackObservedAt time.Time, entry logs.EntryUpsert) (logs.Record, error) {
	if strings.TrimSpace(entry.ID) == "" {
		return logs.Record{}, logs.ErrLogsInvalid
	}
	if _, err := uuid.Parse(strings.TrimSpace(entry.ID)); err != nil {
		return logs.Record{}, logs.ErrLogsInvalid
	}
	if strings.TrimSpace(entry.Message) == "" && len(entry.Payload) == 0 {
		return logs.Record{}, logs.ErrLogsInvalid
	}
	observedAt := entry.ObservedAt
	if observedAt.IsZero() {
		observedAt = fallbackObservedAt
	}
	if observedAt.IsZero() {
		observedAt = now
	}
	payload := entry.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return logs.Record{}, err
	}
	row := tx.QueryRow(ctx,
		`WITH inserted AS (
			INSERT INTO device_logs (id, tenant_id, device_id, observed_at, source, level, message, payload_json, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
			ON CONFLICT (id) DO NOTHING
			RETURNING id::text, tenant_id::text, device_id::text, observed_at, source, level, message, payload_json
		)
		SELECT id, tenant_id, device_id, observed_at, source, level, message, payload_json
		FROM inserted
		UNION ALL
		SELECT id::text, tenant_id::text, device_id::text, observed_at, source, level, message, payload_json
		FROM device_logs
		WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM inserted)`,
		strings.TrimSpace(entry.ID), tenantID, deviceRowID, observedAt, strings.TrimSpace(entry.Source), strings.TrimSpace(entry.Level), strings.TrimSpace(entry.Message), payloadJSON, now,
	)
	rec, err := scanUploadLog(row)
	if err != nil {
		return logs.Record{}, err
	}
	return rec, nil
}

func scanUploadLog(scanner rowScanner) (logs.Record, error) {
	var rec logs.Record
	var payload []byte
	var observedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &observedAt, &rec.Source, &rec.Level, &rec.Message, &payload); err != nil {
		return logs.Record{}, err
	}
	if observedAt.Valid {
		rec.ObservedAt = observedAt.Time
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &rec.Payload); err != nil {
			return logs.Record{}, err
		}
	} else {
		rec.Payload = map[string]any{}
	}
	return rec, nil
}

func scanSearchLog(scanner rowScanner) (logs.Record, error) {
	var rec logs.Record
	var payload []byte
	var observedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &observedAt, &rec.Source, &rec.Level, &rec.Message, &payload); err != nil {
		return logs.Record{}, err
	}
	if observedAt.Valid {
		rec.ObservedAt = observedAt.Time
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &rec.Payload); err != nil {
			return logs.Record{}, err
		}
	} else {
		rec.Payload = map[string]any{}
	}
	return rec, nil
}
