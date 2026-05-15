package managedfilespg

import (
	"context"
	"os"
	"testing"
	"time"

	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/managedfiles"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStoreCreateManagedFileReplacesExistingPath(t *testing.T) {
	pool := openTestPool(t)
	t.Cleanup(pool.Close)
	resetManagedFilesTestDB(t, pool)

	store := New(pool)
	firstTime := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(10 * time.Minute)
	store.SetNow(func() time.Time { return firstTime })

	firstFileID := insertSourceFile(t, pool, "managed-source-one", "artifacts/managed-source-one", "text/plain", "sha256-first", 5)
	first, err := store.CreateManagedFile(context.Background(), bootstrap.SeedTenantID, managedfiles.ManagedFileUpsert{FileID: firstFileID, Path: "/sdcard/xmdm/config.txt", ReplaceVariables: true})
	if err != nil {
		t.Fatalf("create first managed file: %v", err)
	}
	if first.FileID != firstFileID || !first.ReplaceVariables {
		t.Fatalf("unexpected first managed file: %#v", first)
	}

	store.SetNow(func() time.Time { return secondTime })
	secondFileID := insertSourceFile(t, pool, "managed-source-two", "artifacts/managed-source-two", "text/plain", "sha256-second", 6)
	replaced, err := store.CreateManagedFile(context.Background(), bootstrap.SeedTenantID, managedfiles.ManagedFileUpsert{FileID: secondFileID, Path: "/sdcard/xmdm/config.txt", ReplaceVariables: false})
	if err != nil {
		t.Fatalf("replace managed file: %v", err)
	}
	if replaced.ID != first.ID {
		t.Fatalf("expected same binding id after replace, got first=%s replaced=%s", first.ID, replaced.ID)
	}
	if replaced.FileID != secondFileID {
		t.Fatalf("expected replacement file id, got %#v", replaced)
	}
	if replaced.ReplaceVariables {
		t.Fatalf("expected replacement flags to update, got %#v", replaced)
	}
	if !replaced.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("expected created_at to remain stable, got first=%v replaced=%v", first.CreatedAt, replaced.CreatedAt)
	}

	items, err := store.ListManagedFiles(context.Background(), bootstrap.SeedTenantID)
	if err != nil {
		t.Fatalf("list managed files: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one binding after replace, got %#v", items)
	}
	if items[0].FileID != secondFileID {
		t.Fatalf("expected list to show replacement file, got %#v", items[0])
	}
}

func insertSourceFile(t *testing.T, pool *pgxpool.Pool, name, storageKey, mimeType, checksum string, sizeBytes int64) string {
	t.Helper()
	ctx := context.Background()
	artifactID := uuid.NewString()
	fileID := uuid.NewString()
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, tenant_id, storage_key, checksum, size_bytes, mime_type, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
	`, artifactID, bootstrap.SeedTenantID, storageKey, checksum, sizeBytes, mimeType, now)
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO files (id, tenant_id, name, artifact_id, checksum, mime_type, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
	`, fileID, bootstrap.SeedTenantID, name, artifactID, checksum, mimeType, now)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	return fileID
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

func resetManagedFilesTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE managed_files, files, artifacts, audit_events, enrollment_tokens, device_groups, devices, policy_managed_files, policy_certificates, policy_apps, policies, groups, users, roles, tenants RESTART IDENTITY CASCADE;
		INSERT INTO tenants (id, name, status)
		VALUES ('`+bootstrap.SeedTenantID+`', '`+bootstrap.SeedTenantName+`', 'active');
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}
