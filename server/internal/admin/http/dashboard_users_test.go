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

func TestUsersPageRender(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingUserStore{
		users: []users.User{
			{
				RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
				Email:      "admin@example.com",
				RoleID:     "role-1",
			},
		},
		user: users.User{
			RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
			Email:      "admin@example.com",
			RoleID:     "role-1",
		},
		role: roles.Role{
			RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
			Name:        "operators",
			Permissions: []string{"admin:devices"},
		},
		roles: []roles.Role{
			{
				RecordBase:  roles.RecordBase{ID: "role-1", TenantID: "tenant-1", Status: "active"},
				Name:        "operators",
				Permissions: []string{"admin:devices"},
			},
		},
	}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Users:    store,
		Roles:    store,
		Audit:    &recordingAuditStore{},
		TenantID: "tenant-1",
	})

	t.Run("users page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected users page status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"Users", "Manage operator accounts and role bindings.", "Create user", "admin@example.com", "operators"} {
			if !strings.Contains(body, want) {
				t.Fatalf("users page missing %q: %s", want, body)
			}
		}
	})

	t.Run("user detail page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/users/user-1", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected user detail status: %d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		for _, want := range []string{"User Detail", "Edit the operator account or retire it from the active roster.", "Update user", "Retire user", "admin@example.com"} {
			if !strings.Contains(body, want) {
				t.Fatalf("user detail page missing %q: %s", want, body)
			}
		}
	})
}

func TestUsersMutationsRecordAudit(t *testing.T) {
	perms := []auth.Permission{auth.PermissionAdminRead, auth.PermissionAdminWrite}
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, perms)
	session := svc.IssueSession("admin", perms)
	store := &recordingUserStore{
		users: []users.User{
			{
				RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
				Email:      "admin@example.com",
				RoleID:     "role-1",
			},
		},
		user: users.User{
			RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
			Email:      "admin@example.com",
			RoleID:     "role-1",
		},
		createdUser: users.User{
			RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
			Email:      "new@example.com",
			RoleID:     "role-1",
		},
		updatedUser: users.User{
			RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "active"},
			Email:      "admin@example.com",
			RoleID:     "role-1",
		},
		retiredUser: users.User{
			RecordBase: users.RecordBase{ID: "user-1", TenantID: "tenant-1", Status: "retired"},
			Email:      "admin@example.com",
			RoleID:     "role-1",
		},
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
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Users:    store,
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

	t.Run("create user", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/users/create", "email=new%40example.com&password=secret123&roleId=role-1&csrfToken=token")
		assertRedirect(t, rr, "/admin/users?ok=user+created")
		assertAuditRecord(t, auditStore, "create", "users", "user-1")
	})

	t.Run("update user", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/users/user-1/update", "email=admin%40example.com&password=changed123&roleId=role-1&csrfToken=token")
		assertRedirect(t, rr, "/admin/users/user-1?ok=user+updated")
		assertAuditRecord(t, auditStore, "update", "users", "user-1")
	})

	t.Run("retire user", func(t *testing.T) {
		auditStore.records = nil
		rr := postForm("/admin/users/user-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/users?ok=user+retired")
		assertAuditRecord(t, auditStore, "retire", "users", "user-1")
	})
}

type recordingUserStore struct {
	users []users.User
	user  users.User

	createdUser users.User
	updatedUser users.User
	retiredUser users.User

	roles []roles.Role
	role  roles.Role
}

func (s *recordingUserStore) ListUsers(context.Context, string, pagination.Params) ([]users.User, error) {
	return append([]users.User(nil), s.users...), nil
}

func (s *recordingUserStore) ListActiveUsers(context.Context, string) ([]users.User, error) {
	items := make([]users.User, 0, len(s.users))
	for _, item := range s.users {
		if item.Status == "active" {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *recordingUserStore) GetUser(_ context.Context, _ string, id string) (users.User, error) {
	if strings.TrimSpace(id) == strings.TrimSpace(s.user.ID) {
		return s.user, nil
	}
	return users.User{}, httpx.ErrNotFound
}

func (s *recordingUserStore) CreateUser(context.Context, string, users.UserUpsert) (users.User, error) {
	if s.createdUser.ID != "" {
		return s.createdUser, nil
	}
	return s.user, nil
}

func (s *recordingUserStore) UpdateUser(context.Context, string, string, users.UserUpsert) (users.User, error) {
	if s.updatedUser.ID != "" {
		return s.updatedUser, nil
	}
	return s.user, nil
}

func (s *recordingUserStore) RetireUser(context.Context, string, string) (users.User, error) {
	if s.retiredUser.ID != "" {
		return s.retiredUser, nil
	}
	return s.user, nil
}

func (s *recordingUserStore) AuthenticateUser(context.Context, string, string, string) (users.User, roles.Role, error) {
	return s.user, s.role, nil
}

func (s *recordingUserStore) ListRoles(context.Context, string, pagination.Params) ([]roles.Role, error) {
	return append([]roles.Role(nil), s.roles...), nil
}

func (s *recordingUserStore) ListActiveRoles(context.Context, string) ([]roles.Role, error) {
	items := make([]roles.Role, 0, len(s.roles))
	for _, item := range s.roles {
		if item.Status == "active" {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *recordingUserStore) GetRole(_ context.Context, _ string, id string) (roles.Role, error) {
	if strings.TrimSpace(id) == strings.TrimSpace(s.role.ID) {
		return s.role, nil
	}
	return roles.Role{}, httpx.ErrNotFound
}

func (s *recordingUserStore) CreateRole(context.Context, string, roles.RoleUpsert) (roles.Role, error) {
	if s.role.ID != "" {
		return s.role, nil
	}
	return s.role, nil
}

func (s *recordingUserStore) UpdateRole(context.Context, string, string, roles.RoleUpsert) (roles.Role, error) {
	if s.role.ID != "" {
		return s.role, nil
	}
	return s.role, nil
}

func (s *recordingUserStore) RetireRole(context.Context, string, string) (roles.Role, error) {
	if s.role.ID != "" {
		return s.role, nil
	}
	return roles.Role{}, httpx.ErrNotFound
}
