package devicepg

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	device "xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/group"
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
	groupIDs := uniqueStrings(req.GroupIDs)
	deviceID := uuid.NewString()
	var policyID any
	if req.PolicyID != "" {
		policyID = req.PolicyID
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return device.Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.ensureActiveGroups(ctx, tx, tenantID, groupIDs); err != nil {
		return device.Device{}, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO devices (id, tenant_id, display_name, secret_hash, policy_id, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text`,
		deviceID, tenantID, req.Name, req.SecretHash, policyID, now,
	)
	var createdID string
	if err := row.Scan(&createdID); err != nil {
		return device.Device{}, err
	}
	for _, groupID := range groupIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO device_groups (tenant_id, device_id, group_id, created_at)
			 VALUES ($1, $2, $3, $4)`,
			tenantID, createdID, groupID, now,
		); err != nil {
			return device.Device{}, err
		}
	}
	rec, err := s.loadDevice(ctx, tx, tenantID, createdID)
	if err != nil {
		return device.Device{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return device.Device{}, err
	}
	return rec, nil
}

func (s *Store) ensureActiveGroups(ctx context.Context, tx pgx.Tx, tenantID string, groupIDs []string) error {
	for _, groupID := range groupIDs {
		if groupID == "" {
			continue
		}
		var found string
		if err := tx.QueryRow(ctx,
			`SELECT id::text
			 FROM groups
			 WHERE tenant_id = $1 AND id = $2 AND status = $3`,
			tenantID, groupID, group.StatusActive,
		).Scan(&found); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.ErrNotFound
			}
			return err
		}
	}
	return nil
}

func (s *Store) ListDevices(ctx context.Context, tenantID string) ([]device.Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.id::text, d.tenant_id::text, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id::text, d.bootstrap_extras::text,
		        COALESCE(array_agg(DISTINCT dg.group_id::text ORDER BY dg.group_id::text) FILTER (WHERE dg.group_id IS NOT NULL), '{}'::text[]) AS group_ids
		 FROM devices d
		 LEFT JOIN device_groups dg ON dg.tenant_id = d.tenant_id AND dg.device_id = d.id
		 WHERE d.tenant_id = $1
		 GROUP BY d.id, d.tenant_id, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id, d.bootstrap_extras
		 ORDER BY d.created_at`,
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
	if req.Name == "" {
		return device.Device{}, httpx.ErrInvalidInput
	}
	now := s.now()
	var policyID any
	if req.PolicyID != "" {
		policyID = req.PolicyID
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return device.Device{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	groupIDs := uniqueStrings(req.GroupIDs)
	if err := s.ensureActiveGroups(ctx, tx, tenantID, groupIDs); err != nil {
		return device.Device{}, err
	}
	row := tx.QueryRow(ctx,
		`UPDATE devices
		 SET display_name = $3, secret_hash = COALESCE(NULLIF($4, ''), secret_hash), policy_id = $5, updated_at = $6
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text`,
		tenantID, id, req.Name, req.SecretHash, policyID, now,
	)
	var updatedID string
	if err := row.Scan(&updatedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return device.Device{}, httpx.ErrNotFound
		}
		return device.Device{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM device_groups WHERE tenant_id = $1 AND device_id = $2`, tenantID, updatedID); err != nil {
		return device.Device{}, err
	}
	for _, groupID := range groupIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO device_groups (tenant_id, device_id, group_id, created_at)
			 VALUES ($1, $2, $3, $4)`,
			tenantID, updatedID, groupID, now,
		); err != nil {
			return device.Device{}, err
		}
	}
	rec, err := s.loadDevice(ctx, tx, tenantID, updatedID)
	if err != nil {
		return device.Device{}, err
	}
	if err := tx.Commit(ctx); err != nil {
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
		 RETURNING id::text, tenant_id::text, display_name, status, created_at, updated_at, deleted_at, policy_id::text, bootstrap_extras::text,
		           COALESCE((SELECT array_agg(DISTINCT dg.group_id::text ORDER BY dg.group_id::text)
		                    FROM device_groups dg
		                    WHERE dg.tenant_id = devices.tenant_id AND dg.device_id = devices.id), '{}'::text[])`,
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
		if err := s.provisioner.DisableDevice(context.Background(), rec.ID); err != nil {
			log.Printf("mqtt dynsec revoke for %s failed: %v", rec.ID, err)
		}
	}
	return rec, nil
}

func (s *Store) Authenticate(ctx context.Context, tenantID, deviceID, secret string) (device.Device, error) {
	if tenantID == "" || deviceID == "" || secret == "" {
		return device.Device{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`SELECT d.id::text, d.tenant_id::text, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id::text, d.bootstrap_extras::text,
		        COALESCE(array_agg(DISTINCT dg.group_id::text ORDER BY dg.group_id::text) FILTER (WHERE dg.group_id IS NOT NULL), '{}'::text[]) AS group_ids
		 FROM devices d
		 LEFT JOIN device_groups dg ON dg.tenant_id = d.tenant_id AND dg.device_id = d.id
		 WHERE d.tenant_id = $1 AND d.id = $2 AND d.secret_hash = $3
		 GROUP BY d.id, d.tenant_id, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id, d.bootstrap_extras`,
		tenantID, deviceID, enrollment.HashToken(secret),
	)
	return scanAuthenticatedDevice(row)
}

func scanDevice(scanner rowScanner) (device.Device, error) {
	var rec device.Device
	var createdAt pgtype.Timestamptz
	var deletedAt pgtype.Timestamptz
	var policyID pgtype.Text
	var bootstrapExtrasJSON pgtype.Text
	var groupIDs []string
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &createdAt, &rec.UpdatedAt, &deletedAt, &policyID, &bootstrapExtrasJSON, &groupIDs); err != nil {
		return device.Device{}, err
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if policyID.Valid {
		value := policyID.String
		rec.PolicyID = &value
	}
	if bootstrapExtrasJSON.Valid && strings.TrimSpace(bootstrapExtrasJSON.String) != "" {
		var extras map[string]any
		if err := json.Unmarshal([]byte(bootstrapExtrasJSON.String), &extras); err != nil {
			return device.Device{}, err
		}
		rec.BootstrapExtras = extras
	}
	rec.GroupIDs = groupIDs
	return rec, nil
}

func (s *Store) loadDevice(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenantID, id string) (device.Device, error) {
	row := querier.QueryRow(ctx,
		`SELECT d.id::text, d.tenant_id::text, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id::text, d.bootstrap_extras::text,
		        COALESCE(array_agg(DISTINCT dg.group_id::text ORDER BY dg.group_id::text) FILTER (WHERE dg.group_id IS NOT NULL), '{}'::text[]) AS group_ids
		 FROM devices d
		 LEFT JOIN device_groups dg ON dg.tenant_id = d.tenant_id AND dg.device_id = d.id
		 WHERE d.tenant_id = $1 AND d.id = $2
		 GROUP BY d.id, d.tenant_id, d.display_name, d.status, d.created_at, d.updated_at, d.deleted_at, d.policy_id, d.bootstrap_extras`,
		tenantID, id,
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

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	slices.Sort(out)
	return out
}
