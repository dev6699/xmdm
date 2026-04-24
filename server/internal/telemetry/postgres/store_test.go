package telemetrypg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/telemetry"
)

const seededDeviceID = "33333333-3333-3333-3333-333333333333"

func TestStoreUploadTelemetry(t *testing.T) {
	pool := openTelemetryTestPool(t)
	t.Cleanup(pool.Close)
	resetTelemetryTestDB(t, pool)

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	deviceSecret := "device-secret"
	store := New(pool)
	store.SetNow(func() time.Time { return now })

	rec, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "device-123", deviceSecret, telemetry.UploadRequest{
		Heartbeat: map[string]any{"online": true},
		Battery:   map[string]any{"level": 87},
	})
	if err != nil {
		t.Fatalf("upload telemetry: %v", err)
	}
	if rec.DeviceID != seededDeviceID || rec.TenantID != bootstrap.SeedTenantID {
		t.Fatalf("unexpected telemetry record: %#v", rec)
	}
	if rec.Payload["heartbeat"] == nil || rec.Payload["battery"] == nil {
		t.Fatalf("unexpected telemetry payload: %#v", rec.Payload)
	}

	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "missing-device", deviceSecret, telemetry.UploadRequest{Heartbeat: map[string]any{"online": true}}); !errors.Is(err, telemetry.ErrDeviceNotFound) {
		t.Fatalf("expected device not found, got %v", err)
	}

	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "device-123", "wrong-secret", telemetry.UploadRequest{Heartbeat: map[string]any{"online": true}}); !errors.Is(err, telemetry.ErrDeviceUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func openTelemetryTestPool(t *testing.T) *pgxpool.Pool {
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

func resetTelemetryTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE device_telemetry, enrollment_tokens, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
		INSERT INTO devices (id, tenant_id, device_id, secret_hash, status, updated_at)
		VALUES (
			'`+seededDeviceID+`',
			'`+bootstrap.SeedTenantID+`',
			'device-123',
			'`+enrollment.HashToken("device-secret")+`',
			'enrolled',
			now()
		);
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
