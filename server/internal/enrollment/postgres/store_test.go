package enrollmentpg

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
)

func TestStoreIssueValidateConsumeRevokeAndExpire(t *testing.T) {
	pool := openTestPool(t)
	t.Cleanup(pool.Close)
	resetEnrollmentTestDB(t, pool)

	store := New(pool)
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return now })
	deviceID := "device-" + uuid.NewString()

	issued, err := store.IssueToken(context.Background(), bootstrap.SeedTenantID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if issued.Secret == "" {
		t.Fatal("issue token returned empty secret")
	}
	if issued.Status != enrollment.TokenStatusIssued {
		t.Fatalf("unexpected status: %s", issued.Status)
	}

	validated, err := store.ValidateToken(context.Background(), bootstrap.SeedTenantID, issued.Secret)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if validated.ID != issued.ID || validated.Status != enrollment.TokenStatusIssued {
		t.Fatalf("unexpected validated token: %#v", validated)
	}

	consumed, err := store.ConsumeToken(context.Background(), bootstrap.SeedTenantID, issued.Secret)
	if err != nil {
		t.Fatalf("consume token: %v", err)
	}
	if consumed.Status != enrollment.TokenStatusConsumed {
		t.Fatalf("unexpected consume status: %s", consumed.Status)
	}
	if _, err := store.ValidateToken(context.Background(), bootstrap.SeedTenantID, issued.Secret); !errors.Is(err, enrollment.ErrTokenConsumed) {
		t.Fatalf("expected consumed validation error, got %v", err)
	}

	revokedIssued, err := store.IssueToken(context.Background(), bootstrap.SeedTenantID, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("issue revoke token: %v", err)
	}
	revoked, err := store.RevokeToken(context.Background(), bootstrap.SeedTenantID, revokedIssued.ID)
	if err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if revoked.Status != enrollment.TokenStatusRevoked {
		t.Fatalf("unexpected revoke status: %s", revoked.Status)
	}
	if _, err := store.ConsumeToken(context.Background(), bootstrap.SeedTenantID, revokedIssued.Secret); !errors.Is(err, enrollment.ErrTokenRevoked) {
		t.Fatalf("expected revoked consume error, got %v", err)
	}

	bindIssued, err := store.IssueToken(context.Background(), bootstrap.SeedTenantID, now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("issue bind token: %v", err)
	}
	bound, err := store.BindDevice(context.Background(), bootstrap.SeedTenantID, bindIssued.Secret, deviceID)
	if err != nil {
		t.Fatalf("bind device: %v", err)
	}
	if bound.DeviceID != deviceID || bound.DeviceSecret == "" || bound.Status != device.StatusEnrolled {
		t.Fatalf("unexpected bound device: %#v", bound)
	}
	if _, err := store.ConsumeToken(context.Background(), bootstrap.SeedTenantID, bindIssued.Secret); !errors.Is(err, enrollment.ErrTokenConsumed) {
		t.Fatalf("expected consumed token after bind, got %v", err)
	}

	dupIssued, err := store.IssueToken(context.Background(), bootstrap.SeedTenantID, now.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("issue duplicate token: %v", err)
	}
	if _, err := store.BindDevice(context.Background(), bootstrap.SeedTenantID, dupIssued.Secret, deviceID); !errors.Is(err, enrollment.ErrDeviceConflict) {
		t.Fatalf("expected duplicate device conflict, got %v", err)
	}

	expiringIssued, err := store.IssueToken(context.Background(), bootstrap.SeedTenantID, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("issue expiring token: %v", err)
	}
	count, err := store.ExpireTokens(context.Background(), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("expire tokens: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one expired token, got %d", count)
	}
	if _, err := store.ValidateToken(context.Background(), bootstrap.SeedTenantID, expiringIssued.Secret); !errors.Is(err, enrollment.ErrTokenExpired) {
		t.Fatalf("expected expired validation error, got %v", err)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
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

func resetEnrollmentTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE enrollment_tokens, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
