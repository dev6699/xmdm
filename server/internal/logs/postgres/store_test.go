package logspg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/logs"
)

const seededDeviceID = "33333333-3333-3333-3333-333333333333"

func TestStoreUploadAndSearchLogs(t *testing.T) {
	pool := openLogsTestPool(t)
	t.Cleanup(pool.Close)
	resetLogsTestDB(t, pool)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := New(pool)
	store.SetNow(func() time.Time { return now })

	records, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "device-123", "device-secret", logs.UploadRequest{
		ObservedAt: now,
		Entries: []logs.EntryUpsert{
			{
				Source:  "launcher",
				Level:   "info",
				Message: "first log",
			},
			{
				ObservedAt: now.Add(time.Minute),
				Source:     "launcher",
				Level:      "warn",
				Message:    "second log",
				Payload:    map[string]any{"code": 42},
			},
		},
	})
	if err != nil {
		t.Fatalf("upload logs: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two records, got %#v", records)
	}
	if records[0].DeviceID != "device-123" || records[0].TenantID != bootstrap.SeedTenantID {
		t.Fatalf("unexpected record: %#v", records[0])
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM devices WHERE tenant_id = $1 AND device_id = $2`, bootstrap.SeedTenantID, "device-123").Scan(&status); err != nil {
		t.Fatalf("load device status: %v", err)
	}
	if status != device.StatusActive {
		t.Fatalf("expected active status after logs upload, got %q", status)
	}

	found, err := store.Search(context.Background(), bootstrap.SeedTenantID, logs.SearchFilter{
		DeviceID: "device-123",
		Query:    "log",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("search logs: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected two search results, got %#v", found)
	}
	if found[0].Message != "second log" || found[1].Message != "first log" {
		t.Fatalf("unexpected search order: %#v", found)
	}
}

func TestStoreUploadLogsValidationAndAuth(t *testing.T) {
	pool := openLogsTestPool(t)
	t.Cleanup(pool.Close)
	resetLogsTestDB(t, pool)

	store := New(pool)
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "device-123", "device-secret", logs.UploadRequest{}); !errors.Is(err, logs.ErrLogsInvalid) {
		t.Fatalf("expected invalid logs payload, got %v", err)
	}
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "missing-device", "device-secret", logs.UploadRequest{Entries: []logs.EntryUpsert{{Message: "hello"}}}); !errors.Is(err, logs.ErrDeviceNotFound) {
		t.Fatalf("expected device not found, got %v", err)
	}
	if _, err := store.Upload(context.Background(), bootstrap.SeedTenantID, "device-123", "wrong-secret", logs.UploadRequest{Entries: []logs.EntryUpsert{{Message: "hello"}}}); !errors.Is(err, logs.ErrDeviceUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func openLogsTestPool(t *testing.T) *pgxpool.Pool {
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

func resetLogsTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE device_logs, device_telemetry, enrollment_tokens, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
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
