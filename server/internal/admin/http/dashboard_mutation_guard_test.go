package adminhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
)

func TestDashboardMutationsRequireLogin(t *testing.T) {
	for _, path := range allDashboardMutationPaths() {
		t.Run(path, func(t *testing.T) {
			mux := http.NewServeMux()
			RegisterDashboard(mux, auth.NewService("admin", "secret", time.Hour), DashboardDependencies{})

			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("csrfToken=token"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-XMDM-CSRF-Token", "token")
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusSeeOther {
				t.Fatalf("expected redirect to login, got %d body=%s", rr.Code, rr.Body.String())
			}
			if loc := rr.Header().Get("Location"); loc != "/admin/login" {
				t.Fatalf("unexpected login redirect location: %q", loc)
			}
		})
	}
}

func TestDashboardMutationsRequireWritePermission(t *testing.T) {
	for _, path := range allDashboardMutationPaths() {
		t.Run(path, func(t *testing.T) {
			svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminRead})
			session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminRead})
			mux := http.NewServeMux()
			RegisterDashboard(mux, svc, DashboardDependencies{})

			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("csrfToken=token"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected forbidden render, got %d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "forbidden") {
				t.Fatalf("expected forbidden response body, got %s", rr.Body.String())
			}
		})
	}
}

func TestDashboardMutationsRequireCSRF(t *testing.T) {
	for _, path := range allDashboardMutationPaths() {
		t.Run(path, func(t *testing.T) {
			svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
			session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
			mux := http.NewServeMux()
			RegisterDashboard(mux, svc, DashboardDependencies{})

			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected forbidden render, got %d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "forbidden") {
				t.Fatalf("expected forbidden response body, got %s", rr.Body.String())
			}
		})
	}
}

func allDashboardMutationPaths() []string {
	return []string{
		"/admin/users/create",
		"/admin/users/u-1/update",
		"/admin/users/u-1/retire",
		"/admin/roles/create",
		"/admin/roles/r-1/update",
		"/admin/roles/r-1/retire",
		"/admin/groups/create",
		"/admin/groups/g-1/update",
		"/admin/groups/g-1/retire",
		"/admin/policies/create",
		"/admin/policies/p-1/update",
		"/admin/policies/p-1/retire",
		"/admin/policies/p-1/apps/app-1/toggle",
		"/admin/policies/p-1/certificates/cert-1/toggle",
		"/admin/policies/p-1/managed-files/file-1/toggle",
		"/admin/devices/create",
		"/admin/devices/d-1/update",
		"/admin/devices/d-1/retire",
		"/admin/apps/create",
		"/admin/apps/a-1/update",
		"/admin/apps/a-1/retire",
		"/admin/apps/a-1/versions/create",
		"/admin/managed-files/create",
		"/admin/managed-files/mf-1/retire",
		"/admin/certificates/create",
		"/admin/certificates/c-1/retire",
		"/admin/commands/create",
	}
}
