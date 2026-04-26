package adminhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestRegisterServesCommandForm(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, nil, "tenant-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/commands", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if !strings.Contains(rr.Body.String(), "Create Command") {
		t.Fatalf("missing form content: %s", rr.Body.String())
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

	form := strings.NewReader("type=reboot&targetType=group&targetGroupId=group-123&payload=%7B%22force%22%3Atrue%7D")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRegisterRejectsInvalidExpiresAt(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader(`{"type":"ping","expiresAt":"not-a-timestamp","target":{"type":"device","deviceId":"device-1"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d body=%s", rr.Code, rr.Body.String())
	}
	if len(store.enqueues) != 0 {
		t.Fatalf("expected no enqueue on invalid expiresAt, got %#v", store.enqueues)
	}
}

func TestRegisterRejectsPastExpiresAt(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	store := &fakeAdminCommandStore{}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), svc, nil, nil, store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commands", strings.NewReader(`{"type":"ping","expiresAt":"2024-01-01T00:00:00Z","target":{"type":"device","deviceId":"device-1"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d body=%s", rr.Code, rr.Body.String())
	}
	if len(store.enqueues) != 0 {
		t.Fatalf("expected no enqueue on past expiresAt, got %#v", store.enqueues)
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

func (s *fakeAdminCommandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *fakeAdminCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}

var _ commands.Repository = (*fakeAdminCommandStore)(nil)
