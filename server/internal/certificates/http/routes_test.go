package certificatehttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/device"
	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
)

func TestRegisterCertificateArtifactRoute(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{
				ID:       "device-123",
				TenantID: "tenant-1",
				Status:   device.StatusEnrolled,
			},
			Name:            "device-123",
			BootstrapExtras: map[string]any{},
		},
	}, &fakeCertificateStore{
		certificate: certificates.Certificate{
			RecordBase: certificates.RecordBase{
				ID:       "cert-1",
				TenantID: "tenant-1",
				Status:   certificates.StatusActive,
			},
			Name:       "wifi-root-ca",
			ArtifactID: "artifact-1",
			Checksum:   "sha256-cert-abc",
			Artifact: &files.Artifact{
				RecordBase: files.RecordBase{
					ID:       "artifact-1",
					TenantID: "tenant-1",
					Status:   files.StatusActive,
				},
				StorageKey: "artifacts/wifi-root-ca.pem",
				MimeType:   "application/x-pem-file",
				Checksum:   "sha256-cert-abc",
				SizeBytes:  int64(len("certificate-payload")),
			},
		},
	}, &fakeArtifactStore{content: []byte("certificate-payload")}, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/certificates/cert-1/artifact", nil)
	req.Header.Set("X-XMDM-Device-Secret", "device-secret")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/x-pem-file" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := res.Header().Get("X-XMDM-Artifact-Checksum"); got == "" {
		t.Fatalf("expected checksum header")
	}
	if got := res.Header().Get("Content-Disposition"); got != `attachment; filename="wifi-root-ca"` {
		t.Fatalf("unexpected content disposition: %q", got)
	}
	if body := res.Body.String(); body != "certificate-payload" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestRegisterCertificatesCollectionUsesQueryPagination(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	store := &fakeCertificateStore{}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{}, store, nil, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates?page=2&limit=9", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", res.Code, res.Body.String())
	}
	if store.lastParams.Limit != 9 {
		t.Fatalf("unexpected limit: %+v", store.lastParams)
	}
	if store.lastParams.Offset != 9 {
		t.Fatalf("unexpected offset: %+v", store.lastParams)
	}
}

type fakeDeviceStore struct {
	device device.Device
}

func (s *fakeDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return nil, nil
}

func (s *fakeDeviceStore) ListDevicesByFilter(context.Context, string, pagination.Params, device.DeviceListFilter) ([]device.Device, error) {
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

func (s *fakeDeviceStore) Authenticate(_ context.Context, _ string, deviceID, secret string) (device.Device, error) {
	expectedID := s.device.ID
	if expectedID == "" {
		expectedID = s.device.Name
	}
	if deviceID != expectedID || secret != "device-secret" {
		return device.Device{}, httpx.ErrNotFound
	}
	return s.device, nil
}

type fakeCertificateStore struct {
	certificate certificates.Certificate
	lastParams  pagination.Params
}

func (s *fakeCertificateStore) ListCertificates(_ context.Context, _ string, params pagination.Params) ([]certificates.Certificate, error) {
	s.lastParams = params
	return []certificates.Certificate{s.certificate}, nil
}

func (s *fakeCertificateStore) ListActiveCertificates(_ context.Context, _ string, params pagination.Params) ([]certificates.Certificate, error) {
	s.lastParams = params
	return []certificates.Certificate{s.certificate}, nil
}

func (s *fakeCertificateStore) GetOverviewStats(context.Context, string) (certificates.OverviewStats, error) {
	return certificates.OverviewStats{Total: 1, Active: 1}, nil
}

func (s *fakeCertificateStore) GetCertificate(context.Context, string, string) (certificates.Certificate, error) {
	if s.certificate.ID == "" {
		return certificates.Certificate{}, httpx.ErrNotFound
	}
	return s.certificate, nil
}

func (s *fakeCertificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

func (s *fakeCertificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
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

func (s *fakeArtifactStore) HealthCheck(context.Context) error {
	return nil
}
