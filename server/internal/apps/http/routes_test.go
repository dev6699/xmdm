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
	}, &fakeDeviceStore{}, &fakeArtifactStore{content: artifactBytes}, nil, "tenant-1")

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

type fakeAppStore struct {
	version apps.Version
}

func (s *fakeAppStore) ListApps(context.Context, string) ([]apps.App, error) {
	return nil, nil
}

func (s *fakeAppStore) GetApp(context.Context, string, string) (apps.App, error) {
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

func (s *fakeAppStore) ListVersions(context.Context, string, string) ([]apps.Version, error) {
	return nil, nil
}

func (s *fakeAppStore) GetVersion(context.Context, string, string, string) (apps.Version, error) {
	return s.version, nil
}

func (s *fakeAppStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return apps.Version{}, nil
}

type fakeDeviceStore struct{}

func (s *fakeDeviceStore) ListDevices(context.Context, string) ([]device.Device, error) {
	return nil, nil
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
