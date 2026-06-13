package host

import (
	"context"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/pagination"
	internalplugins "xmdm/server/internal/plugins"
	"xmdm/server/internal/roles"
)

func TestSyncSeedRolePermissionsAddsPluginPermissions(t *testing.T) {
	repo := &fakeRoleRepo{
		roles: []roles.Role{{
			RecordBase:  roles.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"},
			Name:        bootstrap.SeedAdminRoleName,
			Permissions: []string{"admin.read", "admin.write", "devices.read", "devices.write"},
		}},
	}

	err := syncSeedRolePermissions(context.Background(), repo, bootstrap.SeedTenantID, []auth.Permission{
		auth.PermissionAdminRead,
		auth.PermissionAdminWrite,
		auth.PermissionDevicesRead,
		auth.PermissionDevicesWrite,
		auth.Permission("admin:remote-control"),
	})
	if err != nil {
		t.Fatalf("syncSeedRolePermissions: %v", err)
	}
	if len(repo.updatedRoles) != 1 {
		t.Fatalf("expected one role update, got %#v", repo.updatedRoles)
	}
	if got := repo.updatedRoles[0].Permissions; len(got) != 5 || got[4] != "admin:remote-control" {
		t.Fatalf("unexpected merged permissions: %#v", got)
	}
}

type fakeRoleRepo struct {
	roles        []roles.Role
	updatedRoles []roles.RoleUpsert
}

func (r *fakeRoleRepo) ListRoles(context.Context, string, pagination.Params) ([]roles.Role, error) {
	return append([]roles.Role(nil), r.roles...), nil
}
func (r *fakeRoleRepo) ListActiveRoles(context.Context, string) ([]roles.Role, error) {
	return append([]roles.Role(nil), r.roles...), nil
}
func (r *fakeRoleRepo) GetRole(context.Context, string, string) (roles.Role, error) {
	return roles.Role{}, nil
}
func (r *fakeRoleRepo) CreateRole(context.Context, string, roles.RoleUpsert) (roles.Role, error) {
	return roles.Role{}, nil
}
func (r *fakeRoleRepo) UpdateRole(_ context.Context, _ string, _ string, req roles.RoleUpsert) (roles.Role, error) {
	r.updatedRoles = append(r.updatedRoles, req)
	return roles.Role{RecordBase: roles.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"}, Name: req.Name, Permissions: req.Permissions}, nil
}
func (r *fakeRoleRepo) RetireRole(context.Context, string, string) (roles.Role, error) {
	return roles.Role{}, nil
}

var _ roles.Repository = (*fakeRoleRepo)(nil)

func TestSyncSeedRolePermissionsNoopsWhenAlreadyPresent(t *testing.T) {
	repo := &fakeRoleRepo{
		roles: []roles.Role{{
			RecordBase:  roles.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"},
			Name:        bootstrap.SeedAdminRoleName,
			Permissions: []string{"admin.read", "admin.write", "devices.read", "devices.write", "admin:remote-control"},
		}},
	}

	err := syncSeedRolePermissions(context.Background(), repo, bootstrap.SeedTenantID, []auth.Permission{
		auth.PermissionAdminRead,
		auth.PermissionAdminWrite,
		auth.PermissionDevicesRead,
		auth.PermissionDevicesWrite,
		auth.Permission("admin:remote-control"),
	})
	if err != nil {
		t.Fatalf("syncSeedRolePermissions: %v", err)
	}
	if len(repo.updatedRoles) != 0 {
		t.Fatalf("expected no updates, got %#v", repo.updatedRoles)
	}
}

func TestMergePermissionsAppendsUniquePermissions(t *testing.T) {
	merged := mergePermissions([]string{"admin.read", "devices.read"}, []auth.Permission{
		auth.PermissionAdminRead,
		auth.Permission("admin:remote-control"),
		auth.PermissionDevicesWrite,
	})
	want := []string{"admin.read", "devices.read", "admin:remote-control", "devices.write"}
	if len(merged) != len(want) {
		t.Fatalf("unexpected merged length: got %d want %d", len(merged), len(want))
	}
	for i := range want {
		if merged[i] != want[i] {
			t.Fatalf("unexpected merged permissions at %d: got %q want %q", i, merged[i], want[i])
		}
	}
}

func TestMergedAuthPermissionsIncludesPluginPermissions(t *testing.T) {
	mgr := internalplugins.New(internalplugins.Plugin{
		ID:          "remote-control",
		Name:        "Remote Control",
		Enabled:     true,
		Permissions: []string{"admin:remote-control"},
	})
	perms := mergedAuthPermissions(mgr)
	if !containsAuthPermission(perms, auth.Permission("admin:remote-control")) {
		t.Fatalf("expected plugin permission in auth catalog, got %#v", perms)
	}
}

func TestSyncSeedRolePermissionsIgnoresMissingSeedRole(t *testing.T) {
	repo := &fakeRoleRepo{}
	if err := syncSeedRolePermissions(context.Background(), repo, bootstrap.SeedTenantID, []auth.Permission{auth.Permission("admin:remote-control")}); err != nil {
		t.Fatalf("syncSeedRolePermissions: %v", err)
	}
	if len(repo.updatedRoles) != 0 {
		t.Fatalf("expected no updates, got %#v", repo.updatedRoles)
	}
}

func TestSyncSeedRolePermissionsIgnoresRetiredSeedRole(t *testing.T) {
	repo := &fakeRoleRepo{
		roles: []roles.Role{{
			RecordBase:  roles.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "retired", DeletedAt: func() *time.Time { t := time.Now(); return &t }()},
			Name:        bootstrap.SeedAdminRoleName,
			Permissions: []string{"admin.read"},
		}},
	}
	if err := syncSeedRolePermissions(context.Background(), repo, bootstrap.SeedTenantID, []auth.Permission{auth.Permission("admin:remote-control")}); err != nil {
		t.Fatalf("syncSeedRolePermissions: %v", err)
	}
	if len(repo.updatedRoles) != 0 {
		t.Fatalf("expected no updates for retired role, got %#v", repo.updatedRoles)
	}
}

func containsAuthPermission(perms []auth.Permission, target auth.Permission) bool {
	for _, perm := range perms {
		if perm == target {
			return true
		}
	}
	return false
}
