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

type fakeAdminCommandStore struct {
	items []commands.Command
}

func (s *fakeAdminCommandStore) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return append([]commands.Command(nil), s.items...), nil
}

func (s *fakeAdminCommandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *fakeAdminCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}

var _ commands.Repository = (*fakeAdminCommandStore)(nil)
