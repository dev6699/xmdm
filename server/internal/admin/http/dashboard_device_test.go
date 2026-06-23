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
	"xmdm/server/internal/telemetry"
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

func TestDevicesPageShowsBatteryAndLastOnline(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
	observedAt := time.Now().Add(-30 * time.Hour)
	store := &recordingDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: observedAt.Add(-24 * time.Hour)},
			Name:       "tablet-001",
		},
	}
	telemetryStore := &recordingTelemetryStore{
		records: map[string]telemetry.Record{
			"device-1": {
				DeviceID:   "device-1",
				TenantID:   "tenant-1",
				ObservedAt: observedAt,
				Payload:    map[string]any{"battery": map[string]any{"level": 17}},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices:   store,
		Telemetry: telemetryStore,
		TenantID:  "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected devices page status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Created", "Name", "Status", "Battery", "Last online", "Policy", "17%", formatDashboardTime(observedAt), "tablet-001", "status-low", "status-stale"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected devices page body to contain %q, got %s", want, body)
		}
	}
}

func TestDevicesPageShowsDecimalBatteryLevel(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
	observedAt := time.Now().Add(-2 * time.Hour)
	store := &recordingDeviceStore{
		device: device.Device{
			RecordBase: device.RecordBase{ID: "device-decimal", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: observedAt.Add(-24 * time.Hour)},
			Name:       "decimal-tablet",
		},
	}
	telemetryStore := &recordingTelemetryStore{
		records: map[string]telemetry.Record{
			"device-decimal": {
				DeviceID:   "device-decimal",
				TenantID:   "tenant-1",
				ObservedAt: observedAt,
				Payload:    map[string]any{"battery": map[string]any{"level": "19.5"}},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices:   store,
		Telemetry: telemetryStore,
		TenantID:  "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected devices page status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"decimal-tablet", "19.5%", "status-low"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected devices page body to contain %q, got %s", want, body)
		}
	}
}

func TestDevicesPageFiltersByHealth(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
	lowObserved := time.Now().Add(-2 * time.Hour)
	staleObserved := time.Now().Add(-30 * time.Hour)
	healthyObserved := time.Now().Add(-90 * time.Minute)
	lowDevice := device.Device{
		RecordBase: device.RecordBase{ID: "device-low", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-48 * time.Hour)},
		Name:       "low-tablet",
	}
	staleDevice := device.Device{
		RecordBase: device.RecordBase{ID: "device-stale", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-47 * time.Hour)},
		Name:       "stale-tablet",
	}
	healthyDevice := device.Device{
		RecordBase: device.RecordBase{ID: "device-healthy", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-46 * time.Hour)},
		Name:       "healthy-tablet",
	}
	store := &recordingDeviceStore{
		devices: []device.Device{lowDevice, staleDevice, healthyDevice},
		filtered: map[string][]device.Device{
			string(device.HealthFilterLowBattery): []device.Device{lowDevice},
			string(device.HealthFilterStale):      []device.Device{staleDevice},
		},
	}
	telemetryStore := &recordingTelemetryStore{
		records: map[string]telemetry.Record{
			"device-low": {
				DeviceID:   "device-low",
				TenantID:   "tenant-1",
				ObservedAt: lowObserved,
				Payload:    map[string]any{"battery": map[string]any{"level": 17}},
			},
			"device-stale": {
				DeviceID:   "device-stale",
				TenantID:   "tenant-1",
				ObservedAt: staleObserved,
				Payload:    map[string]any{"battery": map[string]any{"level": 82}},
			},
			"device-healthy": {
				DeviceID:   "device-healthy",
				TenantID:   "tenant-1",
				ObservedAt: healthyObserved,
				Payload:    map[string]any{"battery": map[string]any{"level": 70}},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices:   store,
		Telemetry: telemetryStore,
		TenantID:  "tenant-1",
	})

	assertPage := func(path string, want []string, dontWant []string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected devices page status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, wantItem := range want {
			if !strings.Contains(body, wantItem) {
				t.Fatalf("expected %q in body, got %s", wantItem, body)
			}
		}
		for _, dont := range dontWant {
			if strings.Contains(body, dont) {
				t.Fatalf("did not expect %q in body, got %s", dont, body)
			}
		}
	}

	assertPage("/admin/devices?health=low", []string{"low-tablet", "17%", "Low battery"}, []string{"stale-tablet", "healthy-tablet"})
	assertPage("/admin/devices?health=stale", []string{"stale-tablet", "82%", "Stale online"}, []string{"low-tablet", "healthy-tablet"})
}

func TestDevicesPageSearchesByName(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
	store := &recordingDeviceStore{
		devices: []device.Device{
			{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-3 * time.Hour)}, Name: "alpha-tablet"},
			{RecordBase: device.RecordBase{ID: "device-2", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-2 * time.Hour)}, Name: "beta-kiosk"},
			{RecordBase: device.RecordBase{ID: "device-3", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Now().Add(-time.Hour)}, Name: "gamma-tablet"},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices:  store,
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices?search=beta", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected devices page status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"beta-kiosk", `value="beta"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected devices page body to contain %q, got %s", want, body)
		}
	}
	for _, dont := range []string{"alpha-tablet", "gamma-tablet"} {
		if strings.Contains(body, dont) {
			t.Fatalf("did not expect %q in body, got %s", dont, body)
		}
	}
}

type recordingDeviceStore struct {
	device   device.Device
	devices  []device.Device
	filtered map[string][]device.Device
}

func (s *recordingDeviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return append([]device.Device(nil), s.listItems()...), nil
}

func (s *recordingDeviceStore) ListDevicesByFilter(_ context.Context, _ string, _ pagination.Params, filter device.DeviceListFilter) ([]device.Device, error) {
	items := s.listItems()
	health := strings.TrimSpace(strings.ToLower(string(filter.Health)))
	search := strings.TrimSpace(strings.ToLower(filter.NameQuery))
	out := make([]device.Device, 0, len(items))
	for _, item := range items {
		if search != "" && !strings.Contains(strings.ToLower(item.Name), search) {
			continue
		}
		if len(s.filtered) > 0 {
			if filtered, ok := s.filtered[health]; ok && len(filtered) > 0 {
				match := false
				for _, candidate := range filtered {
					if candidate.ID == item.ID {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			} else if health == string(device.HealthFilterLowBattery) || health == string(device.HealthFilterStale) {
				continue
			}
		}
		out = append(out, item)
	}
	return append([]device.Device(nil), out...), nil
}

func (s *recordingDeviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	return append([]device.Device(nil), s.listItems()...), nil
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

func (s *recordingDeviceStore) listItems() []device.Device {
	if len(s.devices) > 0 {
		return s.devices
	}
	if s.device.ID != "" {
		return []device.Device{s.device}
	}
	return nil
}

type recordingTelemetryStore struct {
	records map[string]telemetry.Record
}

func (s *recordingTelemetryStore) ListLatestByDeviceIDs(context.Context, string, []string) (map[string]telemetry.Record, error) {
	return s.records, nil
}
