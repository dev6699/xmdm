package deviceinfopg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const seededDeviceID = "33333333-3333-3333-3333-333333333333"

func TestStoreUploadAndSearchDeviceInfo(t *testing.T) {
	pool := openDeviceInfoTestPool(t)
	t.Cleanup(pool.Close)
	resetDeviceInfoTestDB(t, pool)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := New(pool)
	store.SetNow(func() time.Time { return now })

	records, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "device-secret", deviceinfo.UploadRequest{
		ObservedAt: now,
		Payload: map[string]any{
			"model": "Pixel",
		},
	})
	if err != nil {
		t.Fatalf("upload device info: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record, got %#v", records)
	}
	if records[0].DeviceID != seededDeviceID || records[0].TenantID != bootstrap.SeedTenantID {
		t.Fatalf("unexpected record: %#v", records[0])
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM devices WHERE tenant_id = $1 AND id = $2`, bootstrap.SeedTenantID, seededDeviceID).Scan(&status); err != nil {
		t.Fatalf("load device status: %v", err)
	}
	if status != device.StatusActive {
		t.Fatalf("expected active status after device info upload, got %q", status)
	}

	found, err := store.Search(context.Background(), bootstrap.SeedTenantID, deviceinfo.SearchFilter{
		DeviceID: seededDeviceID,
		Query:    "Pixel",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("search device info: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected one search result, got %#v", found)
	}
	if found[0].Payload["model"] != "Pixel" {
		t.Fatalf("unexpected payload: %#v", found[0].Payload)
	}
}

func TestStoreUploadDeviceInfoValidationAndAuth(t *testing.T) {
	pool := openDeviceInfoTestPool(t)
	t.Cleanup(pool.Close)
	resetDeviceInfoTestDB(t, pool)

	store := New(pool)
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "device-secret", deviceinfo.UploadRequest{}); !errors.Is(err, deviceinfo.ErrDeviceInfoInvalid) {
		t.Fatalf("expected invalid device info payload, got %v", err)
	}
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, uuid.NewString(), "device-secret", deviceinfo.UploadRequest{Payload: map[string]any{"model": "Pixel"}}); !errors.Is(err, deviceinfo.ErrDeviceNotFound) {
		t.Fatalf("expected device not found, got %v", err)
	}
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "wrong-secret", deviceinfo.UploadRequest{Payload: map[string]any{"model": "Pixel"}}); !errors.Is(err, deviceinfo.ErrDeviceUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func openDeviceInfoTestPool(t *testing.T) *pgxpool.Pool {
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

func resetDeviceInfoTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE device_info, device_logs, device_telemetry, enrollment_tokens, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
		INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
		VALUES (
			'`+seededDeviceID+`',
			'`+bootstrap.SeedTenantID+`',
			'`+enrollment.HashToken("device-secret")+`',
			'enrolled',
			now()
		);
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
