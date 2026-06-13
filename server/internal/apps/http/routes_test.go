package apphttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	files "xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func TestRegisterDeviceArtifactDownload(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	artifactBytes := []byte("apk-bytes")
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeAppStore{
		version: apps.Version{
			ID:          "version-1",
			TenantID:    "tenant-1",
			AppID:       "app-1",
			Status:      apps.VersionStatusPublished,
			VersionName: "1.0.0",
			VersionCode: 100,
			ArtifactID:  strPtr("artifact-1"),
			Artifact: &files.Artifact{
				RecordBase: files.RecordBase{
					ID:       "artifact-1",
					TenantID: "tenant-1",
					Status:   files.StatusActive,
				},
				StorageKey: "artifacts/app-1.apk",
				Checksum:   "checksum-abc",
				SizeBytes:  int64(len(artifactBytes)),
				MimeType:   "application/vnd.android.package-archive",
			},
		},
	}, &fakeDeviceStore{}, &fakeArtifactStore{content: artifactBytes}, nil, "tenant-1", "com.xmdm.launcher")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/apps/app-1/versions/version-1/artifact", nil)
	req.Header.Set(deviceSecretHeader, "device-secret")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/vnd.android.package-archive" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := res.Header().Get("X-XMDM-Artifact-Checksum"); got != "checksum-abc" {
		t.Fatalf("unexpected checksum header: %q", got)
	}
	if got := res.Header().Get("Content-Length"); got != "9" {
		t.Fatalf("unexpected content length: %q", got)
	}
	if got := res.Header().Get("X-XMDM-Artifact-Size"); got != "9" {
		t.Fatalf("unexpected artifact size header: %q", got)
	}
	if !bytes.Equal(res.Body.Bytes(), artifactBytes) {
		t.Fatalf("unexpected body: %q", res.Body.Bytes())
	}
}

