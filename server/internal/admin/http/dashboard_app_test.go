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
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	auditStore := &recordingAuditStore{}
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
	existingApp     apps.App
	createdApp      apps.App
	updatedApp      apps.App
	retiredApp      apps.App
	createdVersion  apps.Version
	latestVersion   apps.Version
	getByPackageErr error
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
