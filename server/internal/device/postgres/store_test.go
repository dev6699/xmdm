package devicepg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCanAuthenticateDeviceStatus(t *testing.T) {
	if !canAuthenticateDeviceStatus(device.StatusEnrolled) {
		t.Fatalf("expected enrolled devices to authenticate")
	}
	if !canAuthenticateDeviceStatus(device.StatusActive) {
		t.Fatalf("expected active devices to authenticate")
	}
	if canAuthenticateDeviceStatus(device.StatusRetired) {
		t.Fatalf("expected retired devices to be rejected")
	}
	if canAuthenticateDeviceStatus(device.StatusWiped) {
		t.Fatalf("expected wiped devices to be rejected")
	}
}

func TestScanAuthenticatedDeviceRejectsRetiredAndWiped(t *testing.T) {
	for _, status := range []string{device.StatusRetired, device.StatusWiped} {
		t.Run(status, func(t *testing.T) {
			rec, err := scanAuthenticatedDevice(fakeRowScanner{
				scan: func(dest ...any) error {
					*(dest[0].(*string)) = "device-row-123"
					*(dest[1].(*string)) = "tenant-1"
					*(dest[2].(*string)) = "name"
					*(dest[3].(*string)) = status
					now := time.Now()
					*(dest[4].(*pgtype.Timestamptz)) = pgtype.Timestamptz{Time: now, Valid: true}
					*(dest[5].(*time.Time)) = now
					*(dest[9].(*[]string)) = nil
					return nil
				},
			})
			if !errors.Is(err, httpx.ErrNotFound) {
				t.Fatalf("expected not found, got %v", err)
			}
			if rec.ID != "" {
				t.Fatalf("expected empty record, got %#v", rec)
			}
		})
	}
}

func TestStoreListDevicesByFilterSearchBatteryAndStale(t *testing.T) {
	pool := openDeviceTestPool(t)
	t.Cleanup(pool.Close)
	resetDeviceTestDB(t, pool)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := New(pool)
	store.SetNow(func() time.Time { return now })

	mustExec(t, pool, `
		INSERT INTO tenants (id, name, status)
		VALUES ($1, $2, 'active');
	`, bootstrap.SeedTenantID, bootstrap.SeedTenantName)

	devices := []struct {
		id   string
		name string
	}{
		{id: "00000000-0000-0000-0000-000000000101", name: "alpha-tablet"},
		{id: "00000000-0000-0000-0000-000000000102", name: "beta-tablet"},
		{id: "00000000-0000-0000-0000-000000000103", name: "gamma-kiosk"},
	}
	for _, item := range devices {
		mustExec(t, pool, `
			INSERT INTO devices (id, tenant_id, display_name, secret_hash, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'active', $5, $5);
		`, item.id, bootstrap.SeedTenantID, item.name, enrollment.HashToken("secret-"+item.id), now)
	}
	mustExec(t, pool, `
		INSERT INTO device_info (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $4, $4);
	`, "11111111-1111-1111-1111-111111111101", bootstrap.SeedTenantID, devices[0].id, now.Add(-2*time.Hour), `{"battery":{"level":84}}`)
	mustExec(t, pool, `
		INSERT INTO device_info (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $4, $4);
	`, "11111111-1111-1111-1111-111111111102", bootstrap.SeedTenantID, devices[1].id, now.Add(-2*time.Hour), `{"battery":{"level":"19.5"}}`)
	mustExec(t, pool, `
		INSERT INTO device_info (id, tenant_id, device_id, observed_at, payload_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $4, $4);
	`, "11111111-1111-1111-1111-111111111103", bootstrap.SeedTenantID, devices[2].id, now.Add(-26*time.Hour), `{"battery":{"level":71}}`)

	items, err := store.ListDevicesByFilter(context.Background(), bootstrap.SeedTenantID, pagination.Params{Limit: 10}, device.DeviceListFilter{
		Health:    device.HealthFilterLowBattery,
		NameQuery: "beta",
	})
	if err != nil {
		t.Fatalf("list devices by filter: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one matching device, got %#v", items)
	}
	if items[0].ID != devices[1].id || items[0].Name != devices[1].name {
		t.Fatalf("unexpected filtered device: %#v", items[0])
	}

	searchOnly, err := store.ListDevicesByFilter(context.Background(), bootstrap.SeedTenantID, pagination.Params{Limit: 10}, device.DeviceListFilter{
		NameQuery: "gamma",
	})
	if err != nil {
		t.Fatalf("list devices by name: %v", err)
	}
	if len(searchOnly) != 1 || searchOnly[0].ID != devices[2].id {
		t.Fatalf("expected gamma device search result, got %#v", searchOnly)
	}

	staleOnly, err := store.ListDevicesByFilter(context.Background(), bootstrap.SeedTenantID, pagination.Params{Limit: 10}, device.DeviceListFilter{
		Health: device.HealthFilterStale,
	})
	if err != nil {
		t.Fatalf("list stale devices: %v", err)
	}
	if len(staleOnly) != 1 || staleOnly[0].ID != devices[2].id {
		t.Fatalf("expected gamma stale result, got %#v", staleOnly)
	}
}

func openDeviceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("XMDM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("XMDM_TEST_POSTGRES_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func resetDeviceTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	mustExec(t, pool, `
		TRUNCATE TABLE device_info, device_telemetry, enrollment_tokens, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
	`)
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

type fakeRowScanner struct {
	scan func(...any) error
}

func (f fakeRowScanner) Scan(dest ...any) error {
	if f.scan == nil {
		return pgx.ErrNoRows
	}
	return f.scan(dest...)
}
