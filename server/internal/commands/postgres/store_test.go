package commandspg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScanCommandDecodesPayloadAndExpiry(t *testing.T) {
	expiry := time.Date(2026, 4, 25, 15, 0, 0, 0, time.UTC)
	rec, err := scanCommand(fakeRowScanner{
		scan: func(dest ...any) error {
			*(dest[0].(*string)) = "cmd-1"
			*(dest[1].(*string)) = "tenant-1"
			*(dest[2].(*string)) = "device-1"
			*(dest[3].(*string)) = "reboot"
			*(dest[4].(*[]byte)) = []byte(`{"force":true}`)
			*(dest[5].(*string)) = commands.StatusQueued
			*(dest[6].(*pgtype.Timestamptz)) = pgtype.Timestamptz{Time: expiry, Valid: true}
			*(dest[7].(*time.Time)) = time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
			*(dest[8].(*time.Time)) = time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("scan command: %v", err)
	}
	if rec.ID != "cmd-1" || rec.Type != "reboot" || rec.Status != commands.StatusQueued {
		t.Fatalf("unexpected command: %#v", rec)
	}
	if rec.ExpiresAt == nil || !rec.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected expiry: %#v", rec.ExpiresAt)
	}
	if got := rec.Payload["force"]; got != true {
		t.Fatalf("unexpected payload: %#v", rec.Payload)
	}
}

func TestScanCommandMapsNoRowsToNotFound(t *testing.T) {
	_, err := scanCommand(fakeRowScanner{})
	if !errors.Is(err, httpx.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestEnqueueFansOutToGroupAndBroadcast(t *testing.T) {
	pool := openCommandsTestPool(t)
	t.Cleanup(pool.Close)
	resetCommandsTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })

	var groupID string
	if err := pool.QueryRow(context.Background(), `INSERT INTO groups (id, tenant_id, name, updated_at) VALUES (gen_random_uuid(), $1, $2, $3) RETURNING id::text`, bootstrap.SeedTenantID, "field", now).Scan(&groupID); err != nil {
		t.Fatalf("create group: %v", err)
	}
	deviceIDs := []string{"device-a", "device-b", "device-c"}
	for _, deviceID := range deviceIDs {
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO devices (id, tenant_id, device_id, secret_hash, status, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
			 RETURNING id::text`,
			bootstrap.SeedTenantID, deviceID, enrollment.HashToken("secret-"+deviceID), "active", now,
		).Scan(new(string)); err != nil {
			t.Fatalf("create device %s: %v", deviceID, err)
		}
		if deviceID != "device-c" {
			if _, err := pool.Exec(context.Background(),
				`INSERT INTO device_groups (tenant_id, device_id, group_id, created_at)
				 VALUES ($1, (SELECT id FROM devices WHERE tenant_id = $1 AND device_id = $2), $3, $4)`,
				bootstrap.SeedTenantID, deviceID, groupID, now,
			); err != nil {
				t.Fatalf("assign device %s to group: %v", deviceID, err)
			}
		}
	}

	groupCommands, err := store.Enqueue(context.Background(), bootstrap.SeedTenantID, commands.Upsert{
		Type:   "reboot",
		Target: commands.Target{Type: commands.TargetGroup, GroupID: groupID},
	})
	if err != nil {
		t.Fatalf("enqueue group command: %v", err)
	}
	if len(groupCommands) != 2 {
		t.Fatalf("expected 2 group commands, got %d", len(groupCommands))
	}
	if groupCommands[0].DeviceID != "device-a" || groupCommands[1].DeviceID != "device-b" {
		t.Fatalf("unexpected group fan-out: %#v", groupCommands)
	}

	broadcastCommands, err := store.Enqueue(context.Background(), bootstrap.SeedTenantID, commands.Upsert{
		Type:   "ping",
		Target: commands.Target{Type: commands.TargetBroadcast},
	})
	if err != nil {
		t.Fatalf("enqueue broadcast command: %v", err)
	}
	if len(broadcastCommands) != 3 {
		t.Fatalf("expected 3 broadcast commands, got %d", len(broadcastCommands))
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

func openCommandsTestPool(t *testing.T) *pgxpool.Pool {
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

func resetCommandsTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE commands, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
