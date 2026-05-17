package adminhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/checksum"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/files"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/managedfiles"
	"xmdm/server/internal/policy"
)

func TestRegisterCreatesCommands(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{
			{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"},
			{ID: "cmd-2", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-2"},
		},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader(`{"type":"reboot","target":{"type":"broadcast"}}`))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Commands []commands.Command `json:"commands"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Commands) != 2 {
		t.Fatalf("unexpected commands: %#v", payload.Commands)
	}
}

func TestRegisterCreatesCommandsWithWriteOnlySession(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{
			{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"},
		},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader(`{"type":"reboot","target":{"type":"broadcast"}}`))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterSessionFormsRequireCSRF(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, &fakeAdminCommandStore{}, "tenant-1")

	loginReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/login", nil)
	loginRes := httptest.NewRecorder()
	mux.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login form status = %d, want %d", loginRes.Code, http.StatusOK)
	}
	if !strings.Contains(loginRes.Body.String(), csrfFieldName) {
		t.Fatalf("login form missing csrf token field: %s", loginRes.Body.String())
	}
	csrfToken := mustGetCSRFCookie(t, loginRes.Result())

	unauthReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("username=admin&password=secret"))
	unauthReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unauthReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	unauthRes := httptest.NewRecorder()
	mux.ServeHTTP(unauthRes, unauthReq)
	if unauthRes.Code != http.StatusForbidden {
		t.Fatalf("login without csrf = %d, want %d", unauthRes.Code, http.StatusForbidden)
	}

	directLoginReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("username=admin&password=secret"))
	directLoginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	directLoginRes := httptest.NewRecorder()
	mux.ServeHTTP(directLoginRes, directLoginReq)
	if directLoginRes.Code != http.StatusSeeOther {
		t.Fatalf("direct login without csrf cookie = %d, want %d", directLoginRes.Code, http.StatusSeeOther)
	}

	loginReq = httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=admin&password=secret"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	loginRes = httptest.NewRecorder()
	mux.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusSeeOther {
		t.Fatalf("login with csrf = %d, want %d", loginRes.Code, http.StatusSeeOther)
	}

	session := mustGetSessionCookie(t, loginRes.Result())
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", strings.NewReader("csrfToken="+csrfToken))
	logoutReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
	logoutReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	logoutRes := httptest.NewRecorder()
	mux.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusNoContent {
		t.Fatalf("logout with csrf = %d, want %d", logoutRes.Code, http.StatusNoContent)
	}

	badLogoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", strings.NewReader(""))
	badLogoutReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
	badLogoutReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	badLogoutRes := httptest.NewRecorder()
	mux.ServeHTTP(badLogoutRes, badLogoutReq)
	if badLogoutRes.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf = %d, want %d", badLogoutRes.Code, http.StatusForbidden)
	}

	directLogoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	directLogoutReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
	directLogoutRes := httptest.NewRecorder()
	mux.ServeHTTP(directLogoutRes, directLogoutReq)
	if directLogoutRes.Code != http.StatusNoContent {
		t.Fatalf("direct logout without csrf cookie = %d, want %d", directLogoutRes.Code, http.StatusNoContent)
	}
}

func TestRegisterRejectsReadOnlySessionForCommandCreation(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader(`{"type":"reboot","target":{"type":"broadcast"}}`))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterRejectsTextPlainCommandSubmissionWithoutCSRF(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader("type=reboot&targetType=broadcast"))
	req.Header.Set("Content-Type", "text/plain")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterListsCommands(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/commands", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Commands []commands.Command `json:"commands"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Commands) != 1 || payload.Commands[0].ID != "cmd-1" {
		t.Fatalf("unexpected commands: %#v", payload.Commands)
	}
}

func TestRegisterCommandsPageUsesSelectorsAndShowsAllCommandTypes(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	devices := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", Status: device.StatusActive}, Name: "Tablet One"}, {RecordBase: device.RecordBase{ID: "device-2", Status: device.StatusSuspended}, Name: "Tablet Two"}},
	}
	groups := &fakeDashboardGroupStore{
		items: []group.Group{{RecordBase: group.RecordBase{ID: "group-1", Status: "active"}, Name: "Field Devices"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Commands: &fakeAdminCommandStore{items: []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued, DeviceID: "device-1"}}}, Devices: devices, Groups: groups, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/commands", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<th>Created</th><th>ID</th><th>Type</th><th>Device</th><th>Status</th><th>Expires</th>",
		`type="datetime-local"`,
		`name="targetDeviceId"`,
		`name="targetGroupId"`,
		`value="ping"`,
		`value="reboot"`,
		`value="sync_config"`,
		`value="exit_kiosk"`,
		`value="device"`,
		`value="group"`,
		`device-1`,
		`Tablet One`,
		`Field Devices`,
		`Select device`,
		`Select group`,
		`/admin/commands/cmd-1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("commands page missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `device-2`) || strings.Contains(body, `Tablet Two`) {
		t.Fatalf("commands page should not show suspended device in selector: %s", body)
	}
}

func TestRegisterCreatesCommandFromFormWithLocalExpiryAndTargetSelects(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Commands: store, TenantID: "tenant-1"})

	expiry := time.Now().Add(time.Hour).In(time.Local).Truncate(time.Minute)
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)
	form := strings.NewReader("csrfToken=" + csrfToken + "&type=reboot&targetType=device&targetDeviceId=device-1&payload=%7B%22force%22%3Atrue%7D&expiresAt=" + expiry.Format("2006-01-02T15:04"))
	req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(store.enqueues) != 1 {
		t.Fatalf("expected one enqueue, got %#v", store.enqueues)
	}
	reqUpsert := store.enqueues[0]
	if reqUpsert.Type != "reboot" || reqUpsert.Target.Type != commands.TargetDevice || reqUpsert.Target.DeviceID != "device-1" {
		t.Fatalf("unexpected enqueue request: %#v", reqUpsert)
	}
	if reqUpsert.Target.GroupID != "" {
		t.Fatalf("unexpected group target: %#v", reqUpsert.Target)
	}
	if reqUpsert.ExpiresAt == nil || !reqUpsert.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected expiry: %#v", reqUpsert.ExpiresAt)
	}
	if got := reqUpsert.Payload["force"]; got != true {
		t.Fatalf("unexpected payload: %#v", reqUpsert.Payload)
	}
}

func TestRegisterRejectsBroadcastCommandFromDashboardForm(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Commands: store, TenantID: "tenant-1"})

	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)
	form := strings.NewReader("csrfToken=" + csrfToken + "&type=reboot&targetType=broadcast&payload=%7B%7D")
	req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Location"), "broadcast+commands+are+disabled") {
		t.Fatalf("expected broadcast rejection, got location=%q", rr.Header().Get("Location"))
	}
}

func TestRegisterShowsCommandDetailPageAndLinksDevice(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-row-1", Status: device.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Name: "Tablet One"}},
	}
	commandStore := &fakeAdminCommandStore{
		items: []commands.Command{{
			ID:        "cmd-1",
			Type:      "reboot",
			Status:    commands.StatusAcked,
			DeviceID:  "device-row-1",
			Payload:   map[string]any{"force": true},
			Result:    map[string]any{"status": commands.StatusAcked, "message": "done"},
			CreatedAt: time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 13, 11, 32, 0, 0, time.UTC),
		}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Commands: commandStore, Devices: deviceStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/commands/cmd-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard command detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{
		"Command Detail",
		"Current command",
		"cmd-1",
		"reboot",
		"Tablet One",
		"Payload",
		"Result",
		"done",
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("dashboard command detail missing %q: %s", want, rr.Body.String())
		}
	}
}

func TestRegisterListsAuditEvents(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	auditStore := &fakeAuditStore{
		events: []audit.Event{{Actor: "admin", Action: "create", ResourceType: "commands", ResourceID: "cmd-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, auditStore, &fakeAdminCommandStore{}, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Events []audit.Event `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0].ResourceID != "cmd-1" {
		t.Fatalf("unexpected events: %#v", payload.Events)
	}
}

