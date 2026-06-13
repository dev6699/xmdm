package adminhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
)

func TestCommandSummaryShowsTransportSource(t *testing.T) {
	found := commands.Command{
		ID:        "cmd-1",
		Type:      "ping",
		Status:    commands.StatusAcked,
		CreatedAt: time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 13, 10, 1, 0, 0, time.UTC),
		AckedAt:   timePtr(time.Date(2026, 6, 13, 10, 1, 0, 0, time.UTC)),
		Result: map[string]any{
			"status":  "acked",
			"details": map[string]any{"transportSource": "mqtt"},
		},
	}

	html := string(commandSummary(found, device.Device{}))
	if !strings.Contains(html, "Transport") {
		t.Fatalf("command summary missing transport field: %s", html)
	}
	if !strings.Contains(html, "mqtt") {
		t.Fatalf("command summary missing transport source: %s", html)
	}
}

func TestCreateCommandRequiresWritePermissionAndCSRF(t *testing.T) {
	t.Run("requires write permission", func(t *testing.T) {
		svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
		session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
		cmds := &recordingCommandStore{result: []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued}}}
		mux := http.NewServeMux()
		RegisterDashboard(mux, svc, DashboardDependencies{Commands: cmds, Audit: &recordingAuditStore{}, TenantID: "tenant-1"})

		req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=ping&targetType=device&targetDeviceId=device-1&payload=%7B%7D&csrfToken=token"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if len(cmds.enqueued) != 0 {
			t.Fatalf("expected no command enqueue for read-only session, got %#v", cmds.enqueued)
		}
		if !strings.Contains(rr.Body.String(), "forbidden") {
			t.Fatalf("expected forbidden render, got %s", rr.Body.String())
		}
	})

	t.Run("requires csrf token", func(t *testing.T) {
		svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
		session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
		cmds := &recordingCommandStore{result: []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued}}}
		mux := http.NewServeMux()
		RegisterDashboard(mux, svc, DashboardDependencies{Commands: cmds, Audit: &recordingAuditStore{}, TenantID: "tenant-1"})

		req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=ping&targetType=device&targetDeviceId=device-1&payload=%7B%7D"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if len(cmds.enqueued) != 0 {
			t.Fatalf("expected no command enqueue without csrf, got %#v", cmds.enqueued)
		}
		if !strings.Contains(rr.Body.String(), "forbidden") {
			t.Fatalf("expected forbidden render, got %s", rr.Body.String())
		}
	})
}

func TestCreateCommandRejectsUnsupportedPluginCommandType(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	cmds := &recordingCommandStore{result: []commands.Command{{ID: "cmd-1", Type: "remote-lock", Status: commands.StatusQueued}}}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Commands: cmds,
		Audit:    &recordingAuditStore{},
		PluginManager: plugins.New(plugins.Plugin{
			ID:      "remote-control",
			Name:    "Remote Control",
			Enabled: true,
			CommandTypes: []plugins.CommandType{
				{Type: "remote-lock", Label: "Remote Lock", RequiredPermission: "admin:remote-control"},
			},
		}),
		TenantID: "tenant-1",
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=remote-lock&targetType=device&targetDeviceId=device-1&payload=%7B%7D&csrfToken=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for rejected plugin command type, got %d body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/admin/commands?error=unsupported+command+type") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
	if len(cmds.enqueued) != 0 {
		t.Fatalf("expected no enqueue for rejected plugin command type, got %#v", cmds.enqueued)
	}
}

func TestCreateCommandRecordsAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	cmds := &recordingCommandStore{result: []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued, DeviceID: "device-1"}}}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{Commands: cmds, Audit: auditStore, TenantID: "tenant-1"})

	req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("type=ping&targetType=device&targetDeviceId=device-1&payload=%7B%7D&csrfToken=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for successful command create, got %d body=%s", rr.Code, rr.Body.String())
	}
	if len(cmds.enqueued) != 1 {
		t.Fatalf("expected one enqueue, got %#v", cmds.enqueued)
	}
	if len(auditStore.records) != 1 {
		t.Fatalf("expected one audit record, got %#v", auditStore.records)
	}
	record := auditStore.records[0]
	if record.Action != "create" || record.ResourceType != "commands" || record.ResourceID != "cmd-1" {
		t.Fatalf("unexpected audit record: %#v", record)
	}
	if got := record.Details["deviceId"]; got != "device-1" {
		t.Fatalf("unexpected audit details: %#v", record.Details)
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

type recordingCommandStore struct {
	enqueued []commands.Upsert
	result   []commands.Command
}

func (s *recordingCommandStore) Enqueue(_ context.Context, _ string, req commands.Upsert) ([]commands.Command, error) {
	s.enqueued = append(s.enqueued, req)
	return append([]commands.Command(nil), s.result...), nil
}

func (s *recordingCommandStore) ListRecent(context.Context, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (s *recordingCommandStore) ListRecentAll(context.Context, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *recordingCommandStore) ListPendingForDevice(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (s *recordingCommandStore) GetOverviewStats(context.Context, string) (commands.OverviewStats, error) {
	return commands.OverviewStats{}, nil
}

func (s *recordingCommandStore) Get(context.Context, string, string) (commands.Command, error) {
	return commands.Command{}, httpx.ErrNotFound
}

func (s *recordingCommandStore) ListPending(context.Context, string, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (s *recordingCommandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, httpx.ErrNotFound
}
