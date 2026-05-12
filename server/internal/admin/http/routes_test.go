package adminhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/httpx"
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

func mustGetCSRFCookieFromLogin(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/login", nil)
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
