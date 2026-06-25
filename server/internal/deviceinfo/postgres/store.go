package deviceinfopg

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
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
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

func (s *Store) Upload(ctx context.Context, tenantID, deviceID, secret string, req deviceinfo.UploadRequest) ([]deviceinfo.Record, error) {
	if tenantID == "" || deviceID == "" || secret == "" {
		return nil, httpx.ErrInvalidInput
	}
	if len(req.Payload) == 0 {
		return nil, deviceinfo.ErrDeviceInfoInvalid
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
	record, err := insertDeviceInfo(ctx, tx, tenantID, deviceID, deviceRow.ID, now, req)
	if err != nil {
		return nil, err
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
	return []deviceinfo.Record{record}, nil
}

func (s *Store) Search(ctx context.Context, tenantID string, filter deviceinfo.SearchFilter) ([]deviceinfo.Record, error) {
	if tenantID == "" {
		return nil, httpx.ErrInvalidInput
	}
	page := filter.Pagination
	if page.Limit == 0 && page.Offset == 0 && (filter.Limit != 0 || filter.Offset != 0) {
		page = pagination.Params{Limit: filter.Limit, Offset: filter.Offset}
	}
	page = pagination.Normalize(page, 100, 500)
	args := []any{tenantID}
	conditions := []string{"i.tenant_id = $1"}
	if filter.DeviceID != "" {
		args = append(args, filter.DeviceID)
		conditions = append(conditions, fmt.Sprintf("d.id = $%d", len(args)))
	}
	if filter.Query != "" {
		args = append(args, filter.Query)
		conditions = append(conditions, fmt.Sprintf("i.payload_json::text ILIKE '%%' || $%d || '%%'", len(args)))
	}
	if filter.Since != nil {
		args = append(args, filter.Since.UTC())
		conditions = append(conditions, fmt.Sprintf("i.observed_at >= $%d", len(args)))
	}
	if filter.Until != nil {
		args = append(args, filter.Until.UTC())
		conditions = append(conditions, fmt.Sprintf("i.observed_at <= $%d", len(args)))
	}
	args = append(args, page.Limit, page.Offset)

	query := strings.Builder{}
	query.WriteString(`SELECT i.id::text, i.tenant_id::text, d.id::text, i.observed_at, i.payload_json
		FROM device_info i`)
	query.WriteString(` JOIN devices d ON d.id = i.device_id`)
	query.WriteString(` WHERE `)
	query.WriteString(strings.Join(conditions, " AND "))
	query.WriteString(fmt.Sprintf(` ORDER BY i.observed_at DESC, i.created_at DESC, i.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)))

	rows, err := s.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]deviceinfo.Record, 0)
	for rows.Next() {
		rec, err := scanSearchDeviceInfo(rows)
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

func (s *Store) ListLatestByDeviceIDs(ctx context.Context, tenantID string, deviceIDs []string) (map[string]deviceinfo.Record, error) {
	if tenantID == "" {
		return nil, httpx.ErrInvalidInput
	}
	ids := make([]string, 0, len(deviceIDs))
	seen := map[string]struct{}{}
	for _, id := range deviceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return map[string]deviceinfo.Record{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (d.id) i.id::text, i.tenant_id::text, d.id::text, i.observed_at, i.payload_json
		 FROM device_info i
		 JOIN devices d ON d.id = i.device_id
		 WHERE i.tenant_id = $1 AND d.id = ANY($2::uuid[])
		 ORDER BY d.id, i.observed_at DESC, i.created_at DESC, i.id DESC`,
		tenantID, ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make(map[string]deviceinfo.Record, len(ids))
	for rows.Next() {
		rec, err := scanSearchDeviceInfo(rows)
		if err != nil {
			return nil, err
		}
		items[rec.DeviceID] = rec
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
			return deviceRow{}, deviceinfo.ErrDeviceNotFound
		}
		return deviceRow{}, err
	}
	if rec.Status == device.StatusRetired || rec.Status == device.StatusWiped {
		return deviceRow{}, deviceinfo.ErrDeviceUnauthorized
	}
	if rec.SecretHash != enrollment.HashToken(secret) {
		return deviceRow{}, deviceinfo.ErrDeviceUnauthorized
	}
	return rec, nil
}

func insertDeviceInfo(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, deviceID, deviceRowID string, now time.Time, req deviceinfo.UploadRequest) (deviceinfo.Record, error) {
	observedAt := req.ObservedAt
	if observedAt.IsZero() {
		observedAt = now
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return deviceinfo.Record{}, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO device_info (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)
		 RETURNING id::text, tenant_id::text, observed_at, payload_json`,
		uuid.NewString(), tenantID, deviceRowID, observedAt, payloadJSON, now,
	)
	rec, err := scanUploadDeviceInfo(row)
	if err != nil {
		return deviceinfo.Record{}, err
	}
	rec.DeviceID = deviceID
	return rec, nil
}

func scanUploadDeviceInfo(scanner rowScanner) (deviceinfo.Record, error) {
	var rec deviceinfo.Record
	var payload []byte
	var observedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &observedAt, &payload); err != nil {
		return deviceinfo.Record{}, err
	}
	if observedAt.Valid {
		rec.ObservedAt = observedAt.Time
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &rec.Payload); err != nil {
			return deviceinfo.Record{}, err
		}
	} else {
		rec.Payload = map[string]any{}
	}
	return rec, nil
}

func scanSearchDeviceInfo(scanner rowScanner) (deviceinfo.Record, error) {
	var rec deviceinfo.Record
	var payload []byte
	var observedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.DeviceID, &observedAt, &payload); err != nil {
		return deviceinfo.Record{}, err
	}
	if observedAt.Valid {
		rec.ObservedAt = observedAt.Time
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &rec.Payload); err != nil {
			return deviceinfo.Record{}, err
		}
	} else {
		rec.Payload = map[string]any{}
	}
	return rec, nil
}
