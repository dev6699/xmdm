package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/plugins"
)

func TestNewMuxRateLimitsAdminLogin(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	handler := NewMux(svc, Dependencies{
		PluginManager: plugins.Disabled(),
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("username=admin&password=secret"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.10:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("request %d status = %d, want %d", i+1, rr.Code, http.StatusSeeOther)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("username=admin&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.10:5678"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
}
