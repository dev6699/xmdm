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
)

func TestRegisterCertificateArtifactRoute(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionDevicesWrite})
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{
				ID:       "device-record-1",
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

type fakeDeviceStore struct {
	device device.Device
}

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
	if deviceID != s.device.Name || secret != "device-secret" {
		return device.Device{}, httpx.ErrNotFound
	}
	return s.device, nil
}

type fakeCertificateStore struct {
	certificate certificates.Certificate
}

func (s *fakeCertificateStore) ListCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return []certificates.Certificate{s.certificate}, nil
}

func (s *fakeCertificateStore) ListActiveCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return []certificates.Certificate{s.certificate}, nil
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
