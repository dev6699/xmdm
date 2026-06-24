package adminhttp

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func TestAppMutationsRecordAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite})
	auditStore := &recordingAuditStore{}
	publishedAt := time.Date(2026, 6, 24, 10, 30, 0, 0, time.UTC)
	appStore := &recordingAppStore{
		existingApp: apps.App{
			RecordBase:  apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusActive},
			PackageName: "com.example.app",
			Name:        "Example App",
		},
		createdApp: apps.App{
			RecordBase:  apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusActive},
			PackageName: "com.example.app",
			Name:        "Example App",
		},
		updatedApp: apps.App{
			RecordBase:  apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusActive},
			PackageName: "com.example.app",
			Name:        "Example App Updated",
		},
		retiredApp: apps.App{
			RecordBase:  apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusRetired},
			PackageName: "com.example.app",
			Name:        "Example App",
		},
		createdVersion: apps.Version{
			ID:          "version-1",
			TenantID:    "tenant-1",
			AppID:       "app-1",
			Status:      apps.VersionStatusPublished,
			VersionName: "1.0.0",
			VersionCode: 100,
			Checksum:    "sha256-app-abc",
		},
		latestVersion: apps.Version{
			ID:          "version-1",
			TenantID:    "tenant-1",
			AppID:       "app-1",
			Status:      apps.VersionStatusPublished,
			VersionName: "1.0.0",
			VersionCode: 100,
			Checksum:    "sha256-app-abc",
			PublishedAt: &publishedAt,
			CreatedAt:   publishedAt,
		},
	}
	fileStore := &recordingFileStore{
		file: files.File{
			RecordBase: files.RecordBase{ID: "file-1", Status: files.StatusActive},
			Name:       "com.example.app-100.apk",
			ArtifactID: "artifact-1",
			Checksum:   "sha256-app-abc",
			MimeType:   "application/vnd.android.package-archive",
		},
	}
	artifactStore := &recordingArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Apps:      appStore,
		Files:     fileStore,
		Artifacts: artifactStore,
		Audit:     auditStore,
		TenantID:  "tenant-1",
	})

	postForm := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create app", func(t *testing.T) {
		auditStore.records = nil
		appStore.getByPackageErr = httpx.ErrNotFound
		rr := postMultipart(t, mux, session.ID, "/admin/apps/create", map[string]string{
			"packageName": "com.example.app",
			"name":        "Example App",
			"versionCode": "100",
			"csrfToken":   "token",
		}, "file", "example.apk", []byte("apk-content"))
		assertRedirect(t, rr, "/admin/apps/app-1?ok=managed+app+created")
		assertAuditRecord(t, auditStore, "create", "apps", "app-1")
	})

	t.Run("update app", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/apps/app-1/update", "packageName=com.example.app&name=Example+App+Updated&csrfToken=token")
		assertRedirect(t, rr, "/admin/apps/app-1?ok=app+updated")
		assertAuditRecord(t, auditStore, "update", "apps", "app-1")
	})

	t.Run("retire app", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/apps/app-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/apps/app-1?ok=app+retired")
		assertAuditRecord(t, auditStore, "retire", "apps", "app-1")
	})

	t.Run("create version", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/apps/app-1/versions/create", "versionName=1.0.0&versionCode=100&checksum=sha256-app-abc&csrfToken=token")
		assertRedirect(t, rr, "/admin/apps/app-1?ok=app+version+created")
		assertAuditRecord(t, auditStore, "create", "app_versions", "version-1")
	})

	t.Run("publish version without retyping app metadata", func(t *testing.T) {
		auditStore.records = nil
		appStore.versionByCodeErr = httpx.ErrNotFound
		rr := postMultipart(t, mux, session.ID, "/admin/apps/app-1/versions/publish", map[string]string{
			"versionName": "1.1.0",
			"versionCode": "101",
			"csrfToken":   "token",
		}, "file", "example-101.apk", []byte("apk-content-101"))
		assertRedirect(t, rr, "/admin/apps/app-1?ok=app+version+published")
		assertAuditRecord(t, auditStore, "create", "app_versions", "version-1")
		if appStore.versionByCodeErr != httpx.ErrNotFound {
			t.Fatalf("expected version lookup override to remain set")
		}
	})

	t.Run("publish version does not short-circuit uploaded draft", func(t *testing.T) {
		auditStore.records = nil
		appStore.versionByCodeErr = nil
		appStore.versionByCodeVersion = apps.Version{
			ID:          "version-uploaded",
			TenantID:    "tenant-1",
			AppID:       "app-1",
			Status:      apps.VersionStatusUploaded,
			VersionName: "1.2.0",
			VersionCode: 102,
			Checksum:    "sha256-draft",
		}
		appStore.createdVersion = apps.Version{
			ID:          "version-published",
			TenantID:    "tenant-1",
			AppID:       "app-1",
			Status:      apps.VersionStatusPublished,
			VersionName: "1.2.0",
			VersionCode: 102,
			Checksum:    "sha256-app-abc",
		}
		rr := postMultipart(t, mux, session.ID, "/admin/apps/app-1/versions/publish", map[string]string{
			"versionName": "1.2.0",
			"versionCode": "102",
			"csrfToken":   "token",
		}, "file", "example-102.apk", []byte("apk-content-102"))
		assertRedirect(t, rr, "/admin/apps/app-1?ok=app+version+published")
		assertAuditRecord(t, auditStore, "create", "app_versions", "version-published")
		if appStore.versionByCodeVersion.Status != apps.VersionStatusUploaded {
			t.Fatalf("expected uploaded fixture to remain uploaded, got %#v", appStore.versionByCodeVersion)
		}
	})

	t.Run("app list shows system owned indicator", func(t *testing.T) {
		appStore.existingApp.SystemOwned = true
		req := httptest.NewRequest(http.MethodGet, "/admin/apps", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if strings.Contains(body, ">ID<") || strings.Contains(body, ">app-1<") {
			t.Fatalf("expected id column to be removed from list, got %s", body)
		}
		if !strings.Contains(body, "System owned") {
			t.Fatalf("expected system owned column, got %s", body)
		}
		if !strings.Contains(body, "✓") {
			t.Fatalf("expected system owned tick, got %s", body)
		}
		if strings.Contains(body, "system-owned") {
			t.Fatalf("expected plain tick instead of badge text, got %s", body)
		}
		if !strings.Contains(body, "24 Jun 2026") {
			t.Fatalf("expected publish date in latest published column, got %s", body)
		}
	})

	t.Run("seeded app is publish-only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/apps/app-1", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, "Seeded app") {
			t.Fatalf("expected seeded app callout, got %s", body)
		}
		if !strings.Contains(body, "Publish new version") {
			t.Fatalf("expected publish form, got %s", body)
		}
		if strings.Contains(body, "Update app") || strings.Contains(body, "Retire app") {
			t.Fatalf("expected locked app to hide metadata forms, got %s", body)
		}
	})
}

