package telemetrypg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/telemetry"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

	rec, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, deviceSecret, telemetry.UploadRequest{
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

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM devices WHERE tenant_id = $1 AND id = $2`, bootstrap.SeedTenantID, seededDeviceID).Scan(&status); err != nil {
		t.Fatalf("load device status: %v", err)
	}
	if status != device.StatusActive {
		t.Fatalf("expected active status after telemetry, got %q", status)
	}

	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, uuid.NewString(), deviceSecret, telemetry.UploadRequest{Heartbeat: map[string]any{"online": true}}); !errors.Is(err, telemetry.ErrDeviceNotFound) {
		t.Fatalf("expected device not found, got %v", err)
	}

	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "wrong-secret", telemetry.UploadRequest{Heartbeat: map[string]any{"online": true}}); !errors.Is(err, telemetry.ErrDeviceUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestStoreListLatestByDeviceIDs(t *testing.T) {
	pool := openTelemetryTestPool(t)
	t.Cleanup(pool.Close)
	resetTelemetryTestDB(t, pool)

	first := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	second := first.Add(15 * time.Minute)
	store := New(pool)
	store.SetNow(func() time.Time { return first })
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "device-secret", telemetry.UploadRequest{
		Heartbeat: map[string]any{"online": true},
		Battery:   map[string]any{"level": 41},
	}); err != nil {
		t.Fatalf("upload first telemetry: %v", err)
	}
	store.SetNow(func() time.Time { return second })
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, seededDeviceID, "device-secret", telemetry.UploadRequest{
		Heartbeat: map[string]any{"online": true},
		Battery:   map[string]any{"level": 73},
	}); err != nil {
		t.Fatalf("upload second telemetry: %v", err)
	}

	latest, err := store.ListLatestByDeviceIDs(context.Background(), bootstrap.SeedTenantID, []string{seededDeviceID, "", seededDeviceID})
	if err != nil {
		t.Fatalf("list latest telemetry: %v", err)
	}
	if len(latest) != 1 {
		t.Fatalf("expected one latest record, got %#v", latest)
	}
	rec, ok := latest[seededDeviceID]
	if !ok {
		t.Fatalf("expected latest telemetry for device")
	}
	if got := rec.Payload["battery"].(map[string]any)["level"]; got != float64(73) {
		t.Fatalf("expected latest battery level 73, got %#v", got)
	}
	if !rec.ObservedAt.Equal(second) {
		t.Fatalf("expected latest observed at %s, got %s", second, rec.ObservedAt)
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
