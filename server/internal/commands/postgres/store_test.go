package commandspg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/push"

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
			*(dest[7].(*pgtype.Timestamptz)) = pgtype.Timestamptz{}
			*(dest[8].(*[]byte)) = []byte(`{}`)
			*(dest[9].(*time.Time)) = time.Date(2026, 4, 25, 14, 0, 0, 0, time.UTC)
			*(dest[10].(*time.Time)) = time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
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
	if rec.AckedAt != nil {
		t.Fatalf("expected nil ackedAt, got %#v", rec.AckedAt)
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
	deviceIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, deviceID := range deviceIDs {
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id::text`,
			deviceID, bootstrap.SeedTenantID, enrollment.HashToken("secret-"+deviceID), "active", now,
		).Scan(new(string)); err != nil {
			t.Fatalf("create device %s: %v", deviceID, err)
		}
		if i != len(deviceIDs)-1 {
			if _, err := pool.Exec(context.Background(),
				`INSERT INTO device_groups (tenant_id, device_id, group_id, created_at)
				 VALUES ($1, (SELECT id FROM devices WHERE tenant_id = $1 AND id = $2), $3, $4)`,
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
	if groupCommands[0].DeviceID == "" || groupCommands[1].DeviceID == "" {
		t.Fatalf("expected device IDs, got empty")
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

func TestEnqueuePublishesAndMarksSent(t *testing.T) {
	pool := openCommandsTestPool(t)
	t.Cleanup(pool.Close)
	resetCommandsTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })
	pub := &recordingPublisher{}
	store.SetPublisher(pub)

	var deviceUUID string
	deviceID := uuid.NewString()
	if err := pool.QueryRow(context.Background(), `INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text`,
		deviceID, bootstrap.SeedTenantID, enrollment.HashToken("secret"), "active", now,
	).Scan(&deviceUUID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	created, err := store.Enqueue(context.Background(), bootstrap.SeedTenantID, commands.Upsert{
		Type:   "reboot",
		Target: commands.Target{Type: commands.TargetDevice, DeviceID: deviceID},
		Payload: map[string]any{
			"force": true,
		},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected one command, got %d", len(created))
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].DeviceID != deviceUUID || pub.calls[0].Type != "reboot" {
		t.Fatalf("unexpected publish call: %#v", pub.calls[0])
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM commands WHERE tenant_id = $1 AND device_id = $2`,
		bootstrap.SeedTenantID, deviceUUID).Scan(&status); err != nil {
		t.Fatalf("load command status: %v", err)
	}
	if status != commands.StatusSent {
		t.Fatalf("expected sent status, got %s", status)
	}
}

func TestAcknowledgeUpdatesCommandStatus(t *testing.T) {
	pool := openCommandsTestPool(t)
	t.Cleanup(pool.Close)
	resetCommandsTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })

	var deviceUUID string
	deviceID := uuid.NewString()
	if err := pool.QueryRow(context.Background(), `INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text`,
		deviceID, bootstrap.SeedTenantID, enrollment.HashToken("secret"), "active", now,
	).Scan(&deviceUUID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	var commandID string
	if err := pool.QueryRow(context.Background(), `INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, '{}'::jsonb, $4, $5, $5)
		 RETURNING id::text`,
		bootstrap.SeedTenantID, deviceUUID, "reboot", commands.StatusQueued, now,
	).Scan(&commandID); err != nil {
		t.Fatalf("create command: %v", err)
	}

	rec, err := store.Acknowledge(context.Background(), bootstrap.SeedTenantID, deviceUUID, commandID, commands.Ack{
		Status:  commands.StatusAcked,
		Message: "done",
		Details: map[string]any{"code": 0},
	})
	if err != nil {
		t.Fatalf("ack command: %v", err)
	}
	if rec.Status != commands.StatusAcked || rec.AckedAt == nil {
		t.Fatalf("unexpected ack result: %#v", rec)
	}
	if rec.Result["message"] != "done" {
		t.Fatalf("unexpected result: %#v", rec.Result)
	}
}

func TestExpiredCommandIsNotPendingOrAckable(t *testing.T) {
	pool := openCommandsTestPool(t)
	t.Cleanup(pool.Close)
	resetCommandsTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })

	var deviceUUID string
	deviceID := uuid.NewString()
	if err := pool.QueryRow(context.Background(), `INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text`,
		deviceID, bootstrap.SeedTenantID, enrollment.HashToken("secret"), "active", now,
	).Scan(&deviceUUID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	var commandID string
	expiredAt := now.Add(-time.Minute)
	if err := pool.QueryRow(context.Background(), `INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, expires_at, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, '{}'::jsonb, $4, $5, $6, $6)
		 RETURNING id::text`,
		bootstrap.SeedTenantID, deviceUUID, "ping", commands.StatusQueued, expiredAt, now,
	).Scan(&commandID); err != nil {
		t.Fatalf("create command: %v", err)
	}

	pending, err := store.ListPending(context.Background(), bootstrap.SeedTenantID, deviceUUID, pagination.Params{Limit: pagination.DefaultLimit})
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending commands, got %#v", pending)
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM commands WHERE id = $1`, commandID).Scan(&status); err != nil {
		t.Fatalf("load command status: %v", err)
	}
	if status != commands.StatusExpired {
		t.Fatalf("expected expired status, got %s", status)
	}

	_, err = store.Acknowledge(context.Background(), bootstrap.SeedTenantID, deviceUUID, commandID, commands.Ack{
		Status:  commands.StatusAcked,
		Message: "late",
	})
	if !errors.Is(err, httpx.ErrNotFound) {
		t.Fatalf("expected not found on ack, got %v", err)
	}
}

func TestExpireDueCommandsMarksSentRowsExpired(t *testing.T) {
	pool := openCommandsTestPool(t)
	t.Cleanup(pool.Close)
	resetCommandsTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 25, 16, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })

	var deviceUUID string
	deviceID := uuid.NewString()
	if err := pool.QueryRow(context.Background(), `INSERT INTO devices (id, tenant_id, secret_hash, status, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text`,
		deviceID, bootstrap.SeedTenantID, enrollment.HashToken("secret"), "active", now,
	).Scan(&deviceUUID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO commands (id, tenant_id, device_id, type, payload_json, status, expires_at, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, '{}'::jsonb, $4, $5, $6, $6)`,
		bootstrap.SeedTenantID, deviceUUID, "ping", commands.StatusSent, now.Add(-time.Minute), now,
	); err != nil {
		t.Fatalf("create command: %v", err)
	}

	updated, err := store.ExpireDueCommands(context.Background(), bootstrap.SeedTenantID)
	if err != nil {
		t.Fatalf("expire due commands: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected one expired command, got %d", updated)
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM commands WHERE tenant_id = $1 AND device_id = $2`,
		bootstrap.SeedTenantID, deviceUUID).Scan(&status); err != nil {
		t.Fatalf("load command status: %v", err)
	}
	if status != commands.StatusExpired {
		t.Fatalf("expected expired status, got %s", status)
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

type recordingPublisher struct {
	calls []push.CommandMessage
}

func (p *recordingPublisher) PublishCommand(_ context.Context, deviceID string, message push.CommandMessage) error {
	message.DeviceID = deviceID
	p.calls = append(p.calls, message)
	return nil
}