func postMultipart(t *testing.T, mux *http.ServeMux, sessionID, path string, fields map[string]string, fileField, fileName string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

type recordingAppStore struct {
	existingApp          apps.App
	createdApp           apps.App
	updatedApp           apps.App
	retiredApp           apps.App
	createdVersion       apps.Version
	latestVersion        apps.Version
	getByPackageErr      error
	versionByCodeErr     error
	versionByCodeVersion apps.Version
}

func (s *recordingAppStore) ListApps(context.Context, string, pagination.Params) ([]apps.App, error) {
	return []apps.App{s.existingApp}, nil
}

func (s *recordingAppStore) GetOverviewStats(context.Context, string) (apps.OverviewStats, error) {
	return apps.OverviewStats{}, nil
}

func (s *recordingAppStore) GetApp(context.Context, string, string) (apps.App, error) {
	return s.existingApp, nil
}

func (s *recordingAppStore) GetAppByPackageName(context.Context, string, string) (apps.App, error) {
	if s.getByPackageErr != nil {
		return apps.App{}, s.getByPackageErr
	}
	return s.existingApp, nil
}
func (s *recordingAppStore) UpsertSystemOwnedApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return s.createdApp, nil
}

func (s *recordingAppStore) CreateApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return s.createdApp, nil
}

func (s *recordingAppStore) UpdateApp(context.Context, string, string, apps.AppUpsert) (apps.App, error) {
	return s.updatedApp, nil
}

func (s *recordingAppStore) RetireApp(context.Context, string, string) (apps.App, error) {
	return s.retiredApp, nil
}

func (s *recordingAppStore) ListVersions(context.Context, string, string, pagination.Params) ([]apps.Version, error) {
	return []apps.Version{s.createdVersion}, nil
}

func (s *recordingAppStore) GetVersionByCode(context.Context, string, string, int64) (apps.Version, error) {
	if s.versionByCodeErr != nil {
		return apps.Version{}, s.versionByCodeErr
	}
	if s.versionByCodeVersion.ID != "" {
		return s.versionByCodeVersion, nil
	}
	return s.createdVersion, nil
}

func (s *recordingAppStore) GetLatestPublishedVersion(context.Context, string, string) (apps.Version, error) {
	return s.latestVersion, nil
}

func (s *recordingAppStore) GetVersion(context.Context, string, string, string) (apps.Version, error) {
	return s.createdVersion, nil
}

func (s *recordingAppStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return s.createdVersion, nil
}
