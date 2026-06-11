package adminhttp

import (
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