func TestRegisterEnrollmentAgentAPKDownloadUsesLatestPublishedVersion(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	artifactBytes := []byte("latest-agent-apk")
	olderPublishedAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	latestPublishedAt := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeAppStore{
		apps: []apps.App{
			{RecordBase: apps.RecordBase{ID: "app-other", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.other", Name: "Other"},
			{RecordBase: apps.RecordBase{ID: "app-agent", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.xmdm.launcher", Name: "XMDM Agent"},
		},
		versions: map[string][]apps.Version{
			"app-agent": {
				{ID: "agent-old", TenantID: "tenant-1", AppID: "app-agent", Status: apps.VersionStatusPublished, VersionName: "0.1.0", VersionCode: 1, ArtifactID: strPtr("artifact-old"), Checksum: "checksum-old", PublishedAt: &olderPublishedAt, CreatedAt: olderPublishedAt},
				{ID: "agent-draft", TenantID: "tenant-1", AppID: "app-agent", Status: apps.VersionStatusUploaded, VersionName: "0.3.0", VersionCode: 3, ArtifactID: strPtr("artifact-draft"), Checksum: "checksum-draft", CreatedAt: latestPublishedAt.Add(time.Hour)},
				{ID: "agent-latest", TenantID: "tenant-1", AppID: "app-agent", Status: apps.VersionStatusPublished, VersionName: "0.2.0", VersionCode: 2, ArtifactID: strPtr("artifact-latest"), Checksum: "checksum-latest", PublishedAt: &latestPublishedAt, CreatedAt: latestPublishedAt},
			},
		},
		version: apps.Version{
			ID:          "agent-latest",
			TenantID:    "tenant-1",
			AppID:       "app-agent",
			Status:      apps.VersionStatusPublished,
			VersionName: "0.2.0",
			VersionCode: 2,
			ArtifactID:  strPtr("artifact-latest"),
			Artifact: &files.Artifact{
				RecordBase: files.RecordBase{ID: "artifact-latest", TenantID: "tenant-1", Status: files.StatusActive},
				StorageKey: "artifacts/agent-latest.apk",
				Checksum:   "checksum-latest",
				SizeBytes:  int64(len(artifactBytes)),
				MimeType:   "application/vnd.android.package-archive",
			},
			Checksum:    "checksum-latest",
			PublishedAt: &latestPublishedAt,
			CreatedAt:   latestPublishedAt,
		},
	}, &fakeDeviceStore{}, &fakeArtifactStore{content: artifactBytes}, nil, "tenant-1", "com.xmdm.launcher")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollment/agent.apk", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Type"); got != "application/vnd.android.package-archive" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := res.Header().Get("X-XMDM-Artifact-Checksum"); got != "checksum-latest" {
		t.Fatalf("unexpected checksum header: %q", got)
	}
	if got := res.Header().Get("Content-Disposition"); got != `attachment; filename="com.xmdm.launcher-0.2.0.apk"` {
		t.Fatalf("unexpected disposition: %q", got)
	}
	if !bytes.Equal(res.Body.Bytes(), artifactBytes) {
		t.Fatalf("unexpected body: %q", res.Body.Bytes())
	}
}

func TestRegisterEnrollmentAgentAPKDownloadRejectsMissingAgent(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeAppStore{
		apps: []apps.App{{RecordBase: apps.RecordBase{ID: "app-other", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.other", Name: "Other"}},
	}, &fakeDeviceStore{}, &fakeArtifactStore{}, nil, "tenant-1", "com.xmdm.launcher")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollment/agent.apk", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestRegisterAppVersionsUsesQueryPagination(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	store := &fakeAppStore{
		versions: map[string][]apps.Version{
			"app-1": {
				{ID: "version-1", TenantID: "tenant-1", AppID: "app-1", Status: apps.VersionStatusPublished, VersionName: "1.0.0", VersionCode: 100},
			},
		},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, store, &fakeDeviceStore{}, &fakeArtifactStore{}, nil, "tenant-1", "com.xmdm.launcher")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/versions?page=2&limit=9", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", res.Code, res.Body.String())
	}
	if store.lastVersionsParams.Limit != 9 {
		t.Fatalf("unexpected versions limit: %+v", store.lastVersionsParams)
	}
	if store.lastVersionsParams.Offset != 9 {
		t.Fatalf("unexpected versions offset: %+v", store.lastVersionsParams)
	}
}

type fakeAppStore struct {
	apps               []apps.App
	versions           map[string][]apps.Version
	version            apps.Version
	lastVersionsParams pagination.Params
}

func (s *fakeAppStore) ListApps(_ context.Context, _ string, params pagination.Params) ([]apps.App, error) {
	return append([]apps.App(nil), s.apps...), nil
}

func (s *fakeAppStore) GetOverviewStats(context.Context, string) (apps.OverviewStats, error) {
	return apps.OverviewStats{Total: len(s.apps), Active: len(s.apps)}, nil
}

func (s *fakeAppStore) GetApp(context.Context, string, string) (apps.App, error) {
	return apps.App{}, httpx.ErrNotFound
}

func (s *fakeAppStore) GetAppByPackageName(_ context.Context, _ string, packageName string) (apps.App, error) {
	for _, app := range s.apps {
		if app.PackageName == packageName {
			return app, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}

func (s *fakeAppStore) CreateApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) UpdateApp(context.Context, string, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) RetireApp(context.Context, string, string) (apps.App, error) {
	return apps.App{}, nil
}

func (s *fakeAppStore) ListVersions(_ context.Context, _ string, appID string, params pagination.Params) ([]apps.Version, error) {
	s.lastVersionsParams = params
	return append([]apps.Version(nil), s.versions[appID]...), nil
}

func (s *fakeAppStore) GetVersionByCode(context.Context, string, string, int64) (apps.Version, error) {
	return apps.Version{}, httpx.ErrNotFound
}

func (s *fakeAppStore) GetLatestPublishedVersion(_ context.Context, _ string, appID string) (apps.Version, error) {
	var latest *apps.Version
	for i := range s.versions[appID] {
		version := s.versions[appID][i]
		if version.Status != apps.VersionStatusPublished {
			continue
		}
		if latest == nil {
			latest = &version
			continue
		}
		if version.PublishedAt != nil && latest.PublishedAt != nil {
			if version.PublishedAt.After(*latest.PublishedAt) {
				latest = &version
			}
			continue
		}
		if version.PublishedAt != nil && latest.PublishedAt == nil {
			latest = &version
			continue
		}
		if version.PublishedAt == nil && latest.PublishedAt == nil && version.CreatedAt.After(latest.CreatedAt) {
			latest = &version
		}
	}
	if latest == nil {
		return apps.Version{}, httpx.ErrNotFound
	}
	return *latest, nil
}

func (s *fakeAppStore) GetVersion(context.Context, string, string, string) (apps.Version, error) {
	return s.version, nil
}

func (s *fakeAppStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return apps.Version{}, nil
}

type fakeDeviceStore struct{}

func (s *fakeDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return nil, nil
}

func (s *fakeDeviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	return nil, nil
}

func (s *fakeDeviceStore) GetOverviewStats(context.Context, string) (device.OverviewStats, error) {
	return device.OverviewStats{}, nil
}

func (s *fakeDeviceStore) GetStatusCounts(context.Context, string) (device.StatusCounts, error) {
	return device.StatusCounts{}, nil
}

func (s *fakeDeviceStore) GetDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, httpx.ErrNotFound
}

func (s *fakeDeviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDeviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return device.Device{}, nil
}

type fakeArtifactStore struct {
	content []byte
}

func (s *fakeArtifactStore) Put(context.Context, string, io.Reader, string, int64) error {
	return nil
}

func (s *fakeArtifactStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func (s *fakeArtifactStore) Delete(context.Context, string) error {
	return nil
}

type fakeAuditStore struct{}

func (fakeAuditStore) Record(context.Context, string, string, string, string, string, map[string]any) (audit.Event, error) {
	return audit.Event{}, nil
}

func (fakeAuditStore) List(context.Context, string) ([]audit.Event, error) {
	return nil, nil
}

func strPtr(value string) *string {
	return &value
}
