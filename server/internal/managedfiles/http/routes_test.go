package managedfilehttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"
)

func TestRegisterDeviceManagedFileArtifactRoute(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeManagedFileStore{
		item: managedfiles.ManagedFile{
			RecordBase: managedfiles.RecordBase{
				ID:       "managed-file-1",
				TenantID: "tenant-1",
				Status:   managedfiles.StatusActive,
			},
			FileID: "file-1",
			Path:   "device-config.txt",
			File: &files.File{
				RecordBase: files.RecordBase{
					ID:       "file-1",
					TenantID: "tenant-1",
					Status:   files.StatusActive,
				},
				Name:       "device-config.txt",
				ArtifactID: "artifact-1",
				Checksum:   "sha256-file-abc",
				MimeType:   "text/plain",
				Artifact: &files.Artifact{
					RecordBase: files.RecordBase{
						ID:       "artifact-1",
						TenantID: "tenant-1",
						Status:   files.StatusActive,
					},
					StorageKey: "artifacts/device-config.txt",
					Checksum:   "sha256-file-abc",
					SizeBytes:  19,
					MimeType:   "text/plain",
				},
			},
		},
	}, &fakeDeviceStore{}, &fakeArtifactStore{content: []byte("device-config-bytes")}, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/managed-files/managed-file-1/artifact", nil)
	req.Header.Set(deviceSecretHeader, "device-secret")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := res.Header().Get("X-XMDM-Artifact-Checksum"); got != "sha256-file-abc" {
		t.Fatalf("unexpected checksum header: %q", got)
	}
	if got := res.Header().Get("Content-Disposition"); got != `attachment; filename="device-config.txt"` {
		t.Fatalf("unexpected disposition: %q", got)
	}
	if !bytes.Equal(res.Body.Bytes(), []byte("device-config-bytes")) {
		t.Fatalf("unexpected body: %q", res.Body.Bytes())
	}
}

type fakeManagedFileStore struct {
	item managedfiles.ManagedFile
}

func (s *fakeManagedFileStore) ListManagedFiles(context.Context, string) ([]managedfiles.ManagedFile, error) {
	return []managedfiles.ManagedFile{s.item}, nil
}

func (s *fakeManagedFileStore) GetManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return s.item, nil
}

func (s *fakeManagedFileStore) CreateManagedFile(context.Context, string, managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

func (s *fakeManagedFileStore) RetireManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
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

func (s *fakeDeviceStore) Authenticate(_ context.Context, _ string, deviceID, secret string) (device.Device, error) {
	if deviceID != "device-123" || secret != "device-secret" {
		return device.Device{}, httpx.ErrNotFound
	}
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
