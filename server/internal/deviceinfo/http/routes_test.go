package deviceinfohttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/httpx"
)

func TestRegisterDeviceInfoUpload(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	mux := http.NewServeMux()
	store := &fakeDeviceInfoStore{
		uploadRecords: []deviceinfo.Record{
			{ID: "info-1", TenantID: "tenant-1", DeviceID: "device-123", ObservedAt: time.Unix(0, 0).UTC()},
		},
	}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{}, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/info", bytes.NewBufferString(`{
		"payload":{"model":"Pixel"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(deviceSecretHeader, "device-secret")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	var payload DeviceInfoResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.DeviceInfo) != 1 || payload.DeviceInfo[0].ID != "info-1" {
		t.Fatalf("unexpected upload response: %#v", payload)
	}
}

func TestRegisterDeviceInfoSearch(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	mux := http.NewServeMux()
	store := &fakeDeviceInfoStore{
		searchRecords: []deviceinfo.Record{
			{ID: "info-1", TenantID: "tenant-1", DeviceID: "device-123", ObservedAt: time.Unix(0, 0).UTC()},
		},
	}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{}, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device-info?deviceId=device-123&limit=10", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	var payload DeviceInfoResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.DeviceInfo) != 1 || payload.DeviceInfo[0].ID != "info-1" {
		t.Fatalf("unexpected search response: %#v", payload)
	}
}

type fakeDeviceInfoStore struct {
	uploadRecords []deviceinfo.Record
	searchRecords []deviceinfo.Record
}

func (s *fakeDeviceInfoStore) Upload(context.Context, string, string, string, deviceinfo.UploadRequest) ([]deviceinfo.Record, error) {
	return s.uploadRecords, nil
}

func (s *fakeDeviceInfoStore) Search(context.Context, string, deviceinfo.SearchFilter) ([]deviceinfo.Record, error) {
	return s.searchRecords, nil
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