func TestRegisterCreatesGroupCommandFromForm(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	csrfToken := mustGetCSRFCookieFromLogin(t, mux)

	form := strings.NewReader("csrfToken=" + csrfToken + "&type=reboot&targetType=group&targetGroupId=group-123&payload=%7B%22force%22%3Atrue%7D")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterRejectsFormCommandWithoutCSRF(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{
		items: []commands.Command{{ID: "cmd-1", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-1"}},
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	form := strings.NewReader("type=reboot&targetType=group&targetGroupId=group-123")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterDashboardRedirectsAnonymousUsersToLogin(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("dashboard status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if got := rr.Header().Get("Location"); got != "/admin/login" {
		t.Fatalf("redirect location = %q, want /admin/login", got)
	}
}

func TestRegisterDashboardListsUsers(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Identity: &fakeDashboardIdentityStore{
			users: []identity.User{{RecordBase: identity.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Email: "alice@example.com", RoleID: "role-1"}},
			roles: []identity.Role{
				{RecordBase: identity.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"}, Name: "Operators"},
				{RecordBase: identity.RecordBase{ID: "role-2", TenantID: "tenant-1", Status: "retired"}, Name: "Auditors"},
			},
		},
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "alice@example.com") {
		t.Fatalf("dashboard users page missing user: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/admin/users/user-1") {
		t.Fatalf("dashboard users page should link to user detail page: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/admin/roles/role-1") {
		t.Fatalf("dashboard users page should link role name to role detail page: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), ">Operators<") {
		t.Fatalf("dashboard users page should show role name instead of role id: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<th>Created</th><th>ID</th><th>Email</th><th>Role</th><th>Status</th>") {
		t.Fatalf("dashboard users page should show created column: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), formatDashboardTime(time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC))) {
		t.Fatalf("dashboard users page should show formatted created time: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status-pill") {
		t.Fatalf("dashboard users page should use status pill styling: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Select a role") {
		t.Fatalf("dashboard users page should show role select placeholder: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Auditors (retired)") {
		t.Fatalf("dashboard users page should show retired role option: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "new password optional") {
		t.Fatalf("dashboard users page should not show inline update controls: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/users/user-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard user detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Select a role") {
		t.Fatalf("dashboard user detail page should show role select placeholder: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "leave blank to keep the current password") {
		t.Fatalf("dashboard user detail page should show password placeholder: %s", rr.Body.String())
	}
}

func TestRegisterDashboardListsRoles(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Identity: &fakeDashboardIdentityStore{
			roles: []identity.Role{{RecordBase: identity.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC)}, Name: "Operators", Permissions: []string{"admin.read", "admin.write"}}},
		},
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/roles", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Available permissions:") {
		t.Fatalf("dashboard roles page should show permissions catalog: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<th>Created</th><th>ID</th><th>Name</th><th>Permissions</th><th>Status</th>") {
		t.Fatalf("dashboard roles page should show created column: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), formatDashboardTime(time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC))) {
		t.Fatalf("dashboard roles page should show formatted created time: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status-pill") {
		t.Fatalf("dashboard roles page should use status pill styling: %s", rr.Body.String())
	}
}

func TestRegisterDashboardListsPolicies(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Policies: &fakeDashboardPolicyStore{
			policies: []policy.Policy{{
				RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 33, 0, 0, time.UTC)},
				Name:       "Default policy",
				Version:    2,
				KioskMode:  true,
				Restrictions: json.RawMessage(`{
					"allowPackages":["com.android.chrome"],
					"kioskExitPasscode":"1234"
				}`),
			}},
		},
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/policies", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard policies page status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<th>Created</th><th>ID</th><th>Name</th><th>Kiosk</th><th>Status</th>") {
		t.Fatalf("dashboard policies page should use identity-style columns: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `/admin/policies/policy-1`) {
		t.Fatalf("dashboard policies page should link policy names to detail page: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status-pill status-enabled") || !strings.Contains(rr.Body.String(), ">enabled<") {
		t.Fatalf("dashboard policies page should show kiosk mode as a badge: %s", rr.Body.String())
	}
}

func TestRegisterDashboardPolicyDetailShowsUpdateAndRetire(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Policies: &fakeDashboardPolicyStore{
			policies: []policy.Policy{{
				RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 33, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC)},
				Name:       "Default policy",
				Version:    2,
				KioskMode:  true,
				Restrictions: json.RawMessage(`{
					"allowPackages":["com.android.chrome"],
					"kioskExitPasscode":"1234"
				}`),
			}},
			policyApps: []policy.PolicyApp{
				{
					RecordBase: policy.RecordBase{ID: "policy-app-1", TenantID: "tenant-1", Status: policy.StatusActive},
					PolicyID:   "policy-1",
					AppID:      "app-1",
				},
			},
			policyCertificates: []policy.PolicyCertificate{
				{
					RecordBase:    policy.RecordBase{ID: "policy-cert-1", TenantID: "tenant-1", Status: policy.StatusActive},
					PolicyID:      "policy-1",
					CertificateID: "cert-1",
				},
			},
			policyManagedFiles: []policy.PolicyManagedFile{
				{
					RecordBase:    policy.RecordBase{ID: "policy-file-1", TenantID: "tenant-1", Status: policy.StatusActive},
					PolicyID:      "policy-1",
					ManagedFileID: "mf-1",
				},
			},
		},
		Apps: &fakeDashboardAppStore{
			apps: []apps.App{
				{
					RecordBase:  apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 35, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 35, 0, 0, time.UTC)},
					PackageName: "com.example.enabled",
					Name:        "Enabled app",
				},
				{
					RecordBase:  apps.RecordBase{ID: "app-2", TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 36, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 36, 0, 0, time.UTC)},
					PackageName: "com.example.disabled",
					Name:        "Disabled app",
				},
			},
		},
		Certificates: &fakeDashboardCertificateStore{
			items: []certificates.Certificate{
				{
					RecordBase: certificates.RecordBase{ID: "cert-1", TenantID: "tenant-1", Status: certificates.StatusActive, CreatedAt: time.Date(2026, 5, 13, 12, 10, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 12, 15, 0, 0, time.UTC)},
					Name:       "Root CA",
					ArtifactID: "artifact-1",
					Checksum:   "checksum-1",
					Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 12, 6, 0, 0, time.UTC)}, StorageKey: "certs/root.pem", Checksum: "checksum-1", SizeBytes: 16, MimeType: "application/x-pem-file"},
				},
			},
		},
		ManagedFiles: &fakeDashboardManagedFileStore{
			items: []managedfiles.ManagedFile{
				{
					RecordBase:       managedfiles.RecordBase{ID: "mf-1", TenantID: "tenant-1", Status: managedfiles.StatusActive, CreatedAt: time.Date(2026, 5, 13, 12, 20, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 12, 20, 0, 0, time.UTC)},
					FileID:           "file-1",
					Path:             "/sdcard/xmdm/device-config.txt",
					ReplaceVariables: true,
					File:             &files.File{RecordBase: files.RecordBase{ID: "file-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 12, 19, 0, 0, time.UTC)}, Name: "device-config.txt", ArtifactID: "artifact-2", Checksum: "checksum-2", MimeType: "text/plain", Artifact: &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-2", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 12, 19, 0, 0, time.UTC)}, StorageKey: "files/device-config.txt", Checksum: "checksum-2", SizeBytes: 24, MimeType: "text/plain"}},
				},
			},
		},
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/policies/policy-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard policy detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Policy Detail", "Current policy", "Update policy", "Retire policy", "Managed apps", "Managed certificates", "Managed files", "Enabled app", "Disabled app", "Root CA", "device-config.txt", "Policy state"} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("policy detail should contain %q: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), "Disable") || !strings.Contains(rr.Body.String(), "Enable") {
		t.Fatalf("policy detail should show toggle buttons: %s", rr.Body.String())
	}
	for _, field := range []string{"Enable kiosk mode", "Kiosk exit passcode", "Allow packages", "Block packages", "Suspend packages", "Keep screen on", "Stay awake while plugged in", "Unlock on boot"} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("policy detail should show structured restriction inputs: %s", rr.Body.String())
		}
	}
	kioskIdx := strings.Index(rr.Body.String(), "Enable kiosk mode")
	passcodeIdx := strings.Index(rr.Body.String(), "Kiosk exit passcode")
	if kioskIdx == -1 || passcodeIdx == -1 || passcodeIdx < kioskIdx {
		t.Fatalf("policy detail should show structured restriction inputs: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "name=\"kioskMode\" type=\"checkbox\" value=\"on\" checked") {
		t.Fatalf("policy detail should keep kiosk mode checked when enabled: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status-pill status-enabled") {
		t.Fatalf("policy detail should show kiosk mode as enabled badge: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), formatDashboardTime(time.Date(2026, 5, 13, 11, 33, 0, 0, time.UTC))) {
		t.Fatalf("policy detail should show created time: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `action="/admin/policies/policy-1/certificates/cert-1/toggle"`) {
		t.Fatalf("policy detail should render toggle form for enabled certificate: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `action="/admin/policies/policy-1/managed-files/mf-1/toggle"`) {
		t.Fatalf("policy detail should render toggle form for enabled managed file: %s", rr.Body.String())
	}
}

func TestRegisterDashboardTogglesPolicyManagedApps(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{{
			RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"},
			Name:       "Default policy",
			Version:    1,
		}},
		policyApps: []policy.PolicyApp{
			{
				RecordBase: policy.RecordBase{ID: "policy-app-1", TenantID: "tenant-1", Status: policy.StatusActive},
				PolicyID:   "policy-1",
				AppID:      "app-1",
			},
		},
		policyCertificates: []policy.PolicyCertificate{
			{
				RecordBase:    policy.RecordBase{ID: "policy-cert-1", TenantID: "tenant-1", Status: policy.StatusActive},
				PolicyID:      "policy-1",
				CertificateID: "cert-1",
			},
		},
		policyManagedFiles: []policy.PolicyManagedFile{
			{
				RecordBase:    policy.RecordBase{ID: "policy-file-1", TenantID: "tenant-1", Status: policy.StatusActive},
				PolicyID:      "policy-1",
				ManagedFileID: "mf-1",
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Policies: policyStore,
		Apps: &fakeDashboardAppStore{
			apps: []apps.App{
				{RecordBase: apps.RecordBase{ID: "app-1", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.enabled", Name: "Enabled app"},
				{RecordBase: apps.RecordBase{ID: "app-2", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.disabled", Name: "Disabled app"},
			},
		},
		Certificates: &fakeDashboardCertificateStore{
			items: []certificates.Certificate{
				{RecordBase: certificates.RecordBase{ID: "cert-1", TenantID: "tenant-1", Status: certificates.StatusActive}, Name: "Root CA", ArtifactID: "artifact-1", Artifact: &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive}, StorageKey: "certs/root.pem", MimeType: "application/x-pem-file"}},
				{RecordBase: certificates.RecordBase{ID: "cert-2", TenantID: "tenant-1", Status: certificates.StatusActive}, Name: "WiFi CA", ArtifactID: "artifact-2", Artifact: &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-2", TenantID: "tenant-1", Status: files.StatusActive}, StorageKey: "certs/wifi.pem", MimeType: "application/x-pem-file"}},
			},
		},
		ManagedFiles: &fakeDashboardManagedFileStore{
			items: []managedfiles.ManagedFile{
				{
					RecordBase:       managedfiles.RecordBase{ID: "mf-1", TenantID: "tenant-1", Status: managedfiles.StatusActive},
					FileID:           "file-1",
					Path:             "/sdcard/xmdm/root.txt",
					ReplaceVariables: true,
					File: &files.File{
						RecordBase: files.RecordBase{ID: "file-1", TenantID: "tenant-1", Status: files.StatusActive},
						Name:       "root.txt",
						ArtifactID: "artifact-1",
						Checksum:   "checksum-1",
						MimeType:   "text/plain",
						Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive}, StorageKey: "files/root.txt", Checksum: "checksum-1", SizeBytes: 16, MimeType: "text/plain"},
					},
				},
				{
					RecordBase:       managedfiles.RecordBase{ID: "mf-2", TenantID: "tenant-1", Status: managedfiles.StatusActive},
					FileID:           "file-2",
					Path:             "/sdcard/xmdm/disabled.txt",
					ReplaceVariables: false,
					File: &files.File{
						RecordBase: files.RecordBase{ID: "file-2", TenantID: "tenant-1", Status: files.StatusActive},
						Name:       "disabled.txt",
						ArtifactID: "artifact-2",
						Checksum:   "checksum-2",
						MimeType:   "text/plain",
						Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-2", TenantID: "tenant-1", Status: files.StatusActive}, StorageKey: "files/disabled.txt", Checksum: "checksum-2", SizeBytes: 18, MimeType: "text/plain"},
					},
				},
			},
		},
		TenantID: "tenant-1",
	})

	getReq := httptest.NewRequest(http.MethodGet, "/admin/policies/policy-1", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("policy detail get status = %d body=%s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/apps/app-1/toggle"`) {
		t.Fatalf("policy detail should render toggle form for enabled app: %s", getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/apps/app-2/toggle"`) {
		t.Fatalf("policy detail should render toggle form for disabled app: %s", getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/certificates/cert-1/toggle"`) {
		t.Fatalf("policy detail should render toggle form for enabled certificate: %s", getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/certificates/cert-2/toggle"`) {
		t.Fatalf("policy detail should render toggle form for disabled certificate: %s", getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/managed-files/mf-1/toggle"`) {
		t.Fatalf("policy detail should render toggle form for enabled managed file: %s", getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), `action="/admin/policies/policy-1/managed-files/mf-2/toggle"`) {
		t.Fatalf("policy detail should render toggle form for disabled managed file: %s", getRR.Body.String())
	}

	csrf := mustGetCSRFCookie(t, getRR.Result())
	postReq := httptest.NewRequest(http.MethodPost, "/admin/policies/policy-1/apps/app-2/toggle", strings.NewReader("csrfToken="+csrf))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("toggle enable status = %d body=%s", postRR.Code, postRR.Body.String())
	}
	if got := postRR.Header().Get("Location"); got != "/admin/policies/policy-1?ok=app+enabled" {
		t.Fatalf("toggle enable redirect = %q", got)
	}
	enabled := false
	for _, item := range policyStore.policyApps {
		if item.AppID == "app-2" {
			enabled = item.Status == policy.StatusActive
		}
	}
	if !enabled {
		t.Fatalf("toggle enable should mark app active: %#v", policyStore.policyApps)
	}

	certReq := httptest.NewRequest(http.MethodPost, "/admin/policies/policy-1/certificates/cert-2/toggle", strings.NewReader("csrfToken="+csrf))
	certReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	certReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	certReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	certRR := httptest.NewRecorder()
	mux.ServeHTTP(certRR, certReq)
	if certRR.Code != http.StatusSeeOther {
		t.Fatalf("toggle certificate enable status = %d body=%s", certRR.Code, certRR.Body.String())
	}
	if got := certRR.Header().Get("Location"); got != "/admin/policies/policy-1?ok=certificate+enabled" {
		t.Fatalf("toggle certificate enable redirect = %q", got)
	}
	certEnabled := false
	for _, item := range policyStore.policyCertificates {
		if item.CertificateID == "cert-2" {
			certEnabled = item.Status == policy.StatusActive
		}
	}
	if !certEnabled {
		t.Fatalf("toggle certificate enable should mark certificate active: %#v", policyStore.policyCertificates)
	}

	fileReq := httptest.NewRequest(http.MethodPost, "/admin/policies/policy-1/managed-files/mf-2/toggle", strings.NewReader("csrfToken="+csrf))
	fileReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	fileReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	fileReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	fileRR := httptest.NewRecorder()
	mux.ServeHTTP(fileRR, fileReq)
	if fileRR.Code != http.StatusSeeOther {
		t.Fatalf("toggle managed file enable status = %d body=%s", fileRR.Code, fileRR.Body.String())
	}
	if got := fileRR.Header().Get("Location"); got != "/admin/policies/policy-1?ok=managed+file+enabled" {
		t.Fatalf("toggle managed file enable redirect = %q", got)
	}
	enabled = false
	for _, item := range policyStore.policyManagedFiles {
		if item.ManagedFileID == "mf-2" {
			enabled = item.Status == policy.StatusActive
		}
	}
	if !enabled {
		t.Fatalf("toggle managed file enable should mark binding active: %#v", policyStore.policyManagedFiles)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/admin/policies/policy-1/apps/app-1/toggle", strings.NewReader("csrfToken="+csrf))
	disableReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	disableReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	disableReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	disableRR := httptest.NewRecorder()
	mux.ServeHTTP(disableRR, disableReq)
	if disableRR.Code != http.StatusSeeOther {
		t.Fatalf("toggle disable status = %d body=%s", disableRR.Code, disableRR.Body.String())
	}
	if got := disableRR.Header().Get("Location"); got != "/admin/policies/policy-1?ok=app+disabled" {
		t.Fatalf("toggle disable redirect = %q", got)
	}
	disabled := false
	for _, item := range policyStore.policyApps {
		if item.AppID == "app-1" {
			disabled = item.Status != policy.StatusActive
		}
	}
	if !disabled {
		t.Fatalf("toggle disable should mark app disabled: %#v", policyStore.policyApps)
	}
}

func TestRegisterDashboardCreatesManagedAppInOneFlow(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	appStore := &fakeDashboardAppStore{}
	fileStore := &fakeDashboardFileStore{}
	artifactStore := &fakeDashboardArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Apps:      appStore,
		Files:     fileStore,
		Artifacts: artifactStore,
		TenantID:  "tenant-1",
	})

	getReq := httptest.NewRequest(http.MethodGet, "/admin/apps", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("dashboard apps page status = %d, body=%s", getRR.Code, getRR.Body.String())
	}
	for _, field := range []string{"Create managed app", "packageName", "versionCode", "APK file"} {
		if !strings.Contains(getRR.Body.String(), field) {
			t.Fatalf("dashboard apps page should show %q: %s", field, getRR.Body.String())
		}
	}
	if strings.Contains(getRR.Body.String(), "Publish immediately") {
		t.Fatalf("dashboard apps page should not expose publish toggle: %s", getRR.Body.String())
	}
	for _, field := range []string{`name="artifactId"`, `name="checksum"`, "Create version"} {
		if strings.Contains(getRR.Body.String(), field) {
			t.Fatalf("dashboard apps page should hide legacy version fields: %s", getRR.Body.String())
		}
	}
	csrf := mustGetCSRFCookie(t, getRR.Result())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, kv := range [][2]string{
		{"csrfToken", csrf},
		{"packageName", "com.example.catalog"},
		{"name", "Catalog"},
		{"versionCode", "100"},
	} {
		if err := writer.WriteField(kv[0], kv[1]); err != nil {
			t.Fatalf("write field %s: %v", kv[0], err)
		}
	}
	part, err := writer.CreateFormFile("file", "catalog.apk")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	content := []byte("managed-app-apk-bytes")
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write apk bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/admin/apps/create", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("managed app create status = %d body=%s", postRR.Code, postRR.Body.String())
	}
	if got := postRR.Header().Get("Location"); got != "/admin/apps/app-created?ok=managed+app+created" {
		t.Fatalf("managed app create redirect = %q", got)
	}
	if appStore.createdApp.PackageName != "com.example.catalog" || appStore.createdApp.Name != "Catalog" {
		t.Fatalf("managed app create should forward app fields: %#v", appStore.createdApp)
	}
	if appStore.createdVersion.VersionName != "v100" || appStore.createdVersion.VersionCode != 100 {
		t.Fatalf("managed app create should forward version fields: %#v", appStore.createdVersion)
	}
	if !appStore.createdVersion.Publish {
		t.Fatalf("managed app create should always publish first version: %#v", appStore.createdVersion)
	}
	if appStore.createdVersion.Checksum != checksum.SHA256Base64URL(content) {
		t.Fatalf("managed app version should inherit file checksum: %#v", appStore.createdVersion)
	}
	if appStore.createdVersion.ArtifactID == nil || *appStore.createdVersion.ArtifactID != fileStore.createdFile.ArtifactID {
		t.Fatalf("managed app version should link file artifact: %#v", appStore.createdVersion)
	}
	if !fileStore.createdCalled {
		t.Fatalf("expected dashboard to create file artifact")
	}
	if fileStore.createdFile.Name != "com.example.catalog-100.apk" {
		t.Fatalf("managed app file should derive its name from app/version metadata: %#v", fileStore.createdFile)
	}
	if fileStore.createdFile.Checksum != checksum.SHA256Base64URL(content) {
		t.Fatalf("managed app file should derive checksum from upload: %#v", fileStore.createdFile)
	}
	if fileStore.createdFile.MimeType != "application/vnd.android.package-archive" {
		t.Fatalf("managed app file should use APK mime type: %#v", fileStore.createdFile)
	}
	if artifactStore.putKey == "" || fileStore.createdFile.Artifact == nil || artifactStore.putKey != fileStore.createdFile.Artifact.StorageKey {
		t.Fatalf("managed app should upload to generated storage key: key=%q file=%#v", artifactStore.putKey, fileStore.createdFile)
	}
}

func TestRegisterDashboardPublishesManagedAppVersionForExistingPackage(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	appStore := &fakeDashboardAppStore{
		apps: []apps.App{{RecordBase: apps.RecordBase{ID: "app-existing", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.catalog", Name: "Catalog"}},
	}
	fileStore := &fakeDashboardFileStore{}
	artifactStore := &fakeDashboardArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Apps:      appStore,
		Files:     fileStore,
		Artifacts: artifactStore,
		TenantID:  "tenant-1",
	})

	csrf := mustGetCSRFCookieFromDashboardLogin(t, mux)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, kv := range [][2]string{
		{"csrfToken", csrf},
		{"packageName", "com.example.catalog"},
		{"name", "Catalog"},
		{"versionCode", "101"},
	} {
		if err := writer.WriteField(kv[0], kv[1]); err != nil {
			t.Fatalf("write field %s: %v", kv[0], err)
		}
	}
	part, err := writer.CreateFormFile("file", "catalog.apk")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	content := []byte("managed-app-apk-bytes-existing")
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write apk bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/admin/apps/create", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("managed app reuse status = %d body=%s", postRR.Code, postRR.Body.String())
	}
	if appStore.createAppCalls != 0 {
		t.Fatalf("existing package should not create a new app, calls=%d", appStore.createAppCalls)
	}
	if appStore.createdVersionApp != "app-existing" {
		t.Fatalf("existing package should create version on existing app, got %q", appStore.createdVersionApp)
	}
	if appStore.createdVersion.VersionName != "v101" || appStore.createdVersion.VersionCode != 101 {
		t.Fatalf("existing package should forward version fields: %#v", appStore.createdVersion)
	}
	if !appStore.createdVersion.Publish {
		t.Fatalf("existing package should always publish new version: %#v", appStore.createdVersion)
	}
	if fileStore.createdFile.Artifact == nil || artifactStore.putKey != fileStore.createdFile.Artifact.StorageKey {
		t.Fatalf("existing package should still upload artifact: %#v", fileStore.createdFile)
	}
	if got := postRR.Header().Get("Location"); got != "/admin/apps/app-existing?ok=managed+app+created" {
		t.Fatalf("existing package redirect = %q", got)
	}
}

func TestRegisterDashboardSkipsDuplicateManagedAppVersion(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	checksumValue := checksum.SHA256Base64URL([]byte("managed-app-apk-bytes-existing"))
	appStore := &fakeDashboardAppStore{
		apps: []apps.App{{RecordBase: apps.RecordBase{ID: "app-existing", TenantID: "tenant-1", Status: apps.StatusActive}, PackageName: "com.example.catalog", Name: "Catalog"}},
		versions: map[string][]apps.Version{
			"app-existing": {{
				ID:          "version-existing",
				TenantID:    "tenant-1",
				AppID:       "app-existing",
				Status:      apps.VersionStatusPublished,
				VersionName: "v101",
				VersionCode: 101,
				Checksum:    checksumValue,
			}},
		},
	}
	fileStore := &fakeDashboardFileStore{}
	artifactStore := &fakeDashboardArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Apps:      appStore,
		Files:     fileStore,
		Artifacts: artifactStore,
		TenantID:  "tenant-1",
	})

	csrf := mustGetCSRFCookieFromDashboardLogin(t, mux)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, kv := range [][2]string{
		{"csrfToken", csrf},
		{"packageName", "com.example.catalog"},
		{"name", "Catalog"},
		{"versionCode", "101"},
	} {
		if err := writer.WriteField(kv[0], kv[1]); err != nil {
			t.Fatalf("write field %s: %v", kv[0], err)
		}
	}
	part, err := writer.CreateFormFile("file", "catalog.apk")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write([]byte("managed-app-apk-bytes-existing")); err != nil {
		t.Fatalf("write apk bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/admin/apps/create", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("duplicate managed app status = %d body=%s", postRR.Code, postRR.Body.String())
	}
	if got := postRR.Header().Get("Location"); got != "/admin/apps/app-existing?ok=managed+app+already+up+to+date" {
		t.Fatalf("duplicate managed app redirect = %q", got)
	}
	if appStore.createAppCalls != 0 || appStore.createdVersionApp != "" {
		t.Fatalf("duplicate managed app should not create new app/version: %#v", appStore)
	}
	if fileStore.createdCalled {
		t.Fatalf("duplicate managed app should not upload file artifact")
	}
	if artifactStore.putKey != "" {
		t.Fatalf("duplicate managed app should not upload artifact blob")
	}
}

func TestRegisterDashboardListsAppsInIdentityPattern(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	appCreatedAt := time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC)
	appStore := &fakeDashboardAppStore{
		apps: []apps.App{{
			RecordBase:  apps.RecordBase{ID: "app-existing", TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: appCreatedAt, UpdatedAt: appCreatedAt},
			PackageName: "com.example.catalog",
			Name:        "Catalog",
		}},
		versions: map[string][]apps.Version{
			"app-existing": {{
				ID:          "version-published",
				TenantID:    "tenant-1",
				AppID:       "app-existing",
				Status:      apps.VersionStatusPublished,
				VersionName: "v101",
				VersionCode: 101,
				Checksum:    checksum.SHA256Base64URL([]byte("managed-app-apk-bytes-existing")),
				CreatedAt:   appCreatedAt.Add(time.Minute),
			}},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Apps: appStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/apps", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("apps list status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Created", "ID", "Name", "Package", "Latest published", "Status"} {
		if !strings.Contains(rr.Body.String(), "<th>"+field+"</th>") {
			t.Fatalf("apps list should show %q column: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/apps/app-existing"`) {
		t.Fatalf("apps list should link app names to detail page: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), formatDashboardTime(appCreatedAt)) {
		t.Fatalf("apps list should show created time: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "v101 (#101)") {
		t.Fatalf("apps list should show latest published version: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Actions</th>") || strings.Contains(rr.Body.String(), "Create version") {
		t.Fatalf("apps list should not expose inline actions: %s", rr.Body.String())
	}
}

func TestRegisterDashboardShowsAppDetailPageWithVersions(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	appCreatedAt := time.Date(2026, 5, 13, 11, 35, 0, 0, time.UTC)
	versionCreatedAt := time.Date(2026, 5, 13, 11, 36, 0, 0, time.UTC)
	artifactStore := &fakeDashboardArtifactStore{getContent: []byte("managed-app-apk-bytes-existing")}
	appStore := &fakeDashboardAppStore{
		apps: []apps.App{{
			RecordBase:  apps.RecordBase{ID: "app-existing", TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: appCreatedAt, UpdatedAt: appCreatedAt},
			PackageName: "com.example.catalog",
			Name:        "Catalog",
		}},
		versions: map[string][]apps.Version{
			"app-existing": {{
				ID:          "version-existing",
				TenantID:    "tenant-1",
				AppID:       "app-existing",
				Status:      apps.VersionStatusPublished,
				VersionName: "v101",
				VersionCode: 101,
				ArtifactID:  strPtr("artifact-1"),
				Artifact: &files.Artifact{
					RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: versionCreatedAt},
					StorageKey: "apps/catalog-v101.apk",
					Checksum:   checksum.SHA256Base64URL([]byte("managed-app-apk-bytes-existing")),
					SizeBytes:  int64(len("managed-app-apk-bytes-existing")),
					MimeType:   "application/vnd.android.package-archive",
				},
				Checksum:  checksum.SHA256Base64URL([]byte("managed-app-apk-bytes-existing")),
				CreatedAt: versionCreatedAt,
			}},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Apps:      appStore,
		Artifacts: artifactStore,
		TenantID:  "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/apps/app-existing", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("app detail status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"App Detail", "Current app", "Versions", "Update app", "Retire app"} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("app detail should show %q: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), formatDashboardTime(appCreatedAt)) {
		t.Fatalf("app detail should show created time: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "com.example.catalog") {
		t.Fatalf("app detail should show package name: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "v101") {
		t.Fatalf("app detail should show version data: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/apps/app-existing/download"`) {
		t.Fatalf("app detail should include download link: %s", rr.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/admin/apps/app-existing/download", nil)
	downloadReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	downloadRes := httptest.NewRecorder()
	mux.ServeHTTP(downloadRes, downloadReq)
	if downloadRes.Code != http.StatusOK {
		t.Fatalf("dashboard app download status = %d, body=%s", downloadRes.Code, downloadRes.Body.String())
	}
	if got := downloadRes.Header().Get("Content-Disposition"); got != `attachment; filename="com.example.catalog-v101.apk"` {
		t.Fatalf("unexpected app download disposition: %q", got)
	}
	if got := downloadRes.Body.String(); got != "managed-app-apk-bytes-existing" {
		t.Fatalf("unexpected app download body: %q", got)
	}
}

func TestRegisterDashboardHidesPlainFilesPage(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/files", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected plain files page to be removed, got status %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterDashboardDeviceDetailEnrollmentQRDefaults(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Devices: &fakeDashboardDeviceStore{
			devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusPending, CreatedAt: time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC)}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1")}},
		},
		Policies: &fakeDashboardPolicyStore{
			policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Default policy"}},
		},
		Enrollment: &fakeDashboardEnrollmentStore{
			issued: enrollment.IssuedToken{
				Token: enrollment.Token{
					ID:        "token-generated",
					TenantID:  "tenant-1",
					Status:    enrollment.TokenStatusIssued,
					ExpiresAt: time.Date(2026, 5, 13, 13, 32, 0, 0, time.UTC),
				},
				Secret: "generated-secret",
			},
		},
		TenantID: "tenant-1",
	})

	getReq := httptest.NewRequest(http.MethodGet, "/admin/devices/device-1", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("device detail page status = %d, body=%s", getRR.Code, getRR.Body.String())
	}
	for _, field := range []string{`name="serverProject"`, `name="serverUrl"`, `name="ttl"`, `name="packageUrl"`, `name="packageChecksum"`, `name="deviceId"`, `name="deviceIdUse"`, `name="customer"`, `name="group"`, `name="enrollmentToken"`, `Issue token`, `Validate token`, `Revoke token`, `name="outputFormat"`} {
		if strings.Contains(getRR.Body.String(), field) {
			t.Fatalf("device detail page should not expose %q: %s", field, getRR.Body.String())
		}
	}
	csrf := mustGetCSRFCookie(t, getRR.Result())
	form := strings.NewReader("csrfToken=" + csrf)
	postReq := httptest.NewRequest(http.MethodPost, "/admin/devices/device-1/enrollment/qr", form)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("qr json status = %d, body=%s", postRR.Code, postRR.Body.String())
	}
	if !strings.Contains(postRR.Body.String(), "QR JSON") || !strings.Contains(postRR.Body.String(), "QR preview") {
		t.Fatalf("qr response should render both outputs: %s", postRR.Body.String())
	}
	if !strings.Contains(postRR.Body.String(), "com.xmdm.DEVICE_ID") || !strings.Contains(postRR.Body.String(), "device-1") {
		t.Fatalf("qr json should include device id: %s", postRR.Body.String())
	}
	if !strings.Contains(postRR.Body.String(), "com.xmdm.DEVICE_ID_USE") || !strings.Contains(postRR.Body.String(), "serial") {
		t.Fatalf("qr json should default device id use to serial: %s", postRR.Body.String())
	}
	if !strings.Contains(postRR.Body.String(), "https://mdm.example.com") || !strings.Contains(postRR.Body.String(), "abc123") {
		t.Fatalf("qr json should include hardcoded provisioning defaults: %s", postRR.Body.String())
	}
	if strings.Contains(postRR.Body.String(), "SERVER_PROJECT") || strings.Contains(postRR.Body.String(), "CUSTOMER") || strings.Contains(postRR.Body.String(), "GROUP") {
		t.Fatalf("qr json should not include removed optional bootstrap fields: %s", postRR.Body.String())
	}

	oldReq := httptest.NewRequest(http.MethodGet, "/admin/enrollment", nil)
	oldReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	oldRR := httptest.NewRecorder()
	mux.ServeHTTP(oldRR, oldReq)
	if oldRR.Code != http.StatusNotFound {
		t.Fatalf("old enrollment page should be removed, got %d body=%s", oldRR.Code, oldRR.Body.String())
	}
}

func TestRegisterDashboardAllowsUserLogin(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	store := &fakeDashboardIdentityStore{
		roles: []identity.Role{{RecordBase: identity.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"}, Name: "Operators", Permissions: []string{"admin.read", "admin.write"}}},
		authUsers: map[string]fakeDashboardAuthUser{
			"alice@example.com": {
				passwordHash: mustHashPassword(t, "alice-secret"),
				user: identity.User{
					RecordBase: identity.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
					Email:      "alice@example.com",
					RoleID:     "role-1",
				},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: store, TenantID: "tenant-1"})
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)

	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=alice@example.com&password=alice-secret"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	loginRes := httptest.NewRecorder()
	mux.ServeHTTP(loginRes, loginReq)

	if loginRes.Code != http.StatusSeeOther {
		t.Fatalf("dashboard user login status = %d, body=%s", loginRes.Code, loginRes.Body.String())
	}
	sessionCookie := mustGetSessionCookie(t, loginRes.Result())

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard user overview status = %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterDashboardHidesUpdateFormsForRetiredRecords(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Identity: &fakeDashboardIdentityStore{
			users: []identity.User{{RecordBase: identity.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "retired", CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Email: "alice@example.com", RoleID: "role-1"}},
			roles: []identity.Role{{RecordBase: identity.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "retired", CreatedAt: time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC)}, Name: "Operators", Permissions: []string{"admin.read"}}},
		},
		Policies: &fakeDashboardPolicyStore{
			policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "retired", CreatedAt: time.Date(2026, 5, 13, 11, 32, 0, 0, time.UTC)}, Name: "Default policy", Version: 1, Restrictions: json.RawMessage(`{"kioskExitPasscode":"1234"}`)}},
		},
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/users/user-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard retired user detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Update user") {
		t.Fatalf("retired user page should not show update form: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/roles/role-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard retired role detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Update role") {
		t.Fatalf("retired role page should not show update form: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/policies/policy-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard retired policy detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Update policy") || strings.Contains(rr.Body.String(), "Retire policy") {
		t.Fatalf("retired policy page should not show update forms: %s", rr.Body.String())
	}
}

func TestRegisterDashboardUpdatesUserWithoutPassword(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeDashboardIdentityStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: store, TenantID: "tenant-1"})
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)

	req := httptest.NewRequest(http.MethodPost, "/admin/users/user-1/update", strings.NewReader("csrfToken="+csrfToken+"&email=alice2@example.com&roleId=role-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("dashboard update status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/admin/users/user-1?ok=user+updated" {
		t.Fatalf("dashboard user update redirect = %q", got)
	}
	if store.updatedUser.PasswordHash != "" {
		t.Fatalf("empty password should preserve existing hash, got %q", store.updatedUser.PasswordHash)
	}
}

func TestRegisterDashboardUpdatesRoleWithoutRedirectingToList(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeDashboardIdentityStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: store, TenantID: "tenant-1"})
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)

	req := httptest.NewRequest(http.MethodPost, "/admin/roles/role-1/update", strings.NewReader("csrfToken="+csrfToken+"&name=operators&permissions=%5B%22admin.read%22%5D"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("dashboard role update status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/admin/roles/role-1?ok=role+updated" {
		t.Fatalf("dashboard role update redirect = %q", got)
	}
}

func TestRegisterDashboardUpdatesDeviceWithoutSecret(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive}, Name: "device-a", PolicyID: strPtr("policy-1")}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: store, TenantID: "tenant-1"})
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)

	req := httptest.NewRequest(http.MethodPost, "/admin/devices/device-1/update", strings.NewReader("csrfToken="+csrfToken+"&name=device-a&policyId=policy-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("dashboard device update status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != "/admin/devices/device-1?ok=device+updated" {
		t.Fatalf("dashboard device update redirect = %q", got)
	}
	if store.updatedDevice.SecretHash != "" {
		t.Fatalf("empty device secret should preserve existing hash, got %q", store.updatedDevice.SecretHash)
	}
}

func TestRegisterDashboardDevicesListUsesIdentityStyleColumns(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1")}},
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Default policy"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: deviceStore, Policies: policyStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard devices list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<th>Created</th><th>ID</th><th>Name</th><th>Policy</th><th>Status</th>") {
		t.Fatalf("devices list should use identity-style columns: %s", rr.Body.String())
	}
	for _, field := range []string{`/admin/devices/device-1`, `warehouse-tablet-001`, `/admin/policies/policy-1`, `Default policy`} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("devices list should contain %q: %s", field, rr.Body.String())
		}
	}
	if strings.Contains(rr.Body.String(), "Actions") {
		t.Fatalf("devices list should not expose action column: %s", rr.Body.String())
	}
}

