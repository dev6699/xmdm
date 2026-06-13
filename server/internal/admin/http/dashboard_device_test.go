package adminhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	"xmdm/server/internal/pagination"
)

func TestDeviceMutationsRecordAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	store := &recordingDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive},
			Name:       "device-1",
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices:  store,
		Audit:    auditStore,
		TenantID: "tenant-1",
	})

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create device", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/devices/create", "name=Tablet&policyId=policy-1&csrfToken=token")
		assertRedirect(t, rr, "/admin/devices?ok=device+created")
		assertAuditRecord(t, auditStore, "create", "devices", "device-1")
	})

	t.Run("update device", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/devices/device-1/update", "name=Tablet&policyId=policy-1&csrfToken=token")
		assertRedirect(t, rr, "/admin/devices/device-1?ok=device+updated")
		assertAuditRecord(t, auditStore, "update", "devices", "device-1")
	})

	t.Run("retire device", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/devices/device-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/devices?ok=device+retired")
		assertAuditRecord(t, auditStore, "retire", "devices", "device-1")
	})
}

type recordingDeviceStore struct {
	device device.Device
}

func (s *recordingDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return []device.Device{s.device}, nil
}

func (s *recordingDeviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	return []device.Device{s.device}, nil
}

func (s *recordingDeviceStore) GetOverviewStats(context.Context, string) (device.OverviewStats, error) {
	return device.OverviewStats{}, nil
}

func (s *recordingDeviceStore) GetStatusCounts(context.Context, string) (device.StatusCounts, error) {
	return device.StatusCounts{}, nil
}

func (s *recordingDeviceStore) GetDevice(context.Context, string, string) (device.Device, error) {
	return s.device, nil
}

func (s *recordingDeviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return s.device, nil
}

func (s *recordingDeviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return s.device, nil
}

func (s *recordingDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return s.device, nil
}

func (s *recordingDeviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return s.device, nil
}
