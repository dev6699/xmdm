package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/bootstrap"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("XMDM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("XMDM_TEST_POSTGRES_DSN must be set for server tests")
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

func resetTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE device_telemetry, enrollment_tokens, files, artifacts, app_versions, apps, audit_events, device_groups, devices, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
		INSERT INTO roles (id, tenant_id, name, permissions, status)
		VALUES (
			'`+bootstrap.SeedAdminRoleID+`',
			'`+bootstrap.SeedTenantID+`',
			'`+bootstrap.SeedAdminRoleName+`',
			'`+mustJSON(t, bootstrap.SeedAdminPermissions)+`'::jsonb,
			'active'
		);
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}