func TestRegisterDashboardDevicesListUsesPendingStatusBadge(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusPending, CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1")}},
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Default policy"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: deviceStore, Policies: policyStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard devices list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status-pill status-pending") {
		t.Fatalf("devices list should render pending status as a pill: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), ">pending<") {
		t.Fatalf("devices list should show pending status text: %s", rr.Body.String())
	}
}

func TestRegisterDashboardGroupsListUsesIdentityStyleColumns(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	groupStore := &fakeDashboardGroupStore{
		items: []group.Group{
			{RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Name: "Field Devices"},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Groups: groupStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/groups", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard groups list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Created", "ID", "Name", "Status"} {
		if !strings.Contains(rr.Body.String(), "<th>"+field+"</th>") {
			t.Fatalf("groups list should contain %q header: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/groups/group-1"`) {
		t.Fatalf("groups list should link the name to the detail page: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Actions") {
		t.Fatalf("groups list should not expose action column: %s", rr.Body.String())
	}
}

func TestRegisterDashboardGroupDetailShowsMemberDevices(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	groupStore := &fakeDashboardGroupStore{
		items: []group.Group{
			{RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: "active", CreatedAt: time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)}, Name: "Field Devices"},
		},
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{
			{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 31, 0, 0, time.UTC)}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1"), GroupIDs: []string{"group-1"}},
			{RecordBase: device.RecordBase{ID: "device-2", TenantID: "tenant-1", Status: device.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 32, 0, 0, time.UTC)}, Name: "other-tablet", PolicyID: strPtr("policy-2")},
		},
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{
			{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Baseline"},
			{RecordBase: policy.RecordBase{ID: "policy-2", TenantID: "tenant-1", Status: "active"}, Name: "Kiosk"},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Groups: groupStore, Devices: deviceStore, Policies: policyStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/groups/group-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard group detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Group Detail", "Current group", "Update group", "Retire group", "Member devices", "Field Devices"} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("group detail should contain %q: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/devices/device-1"`) {
		t.Fatalf("group detail should link member devices: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `href="/admin/devices/device-2"`) {
		t.Fatalf("group detail should only show member devices: %s", rr.Body.String())
	}

	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)
	updateReq := httptest.NewRequest(http.MethodPost, "/admin/groups/group-1/update", strings.NewReader("csrfToken="+csrfToken+"&name=Field+Devices+Updated"))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	updateReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	updateRes := httptest.NewRecorder()
	mux.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusSeeOther {
		t.Fatalf("dashboard group update status = %d, body=%s", updateRes.Code, updateRes.Body.String())
	}
	if got := updateRes.Header().Get("Location"); got != "/admin/groups/group-1?ok=group+updated" {
		t.Fatalf("dashboard group update redirect = %q", got)
	}
}

func TestRegisterDashboardDeviceDetailShowsUpdateAndRetire(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusActive}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1")}},
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Default policy"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: deviceStore, Policies: policyStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices/device-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard device detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Device Detail", "Current device", "Update device", "Retire device", "Default policy"} {
		if !strings.Contains(rr.Body.String(), field) {
			t.Fatalf("device detail should contain %q: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), "Active policy") {
		t.Fatalf("device detail should show active policy section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/policies/policy-1"`) {
		t.Fatalf("device detail should link to active policy: %s", rr.Body.String())
	}
	unescaped := html.UnescapeString(rr.Body.String())
	if !strings.Contains(unescaped, "Config preview") || !strings.Contains(unescaped, `"deviceId": "device-1"`) || !strings.Contains(unescaped, `"name": "Default policy"`) {
		t.Fatalf("device detail should show the config preview: %s", rr.Body.String())
	}
	if strings.Contains(unescaped, `"signature"`) {
		t.Fatalf("device detail preview should not include a signature: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "deviceId") || !strings.Contains(rr.Body.String(), "device-1") {
		t.Fatalf("device detail should show the immutable device id: %s", rr.Body.String())
	}
}

func TestRegisterDashboardManagedFilesListUsesDetailPattern(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	artifactStore := &fakeDashboardArtifactStore{getContent: []byte("managed-file-bytes")}
	store := &fakeDashboardManagedFileStore{
		items: []managedfiles.ManagedFile{
			{
				RecordBase: managedfiles.RecordBase{ID: "mf-1", TenantID: "tenant-1", Status: managedfiles.StatusActive, CreatedAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 12, 5, 0, 0, time.UTC)},
				FileID:     "file-1",
				Path:       "/sdcard/xmdm/config.txt",
				File: &files.File{
					RecordBase: files.RecordBase{ID: "file-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 11, 56, 0, 0, time.UTC)},
					Name:       "config.txt",
					ArtifactID: "artifact-1",
					Checksum:   "checksum-1",
					MimeType:   "text/plain",
					Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 11, 51, 0, 0, time.UTC)}, StorageKey: "files/config.txt", Checksum: "checksum-1", SizeBytes: 8, MimeType: "text/plain"},
				},
				ReplaceVariables: true,
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{ManagedFiles: store, Artifacts: artifactStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/managed-files", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard managed files list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Created", "ID", "Path", "File", "Template", "Status"} {
		if !strings.Contains(rr.Body.String(), "<th>"+field+"</th>") {
			t.Fatalf("managed files list should contain %q header: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/managed-files/mf-1"`) {
		t.Fatalf("managed files list should link the path to the detail page: %s", rr.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/admin/managed-files/mf-1", nil)
	detailReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	detailRes := httptest.NewRecorder()
	mux.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("dashboard managed file detail status = %d, body=%s", detailRes.Code, detailRes.Body.String())
	}
	if !strings.Contains(detailRes.Body.String(), "Managed File Detail") || !strings.Contains(detailRes.Body.String(), "Current managed file") || !strings.Contains(detailRes.Body.String(), "Retire managed file") {
		t.Fatalf("managed file detail should show summary and retire action: %s", detailRes.Body.String())
	}
	if !strings.Contains(detailRes.Body.String(), `href="/admin/managed-files/mf-1/download"`) {
		t.Fatalf("managed file detail should include download link: %s", detailRes.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/admin/managed-files/mf-1/download", nil)
	downloadReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	downloadRes := httptest.NewRecorder()
	mux.ServeHTTP(downloadRes, downloadReq)
	if downloadRes.Code != http.StatusOK {
		t.Fatalf("dashboard managed file download status = %d, body=%s", downloadRes.Code, downloadRes.Body.String())
	}
	if got := downloadRes.Header().Get("Content-Disposition"); got != `attachment; filename="config.txt"` {
		t.Fatalf("unexpected managed file download disposition: %q", got)
	}
	if got := downloadRes.Body.String(); got != "managed-file-bytes" {
		t.Fatalf("unexpected managed file download body: %q", got)
	}
}

func TestRegisterDashboardCreatesManagedFileFromUpload(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	fileStore := &fakeDashboardFileStore{}
	managedStore := &fakeDashboardManagedFileStore{}
	artifactStore := &fakeDashboardArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Files: fileStore, ManagedFiles: managedStore, Artifacts: artifactStore, TenantID: "tenant-1"})

	getReq := httptest.NewRequest(http.MethodGet, "/admin/managed-files", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("dashboard managed files get status = %d, body=%s", getRR.Code, getRR.Body.String())
	}
	csrf := mustGetCSRFCookie(t, getRR.Result())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("csrfToken", csrf); err != nil {
		t.Fatalf("write csrf field: %v", err)
	}
	if err := writer.WriteField("path", "/sdcard/xmdm/config.txt"); err != nil {
		t.Fatalf("write path field: %v", err)
	}
	if err := writer.WriteField("replaceVariables", "on"); err != nil {
		t.Fatalf("write replaceVariables field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "config.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("managed file contents")); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/admin/managed-files/create", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("dashboard managed file create status = %d, body=%s", postRR.Code, postRR.Body.String())
	}
	if got := postRR.Header().Get("Location"); got != "/admin/managed-files?ok=managed+file+uploaded" {
		t.Fatalf("dashboard managed file create redirect = %q", got)
	}
	if !fileStore.createdCalled {
		t.Fatalf("expected backing file to be created")
	}
	if fileStore.createdFile.ID == "" || fileStore.createdFile.Name == "" {
		t.Fatalf("expected backing file record to be populated: %#v", fileStore.createdFile)
	}
	if artifactStore.putKey == "" {
		t.Fatalf("expected artifact upload to run")
	}
	if len(managedStore.items) != 1 {
		t.Fatalf("expected one managed file binding, got %#v", managedStore.items)
	}
	if managedStore.items[0].Path != "/sdcard/xmdm/config.txt" {
		t.Fatalf("unexpected managed file path: %#v", managedStore.items[0].Path)
	}
	if managedStore.items[0].FileID != fileStore.createdFile.ID {
		t.Fatalf("expected binding to use created file id: %#v", managedStore.items[0])
	}
	firstFileID := fileStore.createdFile.ID

	var replaceBody bytes.Buffer
	replaceWriter := multipart.NewWriter(&replaceBody)
	if err := replaceWriter.WriteField("csrfToken", csrf); err != nil {
		t.Fatalf("write replace csrf field: %v", err)
	}
	if err := replaceWriter.WriteField("path", "/sdcard/xmdm/config.txt"); err != nil {
		t.Fatalf("write replace path field: %v", err)
	}
	if err := replaceWriter.WriteField("replaceVariables", ""); err != nil {
		t.Fatalf("write replaceVariables field: %v", err)
	}
	replacePart, err := replaceWriter.CreateFormFile("file", "config.txt")
	if err != nil {
		t.Fatalf("create replace form file: %v", err)
	}
	if _, err := replacePart.Write([]byte("managed file contents v2")); err != nil {
		t.Fatalf("write replace content: %v", err)
	}
	if err := replaceWriter.Close(); err != nil {
		t.Fatalf("close replace multipart writer: %v", err)
	}

	replaceReq := httptest.NewRequest(http.MethodPost, "/admin/managed-files/create", &replaceBody)
	replaceReq.Header.Set("Content-Type", replaceWriter.FormDataContentType())
	replaceReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	replaceReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	replaceRR := httptest.NewRecorder()
	mux.ServeHTTP(replaceRR, replaceReq)
	if replaceRR.Code != http.StatusSeeOther {
		t.Fatalf("dashboard managed file replace status = %d, body=%s", replaceRR.Code, replaceRR.Body.String())
	}
	if got := replaceRR.Header().Get("Location"); got != "/admin/managed-files?ok=managed+file+uploaded" {
		t.Fatalf("dashboard managed file replace redirect = %q", got)
	}
	if len(managedStore.items) != 1 {
		t.Fatalf("expected one managed file binding after replace, got %#v", managedStore.items)
	}
	if managedStore.items[0].FileID != fileStore.createdFile.ID {
		t.Fatalf("expected managed file binding to point at replacement file: %#v first=%s current=%s", managedStore.items[0], firstFileID, fileStore.createdFile.ID)
	}
}

func TestRegisterDashboardCertificatesListUsesDetailPattern(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	artifactStore := &fakeDashboardArtifactStore{getContent: []byte("certificate-bytes")}
	store := &fakeDashboardCertificateStore{
		items: []certificates.Certificate{
			{
				RecordBase: certificates.RecordBase{ID: "cert-1", TenantID: "tenant-1", Status: certificates.StatusActive, CreatedAt: time.Date(2026, 5, 13, 12, 10, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 12, 15, 0, 0, time.UTC)},
				Name:       "Root CA",
				ArtifactID: "artifact-1",
				Checksum:   "checksum-1",
				Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-1", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 12, 6, 0, 0, time.UTC)}, StorageKey: "certs/root.pem", Checksum: "checksum-1", SizeBytes: 16, MimeType: "application/x-pem-file"},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Certificates: store, Artifacts: artifactStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/certificates", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard certificates list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	for _, field := range []string{"Created", "ID", "Name", "Artifact", "Status"} {
		if !strings.Contains(rr.Body.String(), "<th>"+field+"</th>") {
			t.Fatalf("certificates list should contain %q header: %s", field, rr.Body.String())
		}
	}
	if !strings.Contains(rr.Body.String(), `href="/admin/certificates/cert-1"`) {
		t.Fatalf("certificates list should link the name to the detail page: %s", rr.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/admin/certificates/cert-1", nil)
	detailReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	detailRes := httptest.NewRecorder()
	mux.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK {
		t.Fatalf("dashboard certificate detail status = %d, body=%s", detailRes.Code, detailRes.Body.String())
	}
	if !strings.Contains(detailRes.Body.String(), "Certificate Detail") || !strings.Contains(detailRes.Body.String(), "Current certificate") || !strings.Contains(detailRes.Body.String(), "Retire certificate") {
		t.Fatalf("certificate detail should show summary and retire action: %s", detailRes.Body.String())
	}
	if !strings.Contains(detailRes.Body.String(), `href="/admin/certificates/cert-1/download"`) {
		t.Fatalf("certificate detail should include download link: %s", detailRes.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/admin/certificates/cert-1/download", nil)
	downloadReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	downloadRes := httptest.NewRecorder()
	mux.ServeHTTP(downloadRes, downloadReq)
	if downloadRes.Code != http.StatusOK {
		t.Fatalf("dashboard certificate download status = %d, body=%s", downloadRes.Code, downloadRes.Body.String())
	}
	if got := downloadRes.Header().Get("Content-Disposition"); got != `attachment; filename="Root CA"` {
		t.Fatalf("unexpected certificate download disposition: %q", got)
	}
	if got := downloadRes.Body.String(); got != "certificate-bytes" {
		t.Fatalf("unexpected certificate download body: %q", got)
	}
}

func TestRegisterDashboardCertificateUploadDerivesArtifactMetadata(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeDashboardCertificateStore{}
	artifactStore := &fakeDashboardArtifactStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Certificates: store, Artifacts: artifactStore, TenantID: "tenant-1"})

	csrf := mustGetCSRFCookieFromDashboardLogin(t, mux)
	content := []byte("-----BEGIN CERTIFICATE-----\nplaywright\n-----END CERTIFICATE-----\n")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, kv := range [][2]string{
		{"csrfToken", csrf},
		{"name", "Root CA"},
	} {
		if err := writer.WriteField(kv[0], kv[1]); err != nil {
			t.Fatalf("write field %s: %v", kv[0], err)
		}
	}
	part, err := writer.CreateFormFile("file", "root-ca.pem")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write certificate content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/admin/certificates/create", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("certificate upload status = %d body=%s", postRR.Code, postRR.Body.String())
	}
	if got := postRR.Header().Get("Location"); got != "/admin/certificates?ok=certificate+uploaded" {
		t.Fatalf("certificate upload redirect = %q", got)
	}
	if store.created.Name != "Root CA" {
		t.Fatalf("certificate upload should forward name: %#v", store.created)
	}
	if store.created.StorageKey == "" || !strings.Contains(store.created.StorageKey, "artifacts/certificates/") {
		t.Fatalf("certificate upload should derive storage key: %#v", store.created)
	}
	if store.created.Checksum != checksum.SHA256Base64URL(content) {
		t.Fatalf("certificate upload should derive checksum: %#v", store.created)
	}
	if store.created.SizeBytes != int64(len(content)) {
		t.Fatalf("certificate upload should derive size: %#v", store.created)
	}
	if store.created.MimeType != "application/x-pem-file" {
		t.Fatalf("certificate upload should derive mime type: %#v", store.created)
	}
	if artifactStore.putKey != store.created.StorageKey {
		t.Fatalf("certificate upload should store artifact at derived key: key=%q created=%#v", artifactStore.putKey, store.created)
	}
}

func TestRegisterDashboardHidesUpdateFormsForRetiredDevice(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	deviceStore := &fakeDashboardDeviceStore{
		devices: []device.Device{{RecordBase: device.RecordBase{ID: "device-1", TenantID: "tenant-1", Status: device.StatusRetired}, Name: "warehouse-tablet-001", PolicyID: strPtr("policy-1")}},
	}
	policyStore := &fakeDashboardPolicyStore{
		policies: []policy.Policy{{RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: "active"}, Name: "Default policy"}},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: deviceStore, Policies: policyStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodGet, "/admin/devices/device-1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard retired device detail status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Update device") || strings.Contains(rr.Body.String(), "Retire device") {
		t.Fatalf("retired device page should not show update forms: %s", rr.Body.String())
	}
}

func TestRegisterDashboardCreatesDeviceWithoutSecret(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeDashboardDeviceStore{}
	groupStore := &fakeDashboardGroupStore{
		items: []group.Group{
			{RecordBase: group.RecordBase{ID: "group-1", TenantID: "tenant-1", Status: "active"}, Name: "Field"},
			{RecordBase: group.RecordBase{ID: "group-2", TenantID: "tenant-1", Status: "active"}, Name: "Kiosk"},
			{RecordBase: group.RecordBase{ID: "group-3", TenantID: "tenant-1", Status: "retired"}, Name: "Legacy"},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Devices: store, Groups: groupStore, TenantID: "tenant-1"})
	csrfToken := mustGetCSRFCookieFromDashboardLogin(t, mux)

	getReq := httptest.NewRequest(http.MethodGet, "/admin/devices", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("dashboard device create form status = %d, body=%s", getRR.Code, getRR.Body.String())
	}
	if strings.Contains(getRR.Body.String(), "Legacy") || strings.Contains(getRR.Body.String(), "group-3") {
		t.Fatalf("dashboard device create page should not show retired groups: %s", getRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/devices/create", strings.NewReader("csrfToken="+csrfToken+"&name=device-a&policyId=policy-1&groupIds=group-1&groupIds=group-2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("dashboard device create status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if store.createdDevice.Name != "device-a" {
		t.Fatalf("unexpected created device display name: %#v", store.createdDevice)
	}
	if store.createdDevice.PolicyID != "policy-1" {
		t.Fatalf("unexpected created device policy: %#v", store.createdDevice)
	}
	if store.createdDevice.SecretHash == "" {
		t.Fatal("expected generated device secret hash")
	}
	if !slices.Equal(store.createdDevice.GroupIDs, []string{"group-1", "group-2"}) {
		t.Fatalf("unexpected created device groups: %#v", store.createdDevice.GroupIDs)
	}
}

func TestRegisterDashboardRejectsMutationWithoutCSRF(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: &fakeDashboardIdentityStore{}, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/create", strings.NewReader("email=a@example.com&password=secret&roleId=role-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard csrf failure status = %d, want rendered forbidden page", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("csrf failure missing forbidden body: %s", rr.Body.String())
	}
}

func TestRegisterDashboardRejectsReadOnlySessionForUserMutation(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: &fakeDashboardIdentityStore{}, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/create", strings.NewReader("csrfToken=csrf&email=a@example.com&password=secret&roleId=role-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard read-only user mutation status = %d, want rendered forbidden page", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("read-only user mutation missing forbidden body: %s", rr.Body.String())
	}
}

func TestRegisterDashboardRejectsReadOnlySessionForRoleMutation(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Identity: &fakeDashboardIdentityStore{}, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodPost, "/admin/roles/create", strings.NewReader("csrfToken=csrf&name=role-a&permissions=%5B%22admin.read%22%5D"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard read-only role mutation status = %d, want rendered forbidden page", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("read-only role mutation missing forbidden body: %s", rr.Body.String())
	}
}

type fakeAdminCommandStore struct {
	items    []commands.Command
	enqueues []commands.Upsert
}

func (s *fakeAdminCommandStore) Enqueue(_ context.Context, _ string, req commands.Upsert) ([]commands.Command, error) {
	s.enqueues = append(s.enqueues, req)
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeAdminCommandStore) ListRecent(context.Context, string, int) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeAdminCommandStore) Get(_ context.Context, _ string, commandID string) (commands.Command, error) {
	for _, item := range s.items {
		if item.ID == commandID {
			return item, nil
		}
	}
	return commands.Command{}, httpx.ErrNotFound
}

func (s *fakeAdminCommandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *fakeAdminCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}

type fakeAuditStore struct {
	events []audit.Event
}

func (s *fakeAuditStore) Record(context.Context, string, string, string, string, string, map[string]any) (audit.Event, error) {
	return audit.Event{}, nil
}

func (s *fakeAuditStore) List(context.Context, string) ([]audit.Event, error) {
	return append([]audit.Event(nil), s.events...), nil
}

var _ commands.Repository = (*fakeAdminCommandStore)(nil)
var _ audit.Store = (*fakeAuditStore)(nil)

type fakeDashboardDeviceStore struct {
	devices       []device.Device
	createdDevice device.DeviceUpsert
	updatedDevice device.DeviceUpsert
}

func (s *fakeDashboardDeviceStore) ListDevices(context.Context, string) ([]device.Device, error) {
	return append([]device.Device(nil), s.devices...), nil
}

func (s *fakeDashboardDeviceStore) CreateDevice(_ context.Context, _ string, req device.DeviceUpsert) (device.Device, error) {
	s.createdDevice = req
	return device.Device{RecordBase: device.RecordBase{ID: "device-new", Status: device.StatusPending}, Name: req.Name, PolicyID: strPtr(req.PolicyID), GroupIDs: append([]string(nil), req.GroupIDs...)}, nil
}

func (s *fakeDashboardDeviceStore) UpdateDevice(_ context.Context, _ string, id string, req device.DeviceUpsert) (device.Device, error) {
	s.updatedDevice = req
	return device.Device{RecordBase: device.RecordBase{ID: id, Status: device.StatusActive}, Name: req.Name, PolicyID: strPtr(req.PolicyID), GroupIDs: append([]string(nil), req.GroupIDs...)}, nil
}

func (s *fakeDashboardDeviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, nil
}

func (s *fakeDashboardDeviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return device.Device{}, nil
}

var _ device.Repository = (*fakeDashboardDeviceStore)(nil)

type fakeDashboardManagedFileStore struct {
	items []managedfiles.ManagedFile
}

func (s *fakeDashboardManagedFileStore) ListManagedFiles(context.Context, string) ([]managedfiles.ManagedFile, error) {
	return append([]managedfiles.ManagedFile(nil), s.items...), nil
}

func (s *fakeDashboardManagedFileStore) GetManagedFile(_ context.Context, _ string, id string) (managedfiles.ManagedFile, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return managedfiles.ManagedFile{}, httpx.ErrNotFound
}

func (s *fakeDashboardManagedFileStore) CreateManagedFile(_ context.Context, tenantID string, req managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	item := managedfiles.ManagedFile{
		RecordBase:       managedfiles.RecordBase{ID: "mf-created", TenantID: tenantID, Status: managedfiles.StatusActive},
		FileID:           req.FileID,
		Path:             req.Path,
		ReplaceVariables: req.ReplaceVariables,
	}
	for i, existing := range s.items {
		if existing.Path == req.Path {
			item.RecordBase = existing.RecordBase
			item.FileID = req.FileID
			item.Path = req.Path
			item.ReplaceVariables = req.ReplaceVariables
			s.items[i] = item
			return item, nil
		}
	}
	s.items = append(s.items, item)
	return item, nil
}

func (s *fakeDashboardManagedFileStore) RetireManagedFile(_ context.Context, _ string, id string) (managedfiles.ManagedFile, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return managedfiles.ManagedFile{}, httpx.ErrNotFound
}

var _ managedfiles.Repository = (*fakeDashboardManagedFileStore)(nil)

type fakeDashboardCertificateStore struct {
	items        []certificates.Certificate
	created      certificates.CertificateUpsert
	createCalled bool
}

func (s *fakeDashboardCertificateStore) ListCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return append([]certificates.Certificate(nil), s.items...), nil
}

func (s *fakeDashboardCertificateStore) ListActiveCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return append([]certificates.Certificate(nil), s.items...), nil
}

func (s *fakeDashboardCertificateStore) GetCertificate(_ context.Context, _ string, id string) (certificates.Certificate, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return certificates.Certificate{}, httpx.ErrNotFound
}

func (s *fakeDashboardCertificateStore) CreateCertificate(_ context.Context, _ string, req certificates.CertificateUpsert) (certificates.Certificate, error) {
	s.created = req
	s.createCalled = true
	return certificates.Certificate{
		RecordBase: certificates.RecordBase{ID: "cert-created", TenantID: "tenant-1", Status: certificates.StatusActive, CreatedAt: time.Date(2026, 5, 13, 12, 20, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 12, 20, 0, 0, time.UTC)},
		Name:       req.Name,
		ArtifactID: "artifact-created",
		Checksum:   req.Checksum,
		Artifact:   &files.Artifact{RecordBase: files.RecordBase{ID: "artifact-created", TenantID: "tenant-1", Status: files.StatusActive, UpdatedAt: time.Date(2026, 5, 13, 12, 20, 0, 0, time.UTC)}, StorageKey: req.StorageKey, Checksum: req.Checksum, SizeBytes: req.SizeBytes, MimeType: req.MimeType},
	}, nil
}

func (s *fakeDashboardCertificateStore) RetireCertificate(_ context.Context, _ string, id string) (certificates.Certificate, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return certificates.Certificate{}, httpx.ErrNotFound
}

var _ certificates.Repository = (*fakeDashboardCertificateStore)(nil)

type fakeDashboardPolicyStore struct {
	policies           []policy.Policy
	policyApps         []policy.PolicyApp
	policyCertificates []policy.PolicyCertificate
	policyManagedFiles []policy.PolicyManagedFile
}

func (s *fakeDashboardPolicyStore) ListPolicies(context.Context, string) ([]policy.Policy, error) {
	return append([]policy.Policy(nil), s.policies...), nil
}

func (s *fakeDashboardPolicyStore) GetPolicy(_ context.Context, _ string, id string) (policy.Policy, error) {
	for _, item := range s.policies {
		if item.ID == id {
			return item, nil
		}
	}
	return policy.Policy{}, httpx.ErrNotFound
}

func (s *fakeDashboardPolicyStore) CreatePolicy(context.Context, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakeDashboardPolicyStore) UpdatePolicy(context.Context, string, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakeDashboardPolicyStore) RetirePolicy(context.Context, string, string) (policy.Policy, error) {
	return policy.Policy{}, nil
}

func (s *fakeDashboardPolicyStore) ListPolicyApps(context.Context, string, string) ([]policy.PolicyApp, error) {
	return append([]policy.PolicyApp(nil), s.policyApps...), nil
}

func (s *fakeDashboardPolicyStore) AddPolicyApp(_ context.Context, tenantID string, policyID string, appID string) (policy.PolicyApp, error) {
	for i := range s.policyApps {
		if s.policyApps[i].TenantID == tenantID && s.policyApps[i].PolicyID == policyID && s.policyApps[i].AppID == appID {
			s.policyApps[i].Status = policy.StatusActive
			return s.policyApps[i], nil
		}
	}
	item := policy.PolicyApp{
		RecordBase: policy.RecordBase{ID: "policy-app-new", TenantID: tenantID, Status: policy.StatusActive},
		PolicyID:   policyID,
		AppID:      appID,
	}
	s.policyApps = append(s.policyApps, item)
	return item, nil
}

func (s *fakeDashboardPolicyStore) RemovePolicyApp(_ context.Context, tenantID string, policyID string, appID string) error {
	for i := range s.policyApps {
		if s.policyApps[i].TenantID == tenantID && s.policyApps[i].PolicyID == policyID && s.policyApps[i].AppID == appID {
			s.policyApps[i].Status = "disabled"
			return nil
		}
	}
	return httpx.ErrNotFound
}

func (s *fakeDashboardPolicyStore) ListPolicyCertificates(context.Context, string, string) ([]policy.PolicyCertificate, error) {
	return append([]policy.PolicyCertificate(nil), s.policyCertificates...), nil
}

func (s *fakeDashboardPolicyStore) AddPolicyCertificate(_ context.Context, tenantID string, policyID string, certificateID string) (policy.PolicyCertificate, error) {
	for i := range s.policyCertificates {
		if s.policyCertificates[i].TenantID == tenantID && s.policyCertificates[i].PolicyID == policyID && s.policyCertificates[i].CertificateID == certificateID {
			s.policyCertificates[i].Status = policy.StatusActive
			return s.policyCertificates[i], nil
		}
	}
	item := policy.PolicyCertificate{
		RecordBase:    policy.RecordBase{ID: "policy-cert-new", TenantID: tenantID, Status: policy.StatusActive},
		PolicyID:      policyID,
		CertificateID: certificateID,
	}
	s.policyCertificates = append(s.policyCertificates, item)
	return item, nil
}

func (s *fakeDashboardPolicyStore) RemovePolicyCertificate(_ context.Context, tenantID string, policyID string, certificateID string) error {
	for i := range s.policyCertificates {
		if s.policyCertificates[i].TenantID == tenantID && s.policyCertificates[i].PolicyID == policyID && s.policyCertificates[i].CertificateID == certificateID {
			s.policyCertificates[i].Status = "disabled"
			return nil
		}
	}
	return httpx.ErrNotFound
}

func (s *fakeDashboardPolicyStore) ListPolicyManagedFiles(context.Context, string, string) ([]policy.PolicyManagedFile, error) {
	return append([]policy.PolicyManagedFile(nil), s.policyManagedFiles...), nil
}

func (s *fakeDashboardPolicyStore) AddPolicyManagedFile(_ context.Context, tenantID string, policyID string, managedFileID string) (policy.PolicyManagedFile, error) {
	for i := range s.policyManagedFiles {
		if s.policyManagedFiles[i].TenantID == tenantID && s.policyManagedFiles[i].PolicyID == policyID && s.policyManagedFiles[i].ManagedFileID == managedFileID {
			s.policyManagedFiles[i].Status = policy.StatusActive
			return s.policyManagedFiles[i], nil
		}
	}
	item := policy.PolicyManagedFile{
		RecordBase:    policy.RecordBase{ID: "policy-file-new", TenantID: tenantID, Status: policy.StatusActive},
		PolicyID:      policyID,
		ManagedFileID: managedFileID,
	}
	s.policyManagedFiles = append(s.policyManagedFiles, item)
	return item, nil
}

func (s *fakeDashboardPolicyStore) RemovePolicyManagedFile(_ context.Context, tenantID string, policyID string, managedFileID string) error {
	for i := range s.policyManagedFiles {
		if s.policyManagedFiles[i].TenantID == tenantID && s.policyManagedFiles[i].PolicyID == policyID && s.policyManagedFiles[i].ManagedFileID == managedFileID {
			s.policyManagedFiles[i].Status = "disabled"
			return nil
		}
	}
	return httpx.ErrNotFound
}

var _ policy.Repository = (*fakeDashboardPolicyStore)(nil)

type fakeDashboardGroupStore struct {
	items []group.Group
}

func (s *fakeDashboardGroupStore) ListGroups(context.Context, string) ([]group.Group, error) {
	return append([]group.Group(nil), s.items...), nil
}

func (s *fakeDashboardGroupStore) CreateGroup(context.Context, string, group.GroupUpsert) (group.Group, error) {
	return group.Group{}, nil
}

func (s *fakeDashboardGroupStore) UpdateGroup(_ context.Context, tenantID, id string, req group.GroupUpsert) (group.Group, error) {
	return group.Group{RecordBase: group.RecordBase{ID: id, TenantID: tenantID, Status: "active"}, Name: req.Name}, nil
}

func (s *fakeDashboardGroupStore) RetireGroup(context.Context, string, string) (group.Group, error) {
	return group.Group{}, nil
}

var _ group.Repository = (*fakeDashboardGroupStore)(nil)

type fakeDashboardAppStore struct {
	apps              []apps.App
	versions          map[string][]apps.Version
	createAppCalls    int
	createdApp        apps.AppUpsert
	createdVersion    apps.VersionUpsert
	createdVersionApp string
	retiredAppIDs     []string
}

func (s *fakeDashboardAppStore) ListApps(context.Context, string) ([]apps.App, error) {
	return append([]apps.App(nil), s.apps...), nil
}

func (s *fakeDashboardAppStore) GetApp(_ context.Context, _ string, id string) (apps.App, error) {
	for _, item := range s.apps {
		if item.ID == id {
			return item, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}

func (s *fakeDashboardAppStore) CreateApp(_ context.Context, _ string, req apps.AppUpsert) (apps.App, error) {
	s.createAppCalls++
	s.createdApp = req
	return apps.App{RecordBase: apps.RecordBase{ID: "app-created", TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC)}, PackageName: req.PackageName, Name: req.Name}, nil
}

func (s *fakeDashboardAppStore) UpdateApp(_ context.Context, _ string, id string, req apps.AppUpsert) (apps.App, error) {
	return apps.App{RecordBase: apps.RecordBase{ID: id, TenantID: "tenant-1", Status: apps.StatusActive, CreatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC)}, PackageName: req.PackageName, Name: req.Name}, nil
}

func (s *fakeDashboardAppStore) RetireApp(_ context.Context, _ string, id string) (apps.App, error) {
	s.retiredAppIDs = append(s.retiredAppIDs, id)
	return apps.App{RecordBase: apps.RecordBase{ID: id, TenantID: "tenant-1", Status: apps.StatusRetired, CreatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 5, 13, 11, 34, 0, 0, time.UTC)}}, nil
}

func (s *fakeDashboardAppStore) ListVersions(_ context.Context, _ string, appID string) ([]apps.Version, error) {
	if s.versions == nil {
		return nil, nil
	}
	return append([]apps.Version(nil), s.versions[appID]...), nil
}

func (s *fakeDashboardAppStore) GetVersion(_ context.Context, _ string, appID string, versionID string) (apps.Version, error) {
	if s.versions == nil {
		return apps.Version{}, httpx.ErrNotFound
	}
	for id, items := range s.versions {
		if id != appID {
			continue
		}
		for _, item := range items {
			if item.ID == versionID {
				return item, nil
			}
		}
	}
	return apps.Version{}, httpx.ErrNotFound
}

func (s *fakeDashboardAppStore) CreateVersion(_ context.Context, _ string, appID string, req apps.VersionUpsert) (apps.Version, error) {
	s.createdVersionApp = appID
	s.createdVersion = req
	artifactID := ""
	if req.ArtifactID != nil {
		artifactID = *req.ArtifactID
	}
	status := apps.VersionStatusUploaded
	if req.Publish {
		status = apps.VersionStatusPublished
	}
	return apps.Version{
		ID:          "version-created",
		TenantID:    "tenant-1",
		AppID:       appID,
		Status:      status,
		VersionName: req.VersionName,
		VersionCode: req.VersionCode,
		ArtifactID:  &artifactID,
		Checksum:    req.Checksum,
	}, nil
}

var _ apps.Repository = (*fakeDashboardAppStore)(nil)

type fakeDashboardFileStore struct {
	createdCalled bool
	createdFile   files.File
	createdCount  int
	retiredIDs    []string
}

func (s *fakeDashboardFileStore) ListFiles(context.Context, string) ([]files.File, error) {
	return nil, nil
}

func (s *fakeDashboardFileStore) GetFile(context.Context, string, string) (files.File, error) {
	return files.File{}, nil
}

func (s *fakeDashboardFileStore) CreateFile(_ context.Context, _ string, req files.FileUpsert) (files.File, error) {
	s.createdCalled = true
	s.createdCount++
	fileID := "file-created-" + strconv.Itoa(s.createdCount)
	artifactID := "artifact-created-" + strconv.Itoa(s.createdCount)
	file := files.File{
		RecordBase: files.RecordBase{ID: fileID, TenantID: "tenant-1", Status: files.StatusActive},
		Name:       req.Name,
		ArtifactID: artifactID,
		Checksum:   req.Checksum,
		MimeType:   req.MimeType,
		Artifact: &files.Artifact{
			RecordBase: files.RecordBase{ID: artifactID, TenantID: "tenant-1", Status: files.StatusActive},
			StorageKey: req.StorageKey,
			Checksum:   req.Checksum,
			SizeBytes:  req.SizeBytes,
			MimeType:   req.MimeType,
		},
	}
	s.createdFile = file
	return file, nil
}

func (s *fakeDashboardFileStore) RetireFile(_ context.Context, _ string, id string) (files.File, error) {
	s.retiredIDs = append(s.retiredIDs, id)
	return s.createdFile, nil
}

var _ files.Repository = (*fakeDashboardFileStore)(nil)

type fakeDashboardArtifactStore struct {
	putKey     string
	getContent []byte
}

func (s *fakeDashboardArtifactStore) Put(_ context.Context, key string, _ io.Reader, _ string, _ int64) error {
	s.putKey = key
	return nil
}

func (s *fakeDashboardArtifactStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.getContent)), nil
}

func (s *fakeDashboardArtifactStore) Delete(context.Context, string) error {
	return nil
}

var _ artifacts.Store = (*fakeDashboardArtifactStore)(nil)

type fakeDashboardEnrollmentStore struct {
	tokens []enrollment.Token
	issued enrollment.IssuedToken
}

func (s *fakeDashboardEnrollmentStore) IssueToken(context.Context, string, time.Time) (enrollment.IssuedToken, error) {
	return s.issued, nil
}

func (s *fakeDashboardEnrollmentStore) ValidateToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{}, nil
}

func (s *fakeDashboardEnrollmentStore) ConsumeToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{}, nil
}

