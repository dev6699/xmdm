package loghttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
	"xmdm/server/internal/pagination"
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/logs", bytes.NewBufferString(fmt.Sprintf(`{
		"entries":[{"id":"%s","source":"launcher","level":"info","message":"hello"}]
	}`, uuid.NewString())))
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
func (s *fakeDeviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return device.Device{}, nil
}
