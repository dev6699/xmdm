package adminhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/roles"
	"xmdm/server/internal/users"
)

func TestRolesPageRender(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingRoleStore{
		roles: []roles.Role{
			{
				RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
				Name:        "operators",
				Permissions: []string{"admin:devices"},
			},
		},
		role: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
			Name:        "operators",
			Permissions: []string{"admin:devices"},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Roles:    store,
		Audit:    &recordingAuditStore{},
		TenantID: "tenant-1",
	})

	t.Run("roles page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/roles", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected roles page status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Roles", "Define the permission bundles available to operators.", "Create role", "operators", "admin:devices"} {
			if !strings.Contains(body, want) {
				t.Fatalf("roles page missing %q: %s", want, body)
			}
		}
	})

	t.Run("role detail page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/roles/role-1", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected role detail status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Role Detail", "Edit the permission bundle or retire it from active use.", "Update role", "Retire role", "operators"} {
			if !strings.Contains(body, want) {
				t.Fatalf("role detail page missing %q: %s", want, body)
			}
		}
	})
}

func TestRolesMutationsRecordAudit(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingRoleStore{
		roles: []roles.Role{
			{
				RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
				Name:        "operators",
				Permissions: []string{"admin:devices"},
			},
		},
		role: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
			Name:        "operators",
			Permissions: []string{"admin:devices"},
		},
		createdRole: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
			Name:        "operators",
			Permissions: []string{"admin:devices"},
		},
		updatedRole: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
			Name:        "operators-updated",
			Permissions: []string{"admin:devices"},
		},
		retiredRole: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "retired"},
			Name:        "operators",
			Permissions: []string{"admin:devices"},
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Roles:    store,
		Audit:    auditStore,
		TenantID: "tenant-1",
	})

	postForm := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create role", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/roles/create", "name=operators&permissions=admin%3Adevices&csrfToken=token")
		assertRedirect(t, rr, "/admin/roles?ok=role+created")
		assertAuditRecord(t, auditStore, "create", "roles", "role-1")
	})

	t.Run("update role", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/roles/role-1/update", "name=operators-updated&permissions=admin%3Adevices&csrfToken=token")
		assertRedirect(t, rr, "/admin/roles/role-1?ok=role+updated")
		assertAuditRecord(t, auditStore, "update", "roles", "role-1")
	})

	t.Run("retire role", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/roles/role-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/roles?ok=role+retired")
		assertAuditRecord(t, auditStore, "retire", "roles", "role-1")
	})
}

type recordingRoleStore struct {
	roles []roles.Role
	role  roles.Role

	createdRole roles.Role
	updatedRole roles.Role
	retiredRole roles.Role
}

func (s *recordingRoleStore) ListUsers(context.Context, string, pagination.Params) ([]users.User, error) {
	return nil, nil
}

func (s *recordingRoleStore) ListActiveUsers(context.Context, string) ([]users.User, error) {
	return nil, nil
}

func (s *recordingRoleStore) GetUser(context.Context, string, string) (users.User, error) {
	return users.User{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) CreateUser(context.Context, string, users.UserUpsert) (users.User, error) {
	return users.User{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) UpdateUser(context.Context, string, string, users.UserUpsert) (users.User, error) {
	return users.User{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) RetireUser(context.Context, string, string) (users.User, error) {
	return users.User{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) ListRoles(context.Context, string, pagination.Params) ([]roles.Role, error) {
	return append([]roles.Role(nil), s.roles...), nil
}

func (s *recordingRoleStore) ListActiveRoles(context.Context, string) ([]roles.Role, error) {
	items := make([]roles.Role, 0, len(s.roles))
	for _, item := range s.roles {
		if item.Status == "active" {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *recordingRoleStore) AuthenticateUser(context.Context, string, string, string) (users.User, roles.Role, error) {
	return users.User{}, roles.Role{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) GetRole(_ context.Context, _ string, id string) (roles.Role, error) {
	if strings.TrimSpace(id) == strings.TrimSpace(s.role.ID) {
		return s.role, nil
	}
	return roles.Role{}, httpx.ErrNotFound
}

func (s *recordingRoleStore) CreateRole(context.Context, string, roles.RoleUpsert) (roles.Role, error) {
	if s.createdRole.ID != "" {
		return s.createdRole, nil
	}
	return s.role, nil
}

func (s *recordingRoleStore) UpdateRole(context.Context, string, string, roles.RoleUpsert) (roles.Role, error) {
	if s.updatedRole.ID != "" {
		return s.updatedRole, nil
	}
	return s.role, nil
}

func (s *recordingRoleStore) RetireRole(context.Context, string, string) (roles.Role, error) {
	if s.retiredRole.ID != "" {
		return s.retiredRole, nil
	}
	return s.role, nil
}