func (s *fakeDashboardEnrollmentStore) BindDevice(context.Context, string, string, string, map[string]any) (enrollment.BoundDevice, error) {
	return enrollment.BoundDevice{}, nil
}

func (s *fakeDashboardEnrollmentStore) ListTokens(context.Context, string) ([]enrollment.Token, error) {
	return append([]enrollment.Token(nil), s.tokens...), nil
}

func (s *fakeDashboardEnrollmentStore) RevokeToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{}, nil
}

func (s *fakeDashboardEnrollmentStore) ExpireTokens(context.Context, time.Time) (int64, error) {
	return 0, nil
}

var _ enrollment.Repository = (*fakeDashboardEnrollmentStore)(nil)

func strPtr(value string) *string {
	return &value
}

type fakeDashboardIdentityStore struct {
	users       []identity.User
	roles       []identity.Role
	authUsers   map[string]fakeDashboardAuthUser
	updatedUser identity.UserUpsert
}

type fakeDashboardAuthUser struct {
	passwordHash string
	user         identity.User
}

func (s *fakeDashboardIdentityStore) ListUsers(context.Context, string) ([]identity.User, error) {
	return append([]identity.User(nil), s.users...), nil
}

func (s *fakeDashboardIdentityStore) CreateUser(context.Context, string, identity.UserUpsert) (identity.User, error) {
	return identity.User{}, nil
}

