package policypg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	policy "xmdm/server/internal/policy"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScanPolicyAllowsNullKioskAppPackage(t *testing.T) {
	rec, err := scanPolicy(fakePolicyRow{kioskAppPackage: nil})
	if err != nil {
		t.Fatalf("scan policy: %v", err)
	}
	if rec.KioskAppPackage != "" {
		t.Fatalf("expected empty kiosk app package, got %q", rec.KioskAppPackage)
	}
}

func TestScanPolicyReadsKioskAppPackage(t *testing.T) {
	rec, err := scanPolicy(fakePolicyRow{kioskAppPackage: "com.android.chrome"})
	if err != nil {
		t.Fatalf("scan policy: %v", err)
	}
	if rec.KioskAppPackage != "com.android.chrome" {
		t.Fatalf("expected kiosk app package to round-trip, got %q", rec.KioskAppPackage)
	}
}

func TestStoreAssignsPolicyVersion(t *testing.T) {
	pool := openTestPool(t)
	t.Cleanup(pool.Close)
	resetPolicyTestDB(t, pool)

	store := New(pool)
	store.SetNow(func() time.Time { return time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC) })

	created, err := store.CreatePolicy(context.Background(), bootstrap.SeedTenantID, policy.PolicyUpsert{
		Name:         "policy-a",
		KioskMode:    false,
		Restrictions: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected initial policy version 1, got %d", created.Version)
	}

	updated, err := store.UpdatePolicy(context.Background(), bootstrap.SeedTenantID, created.ID, policy.PolicyUpsert{
		Name:         "policy-a-updated",
		KioskMode:    true,
		Restrictions: json.RawMessage(`{"kioskExitPasscode":"1234"}`),
	})
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected updated policy version 2, got %d", updated.Version)
	}
	if updated.Name != "policy-a-updated" {
		t.Fatalf("unexpected updated policy: %#v", updated)
	}
}

func TestStoreTogglesPolicyApps(t *testing.T) {
	pool := openTestPool(t)
	t.Cleanup(pool.Close)
	resetPolicyTestDB(t, pool)

	store := New(pool)
	store.SetNow(func() time.Time { return time.Date(2026, 5, 13, 12, 10, 0, 0, time.UTC) })

	policyRec, err := store.CreatePolicy(context.Background(), bootstrap.SeedTenantID, policy.PolicyUpsert{
		Name:         "policy-apps",
		KioskMode:    false,
		Restrictions: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO apps (id, tenant_id, package_name, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', $5, $5)
	`, "app-1", bootstrap.SeedTenantID, "com.example.catalog", "Catalog", time.Date(2026, 5, 13, 12, 11, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("insert app: %v", err)
	}

	created, err := store.AddPolicyApp(context.Background(), bootstrap.SeedTenantID, policyRec.ID, "app-1")
	if err != nil {
		t.Fatalf("add policy app: %v", err)
	}
	if created.Status != policy.StatusActive {
		t.Fatalf("expected active assignment, got %#v", created)
	}

	items, err := store.ListPolicyApps(context.Background(), bootstrap.SeedTenantID, policyRec.ID)
	if err != nil {
		t.Fatalf("list policy apps: %v", err)
	}
	if len(items) != 1 || items[0].Status != policy.StatusActive {
		t.Fatalf("expected one active policy app, got %#v", items)
	}

	if err := store.RemovePolicyApp(context.Background(), bootstrap.SeedTenantID, policyRec.ID, "app-1"); err != nil {
		t.Fatalf("remove policy app: %v", err)
	}
	items, err = store.ListPolicyApps(context.Background(), bootstrap.SeedTenantID, policyRec.ID)
	if err != nil {
		t.Fatalf("list policy apps after remove: %v", err)
	}
	if len(items) != 1 || items[0].Status != "disabled" {
		t.Fatalf("expected disabled policy app, got %#v", items)
	}

	reactivated, err := store.AddPolicyApp(context.Background(), bootstrap.SeedTenantID, policyRec.ID, "app-1")
	if err != nil {
		t.Fatalf("reactivate policy app: %v", err)
	}
	if reactivated.Status != policy.StatusActive {
		t.Fatalf("expected active assignment after re-add, got %#v", reactivated)
	}
	items, err = store.ListPolicyApps(context.Background(), bootstrap.SeedTenantID, policyRec.ID)
	if err != nil {
		t.Fatalf("list policy apps after re-add: %v", err)
	}
	if len(items) != 1 || items[0].Status != policy.StatusActive {
		t.Fatalf("expected reactivated policy app, got %#v", items)
	}
}

type fakePolicyRow struct {
	kioskAppPackage any
}

func (r fakePolicyRow) Scan(dest ...any) error {
	values := []any{
		"id-1",
		"tenant-1",
		"policy-1",
		1,
		true,
		r.kioskAppPackage,
		json.RawMessage(`{"blockPackages":["com.android.chrome"]}`),
		"active",
		time.Unix(120, 0).UTC(),
		time.Unix(123, 0).UTC(),
		pgtype.Timestamptz{},
	}
	for i, d := range dest {
		if err := assignPolicyValue(d, values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assignPolicyValue(dest any, value any) error {
	switch d := dest.(type) {
	case *string:
		if value == nil {
			*d = ""
			return nil
		}
		*d = value.(string)
	case *int:
		*d = value.(int)
	case *bool:
		*d = value.(bool)
	case *pgtype.Text:
		if value == nil {
			*d = pgtype.Text{}
			return nil
		}
		*d = pgtype.Text{String: value.(string), Valid: true}
	case *json.RawMessage:
		if value == nil {
			*d = nil
			return nil
		}
		raw := value.(json.RawMessage)
		*d = append(json.RawMessage(nil), raw...)
	case *time.Time:
		*d = value.(time.Time)
	case *pgtype.Timestamptz:
		*d = value.(pgtype.Timestamptz)
	default:
		return fmt.Errorf("unsupported destination type %T", dest)
	}
	return nil
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

func resetPolicyTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE enrollment_tokens, device_telemetry, device_info, device_logs, audit_events, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ($1, $2, 'active');
	`, bootstrap.SeedTenantID, bootstrap.SeedTenantName)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
