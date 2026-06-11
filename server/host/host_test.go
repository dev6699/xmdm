package host

import (
	"context"
	"testing"
	"time"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/identity"
	"xmdm/server/internal/pagination"
	internalplugins "xmdm/server/internal/plugins"
)

func TestSyncSeedRolePermissionsAddsPluginPermissions(t *testing.T) {
	repo := &fakeIdentityRepo{
		roles: []identity.Role{{
			RecordBase:  identity.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"},
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

type fakeIdentityRepo struct {
	roles        []identity.Role
	updatedRoles []identity.RoleUpsert
}

func (r *fakeIdentityRepo) ListUsers(context.Context, string, pagination.Params) ([]identity.User, error) {
	return nil, nil
}
func (r *fakeIdentityRepo) ListActiveUsers(context.Context, string) ([]identity.User, error) {
	return nil, nil
}
func (r *fakeIdentityRepo) GetUser(context.Context, string, string) (identity.User, error) {
	return identity.User{}, nil
}
func (r *fakeIdentityRepo) CreateUser(context.Context, string, identity.UserUpsert) (identity.User, error) {
	return identity.User{}, nil
}
func (r *fakeIdentityRepo) UpdateUser(context.Context, string, string, identity.UserUpsert) (identity.User, error) {
	return identity.User{}, nil
}
func (r *fakeIdentityRepo) RetireUser(context.Context, string, string) (identity.User, error) {
	return identity.User{}, nil
}
func (r *fakeIdentityRepo) AuthenticateUser(context.Context, string, string, string) (identity.User, identity.Role, error) {
	return identity.User{}, identity.Role{}, nil
}
func (r *fakeIdentityRepo) ListRoles(context.Context, string, pagination.Params) ([]identity.Role, error) {
	return append([]identity.Role(nil), r.roles...), nil
}
func (r *fakeIdentityRepo) ListActiveRoles(context.Context, string) ([]identity.Role, error) {
	return append([]identity.Role(nil), r.roles...), nil
}
func (r *fakeIdentityRepo) GetRole(context.Context, string, string) (identity.Role, error) {
	return identity.Role{}, nil
}
func (r *fakeIdentityRepo) CreateRole(context.Context, string, identity.RoleUpsert) (identity.Role, error) {
	return identity.Role{}, nil
}
func (r *fakeIdentityRepo) UpdateRole(_ context.Context, _ string, _ string, req identity.RoleUpsert) (identity.Role, error) {
	r.updatedRoles = append(r.updatedRoles, req)
	return identity.Role{RecordBase: identity.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"}, Name: req.Name, Permissions: req.Permissions}, nil
}
func (r *fakeIdentityRepo) RetireRole(context.Context, string, string) (identity.Role, error) {
	return identity.Role{}, nil
}

var _ identity.Repository = (*fakeIdentityRepo)(nil)

func TestSyncSeedRolePermissionsNoopsWhenAlreadyPresent(t *testing.T) {
	repo := &fakeIdentityRepo{
		roles: []identity.Role{{
			RecordBase:  identity.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "active"},
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
	repo := &fakeIdentityRepo{}
	if err := syncSeedRolePermissions(context.Background(), repo, bootstrap.SeedTenantID, []auth.Permission{auth.Permission("admin:remote-control")}); err != nil {
		t.Fatalf("syncSeedRolePermissions: %v", err)
	}
	if len(repo.updatedRoles) != 0 {
		t.Fatalf("expected no updates, got %#v", repo.updatedRoles)
	}
}

func TestSyncSeedRolePermissionsIgnoresRetiredSeedRole(t *testing.T) {
	repo := &fakeIdentityRepo{
		roles: []identity.Role{{
			RecordBase:  identity.RecordBase{ID: bootstrap.SeedAdminRoleID, TenantID: bootstrap.SeedTenantID, Status: "retired", DeletedAt: func() *time.Time { t := time.Now(); return &t }()},
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