func (s *fakeDashboardIdentityStore) UpdateUser(_ context.Context, _ string, id string, req identity.UserUpsert) (identity.User, error) {
	s.updatedUser = req
	status := "active"
	for _, user := range s.users {
		if user.ID == id {
			status = user.Status
			break
		}
	}
	if status != "active" {
		return identity.User{}, httpx.ErrNotFound
	}
	return identity.User{RecordBase: identity.RecordBase{ID: id, Status: status}, Email: req.Email, RoleID: req.RoleID}, nil
}

func (s *fakeDashboardIdentityStore) RetireUser(context.Context, string, string) (identity.User, error) {
	return identity.User{}, nil
}

func (s *fakeDashboardIdentityStore) AuthenticateUser(_ context.Context, _ string, email, password string) (identity.User, identity.Role, error) {
	rec, ok := s.authUsers[email]
	if !ok || !identity.VerifyPassword(rec.passwordHash, password) {
		return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
	}
	for _, role := range s.roles {
		if role.ID == rec.user.RoleID {
			return rec.user, role, nil
		}
	}
	return identity.User{}, identity.Role{}, identity.ErrInvalidCredentials
}

func (s *fakeDashboardIdentityStore) ListRoles(context.Context, string) ([]identity.Role, error) {
	return append([]identity.Role(nil), s.roles...), nil
}

