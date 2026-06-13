package adminhttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
)

func TestLogoutRedirectsForHTMLRequests(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite})
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{})

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader("csrfToken=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("X-XMDM-CSRF-Token", "token")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if got := rr.Header().Get("Location"); got != "/admin/login" {
		t.Fatalf("logout redirect location = %q, want %q", got, "/admin/login")
	}
}

func TestLogoutReturnsNoContentForAPICalls(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite})
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{})

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader("csrfToken=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-XMDM-CSRF-Token", "token")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestLoginFailedRedirectsWithFlash(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{})

	req := httptest.NewRequest(http.MethodPost, "/admin/login?next=/admin/devices", strings.NewReader("username=admin&password=wrong&csrfToken=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if got := rr.Header().Get("Location"); got != "/admin/login?next=%2Fadmin%2Fdevices" {
		t.Fatalf("login redirect location = %q, want %q", got, "/admin/login?next=%2Fadmin%2Fdevices")
	}
	flashCookie := cookieValue(rr.Header()["Set-Cookie"], loginFlashCookieName)
	if flashCookie == "" {
		t.Fatalf("login flash cookie not set")
	}
}

func TestLoginFlashClearsOnReload(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{})

	flash, err := json.Marshal(loginFlashData{Username: "admin", Error: "invalid credentials"})
	if err != nil {
		t.Fatalf("marshal flash: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	req.AddCookie(&http.Cookie{Name: loginFlashCookieName, Value: base64.RawURLEncoding.EncodeToString(flash)})
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "invalid credentials") {
		t.Fatalf("login page body missing error, got %q", body)
	}
	if !strings.Contains(body, `value="admin"`) {
		t.Fatalf("login page body missing username, got %q", body)
	}
	if got := strings.Join(rr.Header()["Set-Cookie"], "\n"); !strings.Contains(got, loginFlashCookieName+"=") || !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("login flash cookie was not cleared, set-cookie = %q", got)
	}
}

func cookieValue(headers []string, name string) string {
	for _, header := range headers {
		if strings.HasPrefix(header, name+"=") {
			return header
		}
	}
	return ""
}
