package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"
)

func TestNewMuxRateLimitsBrowserAdminLogin(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	handler := NewMux(svc, Dependencies{
		PluginManager: plugins.Disabled(),
	})

	tokenReq := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	tokenReq.RemoteAddr = "198.51.100.20:1234"
	tokenRes := httptest.NewRecorder()
	handler.ServeHTTP(tokenRes, tokenReq)
	if tokenRes.Code != http.StatusOK {
		t.Fatalf("token request status = %d, want %d", tokenRes.Code, http.StatusOK)
	}
	var csrfToken string
	for _, cookie := range tokenRes.Result().Cookies() {
		if cookie.Name == "xmdm_csrf" {
			csrfToken = cookie.Value
			break
		}
	}
	if csrfToken == "" {
		t.Fatalf("missing csrf token cookie")
	}

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=admin&password=secret"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.20:1234"
		req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("request %d status = %d, want %d", i+1, rr.Code, http.StatusSeeOther)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=admin&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.20:5678"
	req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
}

func TestNewMuxRateLimitsDashboardCommandCreate(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	handler := NewMux(svc, Dependencies{
		Commands:      stubCommandRepository{},
		PluginManager: plugins.Disabled(),
	})

	tokenReq := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	tokenReq.RemoteAddr = "198.51.100.21:1234"
	tokenRes := httptest.NewRecorder()
	handler.ServeHTTP(tokenRes, tokenReq)
	if tokenRes.Code != http.StatusOK {
		t.Fatalf("token request status = %d, want %d", tokenRes.Code, http.StatusOK)
	}
	var csrfToken string
	for _, cookie := range tokenRes.Result().Cookies() {
		if cookie.Name == "xmdm_csrf" {
			csrfToken = cookie.Value
			break
		}
	}
	if csrfToken == "" {
		t.Fatalf("missing csrf token cookie")
	}
	sessionID := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}).ID

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("csrfToken="+csrfToken+"&type=ping&targetType=device&targetDeviceId=device-1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.21:1234"
		req.AddCookie(&http.Cookie{Name: "xmdm_session", Value: sessionID})
		req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("request %d status = %d, want %d", i+1, rr.Code, http.StatusSeeOther)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/commands/create", strings.NewReader("csrfToken="+csrfToken+"&type=ping&targetType=device&targetDeviceId=device-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.21:5678"
	req.AddCookie(&http.Cookie{Name: "xmdm_session", Value: sessionID})
	req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
}

type stubCommandRepository struct{}

func (stubCommandRepository) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return []commands.Command{{ID: "cmd-1", Type: "ping", Status: commands.StatusQueued}}, nil
}

func (stubCommandRepository) ListRecent(context.Context, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (stubCommandRepository) ListRecentAll(context.Context, string) ([]commands.Command, error) {
	return nil, nil
}

func (stubCommandRepository) ListPendingForDevice(context.Context, string, string) ([]commands.Command, error) {
	return nil, nil
}

func (stubCommandRepository) GetOverviewStats(context.Context, string) (commands.OverviewStats, error) {
	return commands.OverviewStats{}, nil
}

func (stubCommandRepository) Get(context.Context, string, string) (commands.Command, error) {
	return commands.Command{}, nil
}

func (stubCommandRepository) ListPending(context.Context, string, string, pagination.Params) ([]commands.Command, error) {
	return nil, nil
}

func (stubCommandRepository) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}