func (s *fakeDashboardIdentityStore) CreateRole(context.Context, string, identity.RoleUpsert) (identity.Role, error) {
	return identity.Role{}, nil
}

func (s *fakeDashboardIdentityStore) UpdateRole(_ context.Context, _ string, id string, req identity.RoleUpsert) (identity.Role, error) {
	status := "active"
	for _, role := range s.roles {
		if role.ID == id {
			status = role.Status
			break
		}
	}
	if status != "active" {
		return identity.Role{}, httpx.ErrNotFound
	}
	return identity.Role{RecordBase: identity.RecordBase{ID: id, Status: status}, Name: req.Name, Permissions: req.Permissions}, nil
}

func (s *fakeDashboardIdentityStore) RetireRole(context.Context, string, string) (identity.Role, error) {
	return identity.Role{}, nil
}

var _ identity.Repository = (*fakeDashboardIdentityStore)(nil)

func mustGetCSRFCookieFromLogin(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/login", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return mustGetCSRFCookie(t, rr.Result())
}

func mustGetCSRFCookieFromDashboardLogin(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return mustGetCSRFCookie(t, rr.Result())
}

func mustGetCSRFCookie(t *testing.T, res *http.Response) string {
	t.Helper()
	for _, cookie := range res.Cookies() {
		if cookie.Name == csrfCookieName {
			return cookie.Value
		}
	}
	t.Fatalf("missing csrf cookie")
	return ""
}

func mustGetSessionCookie(t *testing.T, res *http.Response) string {
	t.Helper()
	for _, cookie := range res.Cookies() {
		if cookie.Name == auth.SessionCookieName {
			return cookie.Value
		}
	}
	t.Fatalf("missing session cookie")
	return ""
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := identity.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return hash
}
