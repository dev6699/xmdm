package appspg

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStoreAppMethods(t *testing.T) {
	pool := openAppTestPool(t)
	t.Cleanup(pool.Close)
	resetAppTestDB(t, pool)

	store := New(pool)
	baseTime := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return baseTime })

	mustExec(t, pool, `
		INSERT INTO tenants (id, name, status)
		VALUES ($1, $2, 'active');
	`, bootstrap.SeedTenantID, bootstrap.SeedTenantName)

	store.SetNow(func() time.Time { return baseTime.Add(1 * time.Minute) })
	appOne, err := store.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: "com.example.app",
		Name:        "Example App",
	})
	if err != nil {
		t.Fatalf("create app one: %v", err)
	}
	if appOne.SystemOwned {
		t.Fatalf("expected regular app to be non-system-owned, got %#v", appOne)
	}

	store.SetNow(func() time.Time { return baseTime.Add(2 * time.Minute) })
	systemOwnedApp, err := store.UpsertSystemOwnedApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: bootstrap.SeedAgentAppPackage,
		Name:        bootstrap.SeedAgentAppName,
	})
	if err != nil {
		t.Fatalf("create system-owned app: %v", err)
	}
	if !systemOwnedApp.SystemOwned {
		t.Fatalf("expected system-owned app, got %#v", systemOwnedApp)
	}

	store.SetNow(func() time.Time { return baseTime.Add(3 * time.Minute) })
	appTwo, err := store.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: "com.example.viewer",
		Name:        "Viewer",
	})
	if err != nil {
		t.Fatalf("create app two: %v", err)
	}

	items, err := store.ListApps(context.Background(), bootstrap.SeedTenantID, paginationDefault())
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if got, want := appIDs(items), []string{appTwo.ID, systemOwnedApp.ID, appOne.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected app order: got=%v want=%v", got, want)
	}

	stats, err := store.GetOverviewStats(context.Background(), bootstrap.SeedTenantID)
	if err != nil {
		t.Fatalf("get overview stats: %v", err)
	}
	if stats.Total != 3 || stats.Active != 3 {
		t.Fatalf("unexpected stats: %#v", stats)
	}

	found, err := store.GetApp(context.Background(), bootstrap.SeedTenantID, appOne.ID)
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if found.ID != appOne.ID || found.PackageName != appOne.PackageName {
		t.Fatalf("unexpected app: %#v", found)
	}

	foundByPackage, err := store.GetAppByPackageName(context.Background(), bootstrap.SeedTenantID, bootstrap.SeedAgentAppPackage)
	if err != nil {
		t.Fatalf("get app by package: %v", err)
	}
	if foundByPackage.ID != systemOwnedApp.ID || !foundByPackage.SystemOwned {
		t.Fatalf("unexpected app by package: %#v", foundByPackage)
	}

	t.Run("duplicate create app returns conflict", func(t *testing.T) {
		if _, err := store.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
			PackageName: appOne.PackageName,
			Name:        "Duplicate",
		}); !errors.Is(err, httpx.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("update app enforces conflicts and allows regular edits", func(t *testing.T) {
		if _, err := store.UpdateApp(context.Background(), bootstrap.SeedTenantID, appOne.ID, apps.AppUpsert{
			PackageName: appTwo.PackageName,
			Name:        "Conflict",
		}); !errors.Is(err, httpx.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}

		store.SetNow(func() time.Time { return baseTime.Add(4 * time.Minute) })
		updated, err := store.UpdateApp(context.Background(), bootstrap.SeedTenantID, appOne.ID, apps.AppUpsert{
			PackageName: "com.example.app.updated",
			Name:        "Example App Updated",
		})
		if err != nil {
			t.Fatalf("update app: %v", err)
		}
		if updated.PackageName != "com.example.app.updated" || updated.Name != "Example App Updated" {
			t.Fatalf("unexpected updated app: %#v", updated)
		}
	})

	t.Run("system-owned app is locked", func(t *testing.T) {
		if _, err := store.UpdateApp(context.Background(), bootstrap.SeedTenantID, systemOwnedApp.ID, apps.AppUpsert{
			PackageName: bootstrap.SeedAgentAppPackage,
			Name:        "Changed",
		}); !errors.Is(err, httpx.ErrForbidden) {
			t.Fatalf("expected forbidden update, got %v", err)
		}
		if _, err := store.RetireApp(context.Background(), bootstrap.SeedTenantID, systemOwnedApp.ID); !errors.Is(err, httpx.ErrForbidden) {
			t.Fatalf("expected forbidden retire, got %v", err)
		}
	})

	t.Run("retire app updates status", func(t *testing.T) {
		store.SetNow(func() time.Time { return baseTime.Add(5 * time.Minute) })
		retired, err := store.RetireApp(context.Background(), bootstrap.SeedTenantID, appOne.ID)
		if err != nil {
			t.Fatalf("retire app: %v", err)
		}
		if retired.Status != apps.StatusRetired {
			t.Fatalf("unexpected retired app: %#v", retired)
		}
		stats, err := store.GetOverviewStats(context.Background(), bootstrap.SeedTenantID)
		if err != nil {
			t.Fatalf("get overview stats after retire: %v", err)
		}
		if stats.Total != 3 || stats.Active != 2 {
			t.Fatalf("unexpected stats after retire: %#v", stats)
		}
	})

	t.Run("missing app lookups return not found", func(t *testing.T) {
		missingAppID := uuid.NewString()
		if _, err := store.GetApp(context.Background(), bootstrap.SeedTenantID, missingAppID); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		if _, err := store.GetAppByPackageName(context.Background(), bootstrap.SeedTenantID, "com.example.missing"); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		if _, err := store.UpdateApp(context.Background(), bootstrap.SeedTenantID, missingAppID, apps.AppUpsert{PackageName: "com.example.missing", Name: "Missing"}); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		if _, err := store.RetireApp(context.Background(), bootstrap.SeedTenantID, missingAppID); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestStoreVersionMethods(t *testing.T) {
	pool := openAppTestPool(t)
	t.Cleanup(pool.Close)
	resetAppTestDB(t, pool)

	store := New(pool)
	baseTime := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	store.SetNow(func() time.Time { return baseTime })

	mustExec(t, pool, `
		INSERT INTO tenants (id, name, status)
		VALUES ($1, $2, 'active');
	`, bootstrap.SeedTenantID, bootstrap.SeedTenantName)

	appRec, err := store.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{
		PackageName: "com.example.app",
		Name:        "Example App",
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	artifactOneID := insertArtifact(t, pool, baseTime.Add(1*time.Minute), "artifacts/app-1.apk", "sha256-app-1", 1001, "application/vnd.android.package-archive")
	artifactTwoID := insertArtifact(t, pool, baseTime.Add(2*time.Minute), "artifacts/app-2.apk", "sha256-app-2", 1002, "application/vnd.android.package-archive")
	artifactThreeID := insertArtifact(t, pool, baseTime.Add(3*time.Minute), "artifacts/app-3.apk", "sha256-app-3", 1003, "application/vnd.android.package-archive")
	_ = artifactThreeID

	store.SetNow(func() time.Time { return baseTime.Add(1 * time.Minute) })
	versionOne, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
		VersionName: "1.0.0",
		VersionCode: 100,
		ArtifactID:  &artifactOneID,
		Checksum:    "sha256-app-1",
		Publish:     true,
	})
	if err != nil {
		t.Fatalf("create version one: %v", err)
	}
	if versionOne.PublishedAt == nil {
		t.Fatalf("expected published_at to be set, got %#v", versionOne)
	}

	store.SetNow(func() time.Time { return baseTime.Add(2 * time.Minute) })
	versionTwo, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
		VersionName: "1.0.1",
		VersionCode: 100,
		ArtifactID:  &artifactTwoID,
		Checksum:    "sha256-app-2",
		Publish:     true,
	})
	if err != nil {
		t.Fatalf("create version two: %v", err)
	}

	store.SetNow(func() time.Time { return baseTime.Add(3 * time.Minute) })
	versionThree, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
		VersionName: "1.1.0",
		VersionCode: 101,
		ArtifactID:  &artifactThreeID,
		Checksum:    "sha256-app-3",
		Publish:     true,
	})
	if err != nil {
		t.Fatalf("create version three: %v", err)
	}

	versions, err := store.ListVersions(context.Background(), bootstrap.SeedTenantID, appRec.ID, paginationDefault())
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if got, want := versionIDs(versions), []string{versionThree.ID, versionTwo.ID, versionOne.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected version order: got=%v want=%v", got, want)
	}

	byCode, err := store.GetVersionByCode(context.Background(), bootstrap.SeedTenantID, appRec.ID, 100)
	if err != nil {
		t.Fatalf("get version by code: %v", err)
	}
	if byCode.ID != versionTwo.ID {
		t.Fatalf("expected latest version for code 100, got %#v", byCode)
	}

	latest, err := store.GetLatestPublishedVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID)
	if err != nil {
		t.Fatalf("get latest published version: %v", err)
	}
	if latest.ID != versionThree.ID {
		t.Fatalf("expected latest published version, got %#v", latest)
	}

	fullVersion, err := store.GetVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, versionOne.ID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if fullVersion.Artifact == nil || fullVersion.Artifact.StorageKey != "artifacts/app-1.apk" {
		t.Fatalf("expected joined artifact on version, got %#v", fullVersion)
	}

	t.Run("create version rejects checksum mismatch", func(t *testing.T) {
		store.SetNow(func() time.Time { return baseTime.Add(4 * time.Minute) })
		_, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
			VersionName: "1.2.0",
			VersionCode: 102,
			ArtifactID:  &artifactOneID,
			Checksum:    "sha256-wrong",
			Publish:     true,
		})
		if !errors.Is(err, httpx.ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	})

	t.Run("duplicate version returns conflict", func(t *testing.T) {
		store.SetNow(func() time.Time { return baseTime.Add(5 * time.Minute) })
		_, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
			VersionName: "1.0.0",
			VersionCode: 100,
			ArtifactID:  &artifactOneID,
			Checksum:    "sha256-app-1",
			Publish:     true,
		})
		if !errors.Is(err, httpx.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("version lookups return not found", func(t *testing.T) {
		if _, err := store.GetVersionByCode(context.Background(), bootstrap.SeedTenantID, appRec.ID, 999); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		if _, err := store.GetLatestPublishedVersion(context.Background(), bootstrap.SeedTenantID, uuid.NewString()); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		if _, err := store.GetVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, uuid.NewString()); !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("create version rejects retired app", func(t *testing.T) {
		store.SetNow(func() time.Time { return baseTime.Add(6 * time.Minute) })
		_, err := store.RetireApp(context.Background(), bootstrap.SeedTenantID, appRec.ID)
		if err != nil {
			t.Fatalf("retire app before not found check: %v", err)
		}
		_, err = store.CreateVersion(context.Background(), bootstrap.SeedTenantID, appRec.ID, apps.VersionUpsert{
			VersionName: "2.0.0",
			VersionCode: 200,
			ArtifactID:  &artifactThreeID,
			Checksum:    "sha256-app-3",
			Publish:     true,
		})
		if !errors.Is(err, httpx.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestStoreValidationErrors(t *testing.T) {
	pool := openAppTestPool(t)
	t.Cleanup(pool.Close)
	resetAppTestDB(t, pool)

	store := New(pool)
	mustExec(t, pool, `
		INSERT INTO tenants (id, name, status)
		VALUES ($1, $2, 'active');
	`, bootstrap.SeedTenantID, bootstrap.SeedTenantName)

	if _, err := store.UpsertSystemOwnedApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{}); !errors.Is(err, httpx.ErrInvalidInput) {
		t.Fatalf("expected invalid input for system-owned app, got %v", err)
	}
	if _, err := store.CreateApp(context.Background(), bootstrap.SeedTenantID, apps.AppUpsert{PackageName: "com.example.app"}); !errors.Is(err, httpx.ErrInvalidInput) {
		t.Fatalf("expected invalid input for create app, got %v", err)
	}
	if _, err := store.CreateVersion(context.Background(), bootstrap.SeedTenantID, "missing-app", apps.VersionUpsert{}); !errors.Is(err, httpx.ErrInvalidInput) {
		t.Fatalf("expected invalid input for create version, got %v", err)
	}
}

func paginationDefault() pagination.Params {
	return pagination.Params{Limit: pagination.DefaultLimit}
}

func appIDs(items []apps.App) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func versionIDs(items []apps.Version) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func insertArtifact(t *testing.T, pool *pgxpool.Pool, updatedAt time.Time, storageKey, checksum string, sizeBytes int64, mimeType string) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, pool, `
		INSERT INTO artifacts (id, tenant_id, storage_key, checksum, size_bytes, mime_type, status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7);
	`, id, bootstrap.SeedTenantID, storageKey, checksum, sizeBytes, mimeType, updatedAt)
	return id
}

func openAppTestPool(t *testing.T) *pgxpool.Pool {
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

func resetAppTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE app_versions, apps, audit_events, devices, policies, groups, users, roles, tenants, files, artifacts RESTART IDENTITY CASCADE;
	`)
	if err != nil {
		t.Fatalf("reset postgres: %v", err)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}
