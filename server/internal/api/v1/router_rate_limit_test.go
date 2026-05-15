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

	tokenReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/login", nil)
	tokenReq.RemoteAddr = "198.51.100.10:1234"
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
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=admin&password=secret"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "198.51.100.10:1234"
		req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("request %d status = %d, want %d", i+1, rr.Code, http.StatusSeeOther)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("csrfToken="+csrfToken+"&username=admin&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.10:5678"
	req.AddCookie(&http.Cookie{Name: "xmdm_csrf", Value: csrfToken})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited request status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
}

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
