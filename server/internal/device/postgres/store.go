package devicepg

import (
	"context"
	"errors"
	"log"
	"time"

	device "xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/mqttdynsec"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool        *pgxpool.Pool
	now         func() time.Time
	provisioner mqttdynsec.Provisioner
}

type rowScanner interface {
	Scan(...any) error
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (s *Store) SetProvisioner(provisioner mqttdynsec.Provisioner) {
	s.provisioner = provisioner
}

func (s *Store) SetNow(now func() time.Time) { s.now = now }

func (s *Store) CreateDevice(ctx context.Context, tenantID string, req device.DeviceUpsert) (device.Device, error) {
	if req.Name == "" || req.SecretHash == "" {
		return device.Device{}, httpx.ErrInvalidInput
	}
	now := s.now()
	var policyID any
	if req.PolicyID != "" {
		policyID = req.PolicyID
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO devices (id, tenant_id, device_id, secret_hash, policy_id, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, tenant_id::text, device_id, status, updated_at, deleted_at, policy_id::text`,
		uuid.NewString(), tenantID, req.Name, req.SecretHash, policyID, now,
	)
	return scanDevice(row)
}

func (s *Store) ListDevices(ctx context.Context, tenantID string) ([]device.Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, device_id, status, updated_at, deleted_at, policy_id::text
		 FROM devices
		 WHERE tenant_id = $1
		 ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]device.Device, 0)
	for rows.Next() {
		rec, err := scanDevice(rows)
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

func (s *Store) UpdateDevice(ctx context.Context, tenantID, id string, req device.DeviceUpsert) (device.Device, error) {
	if req.Name == "" || req.SecretHash == "" {
		return device.Device{}, httpx.ErrInvalidInput
	}
	now := s.now()
	var policyID any
	if req.PolicyID != "" {
		policyID = req.PolicyID
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE devices
		 SET device_id = $3, secret_hash = $4, policy_id = $5, updated_at = $6
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, device_id, status, updated_at, deleted_at, policy_id::text`,
		tenantID, id, req.Name, req.SecretHash, policyID, now,
	)
	rec, err := scanDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return device.Device{}, httpx.ErrNotFound
		}
		return device.Device{}, err
	}
	return rec, nil
}

func (s *Store) RetireDevice(ctx context.Context, tenantID, id string) (device.Device, error) {
	now := s.now()
	row := s.pool.QueryRow(ctx,
		`UPDATE devices
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, device_id, status, updated_at, deleted_at, policy_id::text`,
		tenantID, id, device.StatusRetired, now,
	)
	rec, err := scanDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return device.Device{}, httpx.ErrNotFound
		}
		return device.Device{}, err
	}
	if s.provisioner != nil {
		if err := s.provisioner.DisableDevice(context.Background(), rec.Name); err != nil {
			log.Printf("mqtt dynsec revoke for %s failed: %v", rec.Name, err)
		}
	}
	return rec, nil
}

func (s *Store) Authenticate(ctx context.Context, tenantID, deviceID, secret string) (device.Device, error) {
	if tenantID == "" || deviceID == "" || secret == "" {
		return device.Device{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, device_id, status, updated_at, deleted_at, policy_id::text
		 FROM devices
		 WHERE tenant_id = $1 AND device_id = $2 AND secret_hash = $3`,
		tenantID, deviceID, enrollment.HashToken(secret),
	)
	return scanAuthenticatedDevice(row)
}

func scanDevice(scanner rowScanner) (device.Device, error) {
	var rec device.Device
	var deletedAt pgtype.Timestamptz
	var policyID pgtype.Text
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt, &policyID); err != nil {
		return device.Device{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if policyID.Valid {
		value := policyID.String
		rec.PolicyID = &value
	}
	return rec, nil
}

func scanAuthenticatedDevice(scanner rowScanner) (device.Device, error) {
	rec, err := scanDevice(scanner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return device.Device{}, httpx.ErrNotFound
		}
		return device.Device{}, err
	}
	if !canAuthenticateDeviceStatus(rec.Status) {
		return device.Device{}, httpx.ErrNotFound
	}
	return rec, nil
}

func canAuthenticateDeviceStatus(status string) bool {
	switch status {
	case device.StatusRetired, device.StatusWiped:
		return false
	default:
		return true
	}
}
