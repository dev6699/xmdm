package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
)

func TestDisabledManagerRegistersNoRoutes(t *testing.T) {
	mux := http.NewServeMux()
	Disabled().Register(httpx.WithPrefix(mux, "/api/v1/admin"), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled plugins, got %d", res.Code)
	}
}

func TestManagerExposesRegisteredMetadata(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mgr := New(Plugin{
		ID:          "remote-control",
		Name:        "Remote Control",
		Description: "Premium device access",
		Enabled:     true,
		Permissions: []string{"admin:remote-control"},
		Routes: []RouteSpec{
			{Method: http.MethodGet, Path: "/settings"},
		},
		DeviceActions: []DeviceAction{
			{
				ActionID:           "start-session",
				Label:              "Start session",
				Href:               "/admin/plugins/remote-control/sessions/new",
				RequiredPermission: string(auth.PermissionDevicesWrite),
				Enabled:            true,
			},
		},
		CommandTypes: []CommandType{
			{
				Type:               "remote-lock",
				Label:              "Remote Lock",
				TargetScope:        "device",
				RequiredPermission: string(auth.PermissionDevicesWrite),
			},
		},
	})

	mux := http.NewServeMux()
	mgr.Register(httpx.WithPrefix(mux, "/api/v1/admin"), svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for registered plugins, got %d body=%s", res.Code, res.Body.String())
	}

	var payload catalogResponse
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Plugins) != 1 {
		t.Fatalf("unexpected plugin count: %#v", payload.Plugins)
	}
	got := payload.Plugins[0]
	if got.ID != "remote-control" || !got.Enabled {
		t.Fatalf("unexpected plugin metadata: %#v", got)
	}
	if len(got.Routes) != 1 || got.Routes[0].Path != "/settings" {
		t.Fatalf("unexpected routes: %#v", got.Routes)
	}
	if len(got.DeviceActions) != 1 || got.DeviceActions[0].ActionID != "start-session" {
		t.Fatalf("unexpected device actions: %#v", got.DeviceActions)
	}
	if len(got.CommandTypes) != 1 || got.CommandTypes[0].Type != "remote-lock" {
		t.Fatalf("unexpected command types: %#v", got.CommandTypes)
	}
	perms := mgr.PermissionCatalog()
	if !containsPermission(perms, auth.PermissionDevicesWrite) || !containsPermission(perms, auth.Permission("admin:remote-control")) {
		t.Fatalf("unexpected permission catalog: %#v", perms)
	}
}

func TestManagerRequiresAdminAuth(t *testing.T) {
	mgr := New(Plugin{ID: "demo", Name: "Demo", Enabled: true})
	mux := http.NewServeMux()
	mgr.Register(httpx.WithPrefix(mux, "/api/v1/admin"), auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing session, got %d", res.Code)
	}
}

func TestManagerFiltersDeviceActionsByPermissionAndEnablement(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mgr := New(Plugin{
		ID:      "remote-control",
		Name:    "Remote Control",
		Enabled: true,
		DeviceActions: []DeviceAction{
			{
				ActionID: "launch",
				Label:    "Launch session",
				Href:     "/admin/plugins/remote-control/devices/{{deviceId}}/launch",
				Enabled:  true,
			},
			{
				ActionID:           "lock",
				Label:              "Remote Lock",
				Href:               "/admin/plugins/remote-control/devices/{{deviceId}}/lock",
				RequiredPermission: string(auth.PermissionDevicesWrite),
				Enabled:            true,
			},
			{
				ActionID: "hidden",
				Label:    "Hidden",
				Href:     "/admin/plugins/remote-control/devices/{{deviceId}}/hidden",
				Enabled:  false,
			},
		},
	})

	actions := mgr.DeviceActionsFor(&session, "device-1")
	if len(actions) != 1 {
		t.Fatalf("unexpected action count: %#v", actions)
	}
	got := actions[0]
	if got.PluginID != "remote-control" || got.ActionID != "launch" {
		t.Fatalf("unexpected action metadata: %#v", got)
	}
	if got.Href != "/admin/plugins/remote-control/devices/device-1/launch" {
		t.Fatalf("unexpected action href: %#v", got.Href)
	}
}

func containsPermission(perms []auth.Permission, target auth.Permission) bool {
	for _, perm := range perms {
		if perm == target {
			return true
		}
	}
	return false
}
