package loghttp

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
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
)

func TestRegisterDeviceLogUpload(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	mux := http.NewServeMux()
	store := &fakeLogStore{
		uploadRecords: []logs.Record{
			{ID: "log-1", TenantID: "tenant-1", DeviceID: "device-123", ObservedAt: time.Unix(0, 0).UTC(), Message: "first"},
		},
	}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{}, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/logs", bytes.NewBufferString(`{
		"entries":[{"source":"launcher","level":"info","message":"hello"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(deviceSecretHeader, "device-secret")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	var payload UploadResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Logs) != 1 || payload.Logs[0].Message != "first" {
		t.Fatalf("unexpected upload response: %#v", payload)
	}
}

func TestRegisterLogsSearch(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Minute, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	mux := http.NewServeMux()
	store := &fakeLogStore{
		searchRecords: []logs.Record{
			{ID: "log-1", TenantID: "tenant-1", DeviceID: "device-123", ObservedAt: time.Unix(0, 0).UTC(), Message: "first"},
		},
	}
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, &fakeDeviceStore{}, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?deviceId=device-123&limit=10", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	var payload UploadResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Logs) != 1 || payload.Logs[0].Message != "first" {
		t.Fatalf("unexpected search response: %#v", payload)
	}
}

type fakeLogStore struct {
	uploadRecords []logs.Record
	searchRecords []logs.Record
}

func (s *fakeLogStore) Upload(context.Context, string, string, string, logs.UploadRequest) ([]logs.Record, error) {
	return s.uploadRecords, nil
}

func (s *fakeLogStore) Search(context.Context, string, logs.SearchFilter) ([]logs.Record, error) {
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
