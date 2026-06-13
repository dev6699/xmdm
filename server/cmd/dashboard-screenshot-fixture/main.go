package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	adminhttp "xmdm/server/internal/admin/http"
	"xmdm/server/internal/apps"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/commands"
	"xmdm/server/internal/device"
	"xmdm/server/internal/deviceinfo"
	"xmdm/server/internal/enrollment"
	"xmdm/server/internal/files"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/logs"
	managedfiles "xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/policy"
	"xmdm/server/internal/roles"
	"xmdm/server/internal/users"
)

const tenantID = "00000000-0000-0000-0000-000000000000"

func main() {
	fixtureLocation := time.FixedZone("MYT", 8*60*60)
	now := time.Now().In(fixtureLocation).Truncate(time.Minute)
	fileArtifact := files.Artifact{RecordBase: files.RecordBase{ID: "artifact-apk", TenantID: tenantID, Status: "active", UpdatedAt: now}, StorageKey: "artifacts/launcher.apk", Checksum: "sha256-apk", SizeBytes: 2048, MimeType: "application/vnd.android.package-archive"}
	certArtifact := files.Artifact{RecordBase: files.RecordBase{ID: "artifact-cert", TenantID: tenantID, Status: "active", UpdatedAt: now}, StorageKey: "certs/root.pem", Checksum: "sha256-cert", SizeBytes: 512, MimeType: "application/x-pem-file"}
	policyID := "policy-baseline"

	svc := auth.NewService("admin", "admin", time.Hour)
	mux := http.NewServeMux()
	adminhttp.RegisterDashboard(mux, svc, adminhttp.DashboardDependencies{
		Users:           &identityStore{now: now},
		Roles:           &identityStore{now: now},
		Groups:          &groupStore{now: now},
		Policies:        &policyStore{now: now},
		Devices:         &deviceStore{now: now, policyID: policyID},
		Apps:            &appStore{now: now, artifact: fileArtifact},
		Files:           &fileStore{now: now, artifact: fileArtifact},
		ManagedFiles:    &managedFileStore{now: now, artifact: fileArtifact},
		Certificates:    &certificateStore{now: now, artifact: certArtifact},
		Commands:        &commandStore{now: now},
		Logs:            &logStore{now: now},
		DeviceInfo:      &deviceInfoStore{now: now},
		Audit:           &auditStore{now: now},
		Enrollment:      &enrollmentStore{now: now},
		Runtime:         enrollment.RuntimeSnapshot{},
		Artifacts:       artifactStore{},
		ServerPublicURL: "http://127.0.0.1:39091",
		AgentAppPackage: "com.xmdm.agent",
		TenantID:        tenantID,
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:39091", mux))
}

type identityStore struct{ now time.Time }

func (s *identityStore) ListUsers(context.Context, string, pagination.Params) ([]users.User, error) {
	items := []users.User{
		{RecordBase: users.RecordBase{ID: "user-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -14), UpdatedAt: s.now}, Email: "admin@example.com", RoleID: "role-admin"},
		{RecordBase: users.RecordBase{ID: "user-ops", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -10), UpdatedAt: s.now.Add(-2 * time.Hour)}, Email: "ops@example.com", RoleID: "role-operator"},
		{RecordBase: users.RecordBase{ID: "user-auditor", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -7), UpdatedAt: s.now.Add(-6 * time.Hour)}, Email: "auditor@example.com", RoleID: "role-read"},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *identityStore) ListActiveUsers(context.Context, string) ([]users.User, error) {
	items := []users.User{
		{RecordBase: users.RecordBase{ID: "user-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -14), UpdatedAt: s.now}, Email: "admin@example.com", RoleID: "role-admin"},
		{RecordBase: users.RecordBase{ID: "user-ops", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -10), UpdatedAt: s.now.Add(-2 * time.Hour)}, Email: "ops@example.com", RoleID: "role-operator"},
		{RecordBase: users.RecordBase{ID: "user-auditor", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -7), UpdatedAt: s.now.Add(-6 * time.Hour)}, Email: "auditor@example.com", RoleID: "role-read"},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *identityStore) GetUser(_ context.Context, _ string, id string) (users.User, error) {
	for _, item := range []users.User{
		{RecordBase: users.RecordBase{ID: "user-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -14), UpdatedAt: s.now}, Email: "admin@example.com", RoleID: "role-admin"},
		{RecordBase: users.RecordBase{ID: "user-ops", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -10), UpdatedAt: s.now.Add(-2 * time.Hour)}, Email: "ops@example.com", RoleID: "role-operator"},
		{RecordBase: users.RecordBase{ID: "user-auditor", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -7), UpdatedAt: s.now.Add(-6 * time.Hour)}, Email: "auditor@example.com", RoleID: "role-read"},
	} {
		if item.ID == id {
			return item, nil
		}
	}
	return users.User{}, httpx.ErrNotFound
}
func (s *identityStore) CreateUser(context.Context, string, users.UserUpsert) (users.User, error) {
	return users.User{}, nil
}
func (s *identityStore) UpdateUser(context.Context, string, string, users.UserUpsert) (users.User, error) {
	return users.User{}, nil
}
func (s *identityStore) RetireUser(context.Context, string, string) (users.User, error) {
	return users.User{}, nil
}
func (s *identityStore) AuthenticateUser(context.Context, string, string, string) (users.User, roles.Role, error) {
	return users.User{}, roles.Role{}, nil
}
func (s *identityStore) ListRoles(context.Context, string, pagination.Params) ([]roles.Role, error) {
	items := []roles.Role{
		{RecordBase: roles.RecordBase{ID: "role-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -20), UpdatedAt: s.now}, Name: "Administrators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-operator", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -15), UpdatedAt: s.now}, Name: "Operators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-read", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now}, Name: "Read Only", Permissions: []string{"admin.read"}},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *identityStore) ListActiveRoles(context.Context, string) ([]roles.Role, error) {
	items := []roles.Role{
		{RecordBase: roles.RecordBase{ID: "role-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -20), UpdatedAt: s.now}, Name: "Administrators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-operator", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -15), UpdatedAt: s.now}, Name: "Operators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-read", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now}, Name: "Read Only", Permissions: []string{"admin.read"}},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *identityStore) GetRole(_ context.Context, _ string, id string) (roles.Role, error) {
	for _, item := range []roles.Role{
		{RecordBase: roles.RecordBase{ID: "role-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -20), UpdatedAt: s.now}, Name: "Administrators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-operator", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -15), UpdatedAt: s.now}, Name: "Operators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: roles.RecordBase{ID: "role-read", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now}, Name: "Read Only", Permissions: []string{"admin.read"}},
	} {
		if item.ID == id {
			return item, nil
		}
	}
	return roles.Role{}, httpx.ErrNotFound
}
func (s *identityStore) CreateRole(context.Context, string, roles.RoleUpsert) (roles.Role, error) {
	return roles.Role{}, nil
}
func (s *identityStore) UpdateRole(context.Context, string, string, roles.RoleUpsert) (roles.Role, error) {
	return roles.Role{}, nil
}
func (s *identityStore) RetireRole(context.Context, string, string) (roles.Role, error) {
	return roles.Role{}, nil
}

type groupStore struct{ now time.Time }

func (s *groupStore) ListGroups(context.Context, string, pagination.Params) ([]group.Group, error) {
	items := []group.Group{
		{RecordBase: group.RecordBase{ID: "group-field", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -18), UpdatedAt: s.now}, Name: "Field Devices"},
		{RecordBase: group.RecordBase{ID: "group-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -17), UpdatedAt: s.now}, Name: "Kiosk Fleet"},
		{RecordBase: group.RecordBase{ID: "group-warehouse", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now.Add(-2 * time.Hour)}, Name: "Warehouse"},
		{RecordBase: group.RecordBase{ID: "group-rugged", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -9), UpdatedAt: s.now.Add(-5 * time.Hour)}, Name: "Rugged Handhelds"},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *groupStore) ListActiveGroups(context.Context, string) ([]group.Group, error) {
	items := []group.Group{
		{RecordBase: group.RecordBase{ID: "group-field", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -18), UpdatedAt: s.now}, Name: "Field Devices"},
		{RecordBase: group.RecordBase{ID: "group-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -17), UpdatedAt: s.now}, Name: "Kiosk Fleet"},
		{RecordBase: group.RecordBase{ID: "group-warehouse", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now.Add(-2 * time.Hour)}, Name: "Warehouse"},
		{RecordBase: group.RecordBase{ID: "group-rugged", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -9), UpdatedAt: s.now.Add(-5 * time.Hour)}, Name: "Rugged Handhelds"},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *groupStore) GetGroup(_ context.Context, _ string, id string) (group.Group, error) {
	for _, item := range []group.Group{
		{RecordBase: group.RecordBase{ID: "group-field", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -18), UpdatedAt: s.now}, Name: "Field Devices"},
		{RecordBase: group.RecordBase{ID: "group-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -17), UpdatedAt: s.now}, Name: "Kiosk Fleet"},
		{RecordBase: group.RecordBase{ID: "group-warehouse", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -12), UpdatedAt: s.now.Add(-2 * time.Hour)}, Name: "Warehouse"},
		{RecordBase: group.RecordBase{ID: "group-rugged", TenantID: tenantID, Status: "active", CreatedAt: s.now.AddDate(0, 0, -9), UpdatedAt: s.now.Add(-5 * time.Hour)}, Name: "Rugged Handhelds"},
	} {
		if item.ID == id {
			return item, nil
		}
	}
	return group.Group{}, httpx.ErrNotFound
}
func (s *groupStore) ListGroupDevices(_ context.Context, _ string, groupID string, _ pagination.Params) ([]device.Device, error) {
	rows := make([]device.Device, 0)
	for _, item := range fixtureDevices(s.now) {
		for _, id := range item.GroupIDs {
			if id == groupID {
				rows = append(rows, item)
				break
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows, nil
}
func (s *groupStore) CreateGroup(context.Context, string, group.GroupUpsert) (group.Group, error) {
	return group.Group{}, nil
}
func (s *groupStore) UpdateGroup(context.Context, string, string, group.GroupUpsert) (group.Group, error) {
	return group.Group{}, nil
}
func (s *groupStore) RetireGroup(context.Context, string, string) (group.Group, error) {
	return group.Group{}, nil
}

type policyStore struct{ now time.Time }

func fixturePolicies(now time.Time) []policy.Policy {
	items := []policy.Policy{
		{RecordBase: policy.RecordBase{ID: "policy-baseline", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -20), UpdatedAt: now.Add(-30 * time.Minute)}, Name: "Baseline", Version: 5, KioskMode: false, Restrictions: []byte(`{"allowPackages":["com.android.chrome","com.example.viewer","com.xmdm.agent"],"stayAwakeWhilePluggedIn":true}`)},
		{RecordBase: policy.RecordBase{ID: "policy-kiosk", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -18), UpdatedAt: now.Add(-90 * time.Minute)}, Name: "Kiosk", Version: 8, KioskMode: true, KioskAppPackage: "com.xmdm.kiosk", Restrictions: []byte(`{"kioskExitPasscode":"1234","kioskKeepScreenOn":true,"blockPackages":["com.android.settings","com.android.vending"]}`)},
		{RecordBase: policy.RecordBase{ID: "policy-rugged", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -14), UpdatedAt: now.Add(-3 * time.Hour)}, Name: "Rugged Field Ops", Version: 3, KioskMode: false, Restrictions: []byte(`{"allowPackages":["com.zebra.datawedge","com.xmdm.agent","com.example.viewer"],"stayAwakeWhilePluggedIn":false}`)},
		{RecordBase: policy.RecordBase{ID: "policy-guest", TenantID: tenantID, Status: "retired", CreatedAt: now.AddDate(0, 0, -30), UpdatedAt: now.AddDate(0, 0, -2)}, Name: "Guest Demo", Version: 1, KioskMode: false, Restrictions: []byte(`{"allowPackages":["com.android.chrome"]}`)},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (s *policyStore) ListPolicies(context.Context, string, pagination.Params) ([]policy.Policy, error) {
	return fixturePolicies(s.now), nil
}
func (s *policyStore) ListActivePolicies(context.Context, string) ([]policy.Policy, error) {
	items := make([]policy.Policy, 0)
	for _, item := range fixturePolicies(s.now) {
		if item.Status == policy.StatusActive {
			items = append(items, item)
		}
	}
	return items, nil
}
func (s *policyStore) GetOverviewStats(context.Context, string) (policy.OverviewStats, error) {
	items := fixturePolicies(s.now)
	stats := policy.OverviewStats{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case policy.StatusActive:
			stats.Active++
		case policy.StatusRetired:
			stats.Retired++
		}
	}
	return stats, nil
}
func (s *policyStore) GetPolicy(_ context.Context, _ string, id string) (policy.Policy, error) {
	for _, item := range fixturePolicies(s.now) {
		if item.ID == id {
			return item, nil
		}
	}
	return policy.Policy{}, httpx.ErrNotFound
}
func (s *policyStore) CreatePolicy(context.Context, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}
func (s *policyStore) UpdatePolicy(context.Context, string, string, policy.PolicyUpsert) (policy.Policy, error) {
	return policy.Policy{}, nil
}
func (s *policyStore) RetirePolicy(context.Context, string, string) (policy.Policy, error) {
	return policy.Policy{}, nil
}
func (s *policyStore) ListPolicyApps(_ context.Context, _ string, policyID string, _ pagination.Params) ([]policy.PolicyApp, error) {
	rows := []policy.PolicyApp{
		{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-chrome"},
		{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-agent"},
	}
	switch policyID {
	case "policy-kiosk":
		rows = append(rows, policy.PolicyApp{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-kiosk"})
	case "policy-rugged":
		rows = append(rows, policy.PolicyApp{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-datawedge"}, policy.PolicyApp{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-viewer"})
	default:
		rows = append(rows, policy.PolicyApp{RecordBase: policy.RecordBase{Status: policy.StatusActive}, AppID: "app-viewer"})
	}
	return rows, nil
}
func (s *policyStore) ListActivePolicyApps(ctx context.Context, tenantID, policyID string) ([]policy.PolicyApp, error) {
	rows, _ := s.ListPolicyApps(ctx, tenantID, policyID, pagination.Params{})
	out := make([]policy.PolicyApp, 0, len(rows))
	for _, row := range rows {
		if row.Status == policy.StatusActive {
			out = append(out, row)
		}
	}
	return out, nil
}
func (s *policyStore) GetPolicyApp(ctx context.Context, tenantID, policyID, appID string) (policy.PolicyApp, error) {
	rows, _ := s.ListPolicyApps(ctx, tenantID, policyID, pagination.Params{})
	for _, item := range rows {
		if item.AppID == appID {
			return item, nil
		}
	}
	return policy.PolicyApp{}, httpx.ErrNotFound
}
func (s *policyStore) AddPolicyApp(context.Context, string, string, string) (policy.PolicyApp, error) {
	return policy.PolicyApp{}, nil
}
func (s *policyStore) RemovePolicyApp(context.Context, string, string, string) error { return nil }
func (s *policyStore) ListPolicyCertificates(_ context.Context, _ string, policyID string, _ pagination.Params) ([]policy.PolicyCertificate, error) {
	rows := []policy.PolicyCertificate{{RecordBase: policy.RecordBase{Status: policy.StatusActive}, CertificateID: "cert-root"}}
	if policyID == "policy-kiosk" {
		rows = append(rows, policy.PolicyCertificate{RecordBase: policy.RecordBase{Status: policy.StatusActive}, CertificateID: "cert-wifi"})
	}
	return rows, nil
}
func (s *policyStore) ListActivePolicyCertificates(ctx context.Context, tenantID, policyID string) ([]policy.PolicyCertificate, error) {
	rows, _ := s.ListPolicyCertificates(ctx, tenantID, policyID, pagination.Params{})
	out := make([]policy.PolicyCertificate, 0, len(rows))
	for _, row := range rows {
		if row.Status == policy.StatusActive {
			out = append(out, row)
		}
	}
	return out, nil
}
func (s *policyStore) GetPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (policy.PolicyCertificate, error) {
	rows, _ := s.ListPolicyCertificates(ctx, tenantID, policyID, pagination.Params{})
	for _, item := range rows {
		if item.CertificateID == certificateID {
			return item, nil
		}
	}
	return policy.PolicyCertificate{}, httpx.ErrNotFound
}
func (s *policyStore) AddPolicyCertificate(context.Context, string, string, string) (policy.PolicyCertificate, error) {
	return policy.PolicyCertificate{}, nil
}
func (s *policyStore) RemovePolicyCertificate(context.Context, string, string, string) error {
	return nil
}
func (s *policyStore) ListPolicyManagedFiles(_ context.Context, _ string, policyID string, _ pagination.Params) ([]policy.PolicyManagedFile, error) {
	rows := []policy.PolicyManagedFile{{RecordBase: policy.RecordBase{Status: policy.StatusActive}, ManagedFileID: "managed-file-config"}}
	if policyID == "policy-kiosk" {
		rows = append(rows, policy.PolicyManagedFile{RecordBase: policy.RecordBase{Status: policy.StatusActive}, ManagedFileID: "managed-file-kiosk"})
	}
	if policyID == "policy-rugged" {
		rows = append(rows, policy.PolicyManagedFile{RecordBase: policy.RecordBase{Status: policy.StatusActive}, ManagedFileID: "managed-file-field"})
	}
	return rows, nil
}
func (s *policyStore) ListActivePolicyManagedFiles(ctx context.Context, tenantID, policyID string) ([]policy.PolicyManagedFile, error) {
	rows, _ := s.ListPolicyManagedFiles(ctx, tenantID, policyID, pagination.Params{})
	out := make([]policy.PolicyManagedFile, 0, len(rows))
	for _, row := range rows {
		if row.Status == policy.StatusActive {
			out = append(out, row)
		}
	}
	return out, nil
}
func (s *policyStore) GetPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (policy.PolicyManagedFile, error) {
	rows, _ := s.ListPolicyManagedFiles(ctx, tenantID, policyID, pagination.Params{})
	for _, item := range rows {
		if item.ManagedFileID == managedFileID {
			return item, nil
		}
	}
	return policy.PolicyManagedFile{}, httpx.ErrNotFound
}
func (s *policyStore) AddPolicyManagedFile(context.Context, string, string, string) (policy.PolicyManagedFile, error) {
	return policy.PolicyManagedFile{}, nil
}
func (s *policyStore) RemovePolicyManagedFile(context.Context, string, string, string) error {
	return nil
}

type deviceStore struct {
	now      time.Time
	policyID string
}

func fixtureDevices(now time.Time) []device.Device {
	policyBaseline := "policy-baseline"
	policyKiosk := "policy-kiosk"
	policyRugged := "policy-rugged"
	items := []device.Device{
		{RecordBase: device.RecordBase{ID: "device-001", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -15), UpdatedAt: now.Add(-3 * time.Minute)}, Name: "warehouse-tablet-001", PolicyID: &policyBaseline, GroupIDs: []string{"group-warehouse"}},
		{RecordBase: device.RecordBase{ID: "device-002", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -14), UpdatedAt: now.Add(-12 * time.Minute)}, Name: "warehouse-tablet-002", PolicyID: &policyBaseline, GroupIDs: []string{"group-warehouse"}},
		{RecordBase: device.RecordBase{ID: "device-003", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -13), UpdatedAt: now.Add(-35 * time.Minute)}, Name: "frontdesk-kiosk-003", PolicyID: &policyKiosk, GroupIDs: []string{"group-kiosk"}},
		{RecordBase: device.RecordBase{ID: "device-004", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -12), UpdatedAt: now.Add(-90 * time.Minute)}, Name: "lobby-kiosk-004", PolicyID: &policyKiosk, GroupIDs: []string{"group-kiosk"}},
		{RecordBase: device.RecordBase{ID: "device-005", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -11), UpdatedAt: now.Add(-2 * time.Hour)}, Name: "field-phone-005", PolicyID: &policyRugged, GroupIDs: []string{"group-field", "group-rugged"}},
		{RecordBase: device.RecordBase{ID: "device-006", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -10), UpdatedAt: now.Add(-5 * time.Hour)}, Name: "field-phone-006", PolicyID: &policyRugged, GroupIDs: []string{"group-field", "group-rugged"}},
		{RecordBase: device.RecordBase{ID: "device-007", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -9), UpdatedAt: now.Add(-26 * time.Hour)}, Name: "delivery-handheld-007", PolicyID: &policyRugged, GroupIDs: []string{"group-field", "group-rugged"}},
		{RecordBase: device.RecordBase{ID: "device-008", TenantID: tenantID, Status: "pending", CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now.Add(-4 * time.Hour)}, Name: "pending-tablet-008", PolicyID: &policyBaseline, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-009", TenantID: tenantID, Status: "pending", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)}, Name: "pending-kiosk-009", PolicyID: &policyKiosk, GroupIDs: []string{"group-kiosk"}},
		{RecordBase: device.RecordBase{ID: "device-010", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -8), UpdatedAt: now.Add(-8 * time.Minute)}, Name: "warehouse-scanner-010", PolicyID: &policyRugged, GroupIDs: []string{"group-warehouse", "group-rugged"}},
		{RecordBase: device.RecordBase{ID: "device-011", TenantID: tenantID, Status: "retired", CreatedAt: now.AddDate(0, 0, -25), UpdatedAt: now.AddDate(0, 0, -1)}, Name: "retired-tablet-011", PolicyID: &policyBaseline, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-012", TenantID: tenantID, Status: "wiped", CreatedAt: now.AddDate(0, 0, -18), UpdatedAt: now.Add(-12 * time.Hour)}, Name: "wiped-phone-012", PolicyID: &policyBaseline, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-013", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -7), UpdatedAt: now.Add(-45 * time.Minute)}, Name: "nurse-station-013", PolicyID: &policyBaseline, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-014", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -5), UpdatedAt: now.Add(-16 * time.Hour)}, Name: "shared-tablet-014", PolicyID: &policyBaseline, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-015", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -3), UpdatedAt: now.Add(-4 * time.Minute)}, Name: "screening-kiosk-015", PolicyID: &policyKiosk, GroupIDs: []string{"group-kiosk"}},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (s *deviceStore) ListDevices(context.Context, string, pagination.Params) ([]device.Device, error) {
	return fixtureDevices(s.now), nil
}
func (s *deviceStore) ListActiveDevices(context.Context, string) ([]device.Device, error) {
	rows := []device.Device{}
	for _, item := range fixtureDevices(s.now) {
		if item.Status == device.StatusActive || item.Status == device.StatusEnrolled {
			rows = append(rows, item)
		}
	}
	return rows, nil
}
func (s *deviceStore) GetOverviewStats(context.Context, string) (device.OverviewStats, error) {
	items := fixtureDevices(s.now)
	stats := device.OverviewStats{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case device.StatusActive:
			stats.Active++
		case device.StatusPending:
			stats.Pending++
		}
		if item.Status == device.StatusRetired || item.Status == device.StatusWiped {
			stats.RetiredOrWiped++
		}
		if item.PolicyID != nil && *item.PolicyID != "" {
			stats.AssignedPolicy++
		}
	}
	return stats, nil
}
func (s *deviceStore) GetStatusCounts(context.Context, string) (device.StatusCounts, error) {
	counts := device.StatusCounts{}
	for _, item := range fixtureDevices(s.now) {
		switch item.Status {
		case device.StatusPending:
			counts.Pending++
		case device.StatusEnrolled:
			counts.Enrolled++
		case device.StatusActive:
			counts.Active++
		case device.StatusLocked:
			counts.Locked++
		case device.StatusSuspended:
			counts.Suspended++
		case device.StatusRetired:
			counts.Retired++
		case device.StatusWiped:
			counts.Wiped++
		}
	}
	return counts, nil
}
func (s *deviceStore) GetDevice(_ context.Context, _ string, id string) (device.Device, error) {
	for _, item := range fixtureDevices(s.now) {
		if item.ID == id {
			return item, nil
		}
	}
	return device.Device{}, httpx.ErrNotFound
}
func (s *deviceStore) CreateDevice(context.Context, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}
func (s *deviceStore) UpdateDevice(context.Context, string, string, device.DeviceUpsert) (device.Device, error) {
	return device.Device{}, nil
}
func (s *deviceStore) RetireDevice(context.Context, string, string) (device.Device, error) {
	return device.Device{}, nil
}
func (s *deviceStore) Authenticate(context.Context, string, string, string) (device.Device, error) {
	return device.Device{}, nil
}

type appStore struct {
	now      time.Time
	artifact files.Artifact
}

func fixtureApps(now time.Time) []apps.App {
	items := []apps.App{
		{RecordBase: apps.RecordBase{ID: "app-chrome", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -10), UpdatedAt: now}, PackageName: "com.android.chrome", Name: "Chrome"},
		{RecordBase: apps.RecordBase{ID: "app-viewer", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -8), UpdatedAt: now.Add(-2 * time.Hour)}, PackageName: "com.example.viewer", Name: "Document Viewer"},
		{RecordBase: apps.RecordBase{ID: "app-agent", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -7), UpdatedAt: now.Add(-30 * time.Minute)}, PackageName: "com.xmdm.agent", Name: "XMDM Agent"},
		{RecordBase: apps.RecordBase{ID: "app-kiosk", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -6), UpdatedAt: now.Add(-90 * time.Minute)}, PackageName: "com.xmdm.kiosk", Name: "Kiosk Launcher"},
		{RecordBase: apps.RecordBase{ID: "app-datawedge", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -5), UpdatedAt: now.Add(-6 * time.Hour)}, PackageName: "com.zebra.datawedge", Name: "DataWedge"},
		{RecordBase: apps.RecordBase{ID: "app-old-demo", TenantID: tenantID, Status: "retired", CreatedAt: now.AddDate(0, 0, -20), UpdatedAt: now.AddDate(0, 0, -2)}, PackageName: "com.example.old", Name: "Old Demo App"},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (s *appStore) ListApps(context.Context, string, pagination.Params) ([]apps.App, error) {
	return fixtureApps(s.now), nil
}
func (s *appStore) GetOverviewStats(_ context.Context, _ string) (apps.OverviewStats, error) {
	items := fixtureApps(s.now)
	var active int
	for _, item := range items {
		if item.Status == apps.StatusActive {
			active++
		}
	}
	return apps.OverviewStats{
		Total:  len(items),
		Active: active,
	}, nil
}
func (s *appStore) GetAppByPackageName(_ context.Context, _ string, packageName string) (apps.App, error) {
	for _, item := range fixtureApps(s.now) {
		if item.PackageName == packageName {
			return item, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}
func (s *appStore) CreateApp(context.Context, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}
func (s *appStore) UpdateApp(context.Context, string, string, apps.AppUpsert) (apps.App, error) {
	return apps.App{}, nil
}
func (s *appStore) RetireApp(context.Context, string, string) (apps.App, error) {
	return apps.App{}, nil
}
func (s *appStore) GetApp(_ context.Context, _ string, id string) (apps.App, error) {
	for _, item := range fixtureApps(s.now) {
		if item.ID == id {
			return item, nil
		}
	}
	return apps.App{}, httpx.ErrNotFound
}
func (s *appStore) GetVersionByCode(_ context.Context, _ string, appID string, versionCode int64) (apps.Version, error) {
	for _, item := range []apps.Version{
		{ID: "version-100", PublishedAt: timePtr(s.now.AddDate(0, 0, -8)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.0.0", VersionCode: 100, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -8)},
		{ID: "version-120", PublishedAt: timePtr(s.now.AddDate(0, 0, -2)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.2.0", VersionCode: 120, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -2)},
		{ID: "version-agent", PublishedAt: timePtr(s.now.AddDate(0, 0, -1)), TenantID: tenantID, AppID: "app-agent", Status: apps.VersionStatusPublished, VersionName: "0.1.0", VersionCode: 1, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -1)},
	} {
		if item.AppID == appID && item.VersionCode == versionCode {
			return item, nil
		}
	}
	return apps.Version{}, httpx.ErrNotFound
}
func (s *appStore) GetLatestPublishedVersion(_ context.Context, _ string, appID string) (apps.Version, error) {
	var found *apps.Version
	for _, item := range []apps.Version{
		{ID: "version-100", PublishedAt: timePtr(s.now.AddDate(0, 0, -8)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.0.0", VersionCode: 100, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -8)},
		{ID: "version-120", PublishedAt: timePtr(s.now.AddDate(0, 0, -2)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.2.0", VersionCode: 120, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -2)},
		{ID: "version-agent", PublishedAt: timePtr(s.now.AddDate(0, 0, -1)), TenantID: tenantID, AppID: "app-agent", Status: apps.VersionStatusPublished, VersionName: "0.1.0", VersionCode: 1, ArtifactID: &s.artifact.ID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -1)},
	} {
		if item.AppID != appID || item.Status != apps.VersionStatusPublished {
			continue
		}
		if found == nil || item.CreatedAt.After(found.CreatedAt) {
			rec := item
			found = &rec
		}
	}
	if found == nil {
		return apps.Version{}, httpx.ErrNotFound
	}
	return *found, nil
}
func (s *appStore) ListVersions(_ context.Context, _ string, appID string, _ pagination.Params) ([]apps.Version, error) {
	artifactID := s.artifact.ID
	switch appID {
	case "app-chrome":
		items := []apps.Version{
			{ID: "version-100", PublishedAt: timePtr(s.now.AddDate(0, 0, -8)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.0.0", VersionCode: 100, ArtifactID: &artifactID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -8)},
			{ID: "version-120", PublishedAt: timePtr(s.now.AddDate(0, 0, -2)), TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.2.0", VersionCode: 120, ArtifactID: &artifactID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -2)},
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		return items, nil
	case "app-agent":
		items := []apps.Version{
			{ID: "version-agent", PublishedAt: timePtr(s.now.AddDate(0, 0, -1)), TenantID: tenantID, AppID: "app-agent", Status: apps.VersionStatusPublished, VersionName: "0.1.0", VersionCode: 1, ArtifactID: &artifactID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.AddDate(0, 0, -1)},
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
		return items, nil
	default:
		return nil, nil
	}
}
func (s *appStore) GetVersion(context.Context, string, string, string) (apps.Version, error) {
	return apps.Version{}, nil
}
func (s *appStore) CreateVersion(context.Context, string, string, apps.VersionUpsert) (apps.Version, error) {
	return apps.Version{}, nil
}

type fileStore struct {
	now      time.Time
	artifact files.Artifact
}

func (s *fileStore) ListFiles(context.Context, string, pagination.Params) ([]files.File, error) {
	items := []files.File{
		{RecordBase: files.RecordBase{ID: "file-apk", TenantID: tenantID, Status: "active", UpdatedAt: s.now}, Name: "launcher.apk", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: s.artifact.MimeType, Artifact: &s.artifact},
		{RecordBase: files.RecordBase{ID: "file-config", TenantID: tenantID, Status: "active", UpdatedAt: s.now.Add(-2 * time.Hour)}, Name: "device-config.txt", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: "text/plain", Artifact: &s.artifact},
		{RecordBase: files.RecordBase{ID: "file-kiosk", TenantID: tenantID, Status: "active", UpdatedAt: s.now.Add(-4 * time.Hour)}, Name: "kiosk-config.json", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: "application/json", Artifact: &s.artifact},
		{RecordBase: files.RecordBase{ID: "file-field", TenantID: tenantID, Status: "active", UpdatedAt: s.now.Add(-6 * time.Hour)}, Name: "field-rules.json", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: "application/json", Artifact: &s.artifact},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}
func (s *fileStore) GetFile(context.Context, string, string) (files.File, error) {
	return files.File{}, nil
}
func (s *fileStore) CreateFile(context.Context, string, files.FileUpsert) (files.File, error) {
	return files.File{}, nil
}
func (s *fileStore) RetireFile(context.Context, string, string) (files.File, error) {
	return files.File{}, nil
}

type managedFileStore struct {
	now      time.Time
	artifact files.Artifact
}

func fixtureManagedFiles(now time.Time, artifact files.Artifact) []managedfiles.ManagedFile {
	base := func(id, name, mime string, updated time.Time) files.File {
		return files.File{RecordBase: files.RecordBase{ID: id, TenantID: tenantID, Status: "active", UpdatedAt: updated}, Name: name, ArtifactID: artifact.ID, Checksum: artifact.Checksum, MimeType: mime, Artifact: &artifact}
	}
	config := base("file-config", "device-config.txt", "text/plain", now.Add(-2*time.Hour))
	kiosk := base("file-kiosk", "kiosk-config.json", "application/json", now.Add(-4*time.Hour))
	field := base("file-field", "field-rules.json", "application/json", now.Add(-6*time.Hour))
	items := []managedfiles.ManagedFile{
		{RecordBase: managedfiles.RecordBase{ID: "managed-file-config", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -10), UpdatedAt: now}, FileID: config.ID, Path: "/sdcard/xmdm/config.txt", ReplaceVariables: true, File: &config},
		{RecordBase: managedfiles.RecordBase{ID: "managed-file-kiosk", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -8), UpdatedAt: now.Add(-2 * time.Hour)}, FileID: kiosk.ID, Path: "/sdcard/xmdm/kiosk.json", ReplaceVariables: true, File: &kiosk},
		{RecordBase: managedfiles.RecordBase{ID: "managed-file-field", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -6), UpdatedAt: now.Add(-4 * time.Hour)}, FileID: field.ID, Path: "/sdcard/xmdm/field-rules.json", ReplaceVariables: false, File: &field},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (s *managedFileStore) ListManagedFiles(context.Context, string, pagination.Params) ([]managedfiles.ManagedFile, error) {
	return fixtureManagedFiles(s.now, s.artifact), nil
}
func (s *managedFileStore) GetOverviewStats(context.Context, string) (managedfiles.OverviewStats, error) {
	items := fixtureManagedFiles(s.now, s.artifact)
	stats := managedfiles.OverviewStats{Total: len(items)}
	for _, item := range items {
		if item.Status == managedfiles.StatusActive {
			stats.Active++
		}
	}
	return stats, nil
}
func (s *managedFileStore) GetManagedFile(_ context.Context, _ string, id string) (managedfiles.ManagedFile, error) {
	for _, item := range fixtureManagedFiles(s.now, s.artifact) {
		if item.ID == id {
			return item, nil
		}
	}
	return managedfiles.ManagedFile{}, httpx.ErrNotFound
}
func (s *managedFileStore) CreateManagedFile(context.Context, string, managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}
func (s *managedFileStore) RetireManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	return managedfiles.ManagedFile{}, nil
}

type certificateStore struct {
	now      time.Time
	artifact files.Artifact
}

func fixtureCertificates(now time.Time, artifact files.Artifact) []certificates.Certificate {
	items := []certificates.Certificate{
		{RecordBase: certificates.RecordBase{ID: "cert-root", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -20), UpdatedAt: now}, Name: "MDM Root", ArtifactID: artifact.ID, Checksum: artifact.Checksum, Artifact: &artifact},
		{RecordBase: certificates.RecordBase{ID: "cert-wifi", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -12), UpdatedAt: now.Add(-3 * time.Hour)}, Name: "Corporate Wi-Fi", ArtifactID: artifact.ID, Checksum: "sha256-wifi-cert", Artifact: &artifact},
		{RecordBase: certificates.RecordBase{ID: "cert-vpn", TenantID: tenantID, Status: "active", CreatedAt: now.AddDate(0, 0, -8), UpdatedAt: now.Add(-6 * time.Hour)}, Name: "VPN Client CA", ArtifactID: artifact.ID, Checksum: "sha256-vpn-cert", Artifact: &artifact},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (s *certificateStore) ListCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	return fixtureCertificates(s.now, s.artifact), nil
}
func (s *certificateStore) ListActiveCertificates(context.Context, string, pagination.Params) ([]certificates.Certificate, error) {
	return fixtureCertificates(s.now, s.artifact), nil
}
func (s *certificateStore) GetOverviewStats(context.Context, string) (certificates.OverviewStats, error) {
	items := fixtureCertificates(s.now, s.artifact)
	stats := certificates.OverviewStats{Total: len(items)}
	for _, item := range items {
		if item.Status == certificates.StatusActive {
			stats.Active++
		}
	}
	return stats, nil
}
func (s *certificateStore) GetCertificate(_ context.Context, _ string, id string) (certificates.Certificate, error) {
	for _, item := range fixtureCertificates(s.now, s.artifact) {
		if item.ID == id {
			return item, nil
		}
	}
	return certificates.Certificate{}, httpx.ErrNotFound
}
func (s *certificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}
func (s *certificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

type commandStore struct{ now time.Time }

func fixtureCommands(now time.Time) []commands.Command {
	statuses := []struct {
		status string
		count  int
	}{
		{commands.StatusAcked, 11},
		{commands.StatusSent, 6},
		{commands.StatusFailed, 4},
		{commands.StatusQueued, 3},
	}
	types := []string{"ping", "sync_policy", "install_app", "rotate_cert", "reboot", "collect_info"}
	devices := []string{"device-001", "device-002", "device-003", "device-004", "device-005", "device-006", "device-007", "device-010", "device-013", "device-014", "device-015"}
	rows := make([]commands.Command, 0, 24)
	idx := 1
	for _, bucket := range statuses {
		for i := 0; i < bucket.count; i++ {
			created := now.Add(-time.Duration(idx*6+(idx%4)*4) * time.Hour)
			rows = append(rows, commands.Command{
				ID:        fmt.Sprintf("cmd-%02d", idx),
				Type:      types[(idx-1)%len(types)],
				Status:    bucket.status,
				DeviceID:  devices[(idx-1)%len(devices)],
				Payload:   map[string]any{"source": "fixture", "attempt": (idx%3 + 1), "priority": []string{"normal", "high", "low"}[idx%3]},
				Result:    commandResult(bucket.status, idx),
				CreatedAt: created,
				UpdatedAt: created.Add(time.Duration(5+idx%12) * time.Minute),
			})
			idx++
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows
}
func commandResult(status string, idx int) map[string]any {
	switch status {
	case commands.StatusAcked:
		return map[string]any{"status": "acknowledged", "latencyMs": 400 + idx*37}
	case commands.StatusFailed:
		return map[string]any{"status": "failed", "error": []string{"timeout", "package not found", "device offline"}[idx%3]}
	default:
		return nil
	}
}
func (s *commandStore) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return []commands.Command{{ID: "cmd-new", Type: "ping", Status: commands.StatusQueued, DeviceID: "device-001"}}, nil
}
func (s *commandStore) ListRecent(context.Context, string, pagination.Params) ([]commands.Command, error) {
	return fixtureCommands(s.now), nil
}
func (s *commandStore) ListRecentAll(context.Context, string) ([]commands.Command, error) {
	return fixtureCommands(s.now), nil
}
func (s *commandStore) GetOverviewStats(context.Context, string) (commands.OverviewStats, error) {
	items := fixtureCommands(s.now)
	stats := commands.OverviewStats{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case commands.StatusSent:
			stats.Sent++
		case commands.StatusAcked:
			stats.Acked++
		case commands.StatusFailed:
			stats.Failed++
		}
	}
	return stats, nil
}
func (s *commandStore) Get(_ context.Context, _ string, commandID string) (commands.Command, error) {
	for _, item := range fixtureCommands(s.now) {
		if item.ID == commandID {
			return item, nil
		}
	}
	return commands.Command{}, httpx.ErrNotFound
}
func (s *commandStore) ListPending(context.Context, string, string, pagination.Params) ([]commands.Command, error) {
	rows := []commands.Command{}
	for _, item := range fixtureCommands(s.now) {
		if item.Status == commands.StatusQueued || item.Status == commands.StatusSent {
			rows = append(rows, item)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows, nil
}
func (s *commandStore) ListPendingForDevice(context.Context, string, string) ([]commands.Command, error) {
	rows := []commands.Command{}
	for _, item := range fixtureCommands(s.now) {
		if item.Status == commands.StatusQueued || item.Status == commands.StatusSent {
			rows = append(rows, item)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows, nil
}

func (s *commandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}

func (s *auditStore) ListNewest(context.Context, string) ([]audit.Event, error) {
	return fixtureAuditEvents(s.now), nil
}

type logStore struct{ now time.Time }

func fixtureLogs(now time.Time) []logs.Record {
	sources := []string{"launcher", "commands", "agent", "policy", "network"}
	levels := []string{"info", "info", "warn", "info", "error"}
	messages := []string{"config applied", "command dispatched", "battery below threshold", "policy refreshed", "network reconnect failed"}
	rows := make([]logs.Record, 0, 28)
	for i := 0; i < 28; i++ {
		deviceID := fmt.Sprintf("device-%03d", (i%15)+1)
		rows = append(rows, logs.Record{
			ID:         fmt.Sprintf("log-%02d", i+1),
			TenantID:   tenantID,
			DeviceID:   deviceID,
			ObservedAt: now.Add(-time.Duration(i*27) * time.Minute),
			Source:     sources[i%len(sources)],
			Level:      levels[i%len(levels)],
			Message:    messages[i%len(messages)],
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservedAt.After(rows[j].ObservedAt) })
	return rows
}
func (s *logStore) Search(_ context.Context, _ string, filter logs.SearchFilter) ([]logs.Record, error) {
	rows := []logs.Record{}
	for _, item := range fixtureLogs(s.now) {
		if filter.DeviceID != "" && item.DeviceID != filter.DeviceID {
			continue
		}
		rows = append(rows, item)
		if filter.Limit > 0 && len(rows) >= filter.Limit {
			break
		}
	}
	return rows, nil
}

type deviceInfoStore struct{ now time.Time }

func fixtureDeviceInfo(now time.Time) []deviceinfo.Record {
	type deviceProfile struct {
		deviceID string
		model    string
		android  string
		network  string
		baseBatt int
		fresh    bool
	}

	profiles := []deviceProfile{
		{"device-001", "Pixel 8 Pro", "14", "wifi", 91, true},
		{"device-002", "Pixel 8 Pro", "14", "wifi", 78, true},
		{"device-003", "Galaxy Tab Active5", "13", "wifi", 64, true},
		{"device-004", "Galaxy Tab Active5", "13", "ethernet", 88, true},
		{"device-005", "Zebra TC58", "13", "cellular", 52, true},
		{"device-006", "Zebra TC58", "13", "cellular", 45, true},
		{"device-007", "Zebra TC58", "13", "offline", 17, true},
		{"device-008", "Pixel 8", "14", "wifi", 73, true},
		{"device-009", "Lenovo Tab K11", "14", "wifi", 80, true},
		{"device-010", "Zebra TC58", "13", "wifi", 68, true},
		{"device-011", "Pixel 8", "14", "offline", 39, true},
		{"device-012", "Galaxy A55", "15", "cellular", 56, false},
		{"device-013", "Pixel 8 Pro", "14", "wifi", 84, true},
		{"device-014", "Lenovo Tab K11", "14", "wifi", 31, false},
		{"device-015", "Galaxy Tab Active5", "13", "ethernet", 96, true},
	}

	// Deliberately uneven sample counts make the model breakdown and activity
	// charts look more like a real fleet rather than a perfectly flat demo.
	samplesByDevice := map[string]int{
		"device-001": 5,
		"device-002": 4,
		"device-003": 4,
		"device-004": 3,
		"device-005": 4,
		"device-006": 3,
		"device-007": 2,
		"device-008": 1,
		"device-009": 1,
		"device-010": 4,
		"device-011": 1,
		"device-012": 1,
		"device-013": 5,
		"device-014": 2,
		"device-015": 4,
	}

	rows := make([]deviceinfo.Record, 0, 44)
	seq := 1
	for i, profile := range profiles {
		count := samplesByDevice[profile.deviceID]
		for sample := 0; sample < count; sample++ {
			var observedAt time.Time
			if profile.fresh {
				// Spread fresh devices across the last 24 hours.
				if i < 7 {
					observedAt = now.Add(-time.Duration(1+i*2+sample) * time.Hour)
				} else {
					observedAt = now.Add(-time.Duration(30+(i*9+sample*7)%90) * time.Hour)
				}
			} else {
				// Stale/pending/retired devices have older telemetry spread across the week.
				observedAt = now.Add(-time.Duration(36+i*3+sample*6) * time.Hour)
			}
			battery := profile.baseBatt - sample*7 - i%5
			if battery < 8 {
				battery = 8 + (seq % 12)
			}
			rows = append(rows, deviceinfo.Record{
				ID:         fmt.Sprintf("info-%02d", seq),
				TenantID:   tenantID,
				DeviceID:   profile.deviceID,
				ObservedAt: observedAt,
				Payload: map[string]any{
					"batteryLevel": battery,
					"model":        profile.model,
					"android":      profile.android,
					"network":      profile.network,
					"charging":     (seq+i)%4 == 0,
					"serialNumber": fmt.Sprintf("SN-%03d-%04x", i+1, 9000+seq),
				},
			})
			seq++
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservedAt.After(rows[j].ObservedAt) })
	return rows
}
func (s *deviceInfoStore) Search(_ context.Context, _ string, filter deviceinfo.SearchFilter) ([]deviceinfo.Record, error) {
	rows := []deviceinfo.Record{}
	for _, item := range fixtureDeviceInfo(s.now) {
		if filter.DeviceID != "" && item.DeviceID != filter.DeviceID {
			continue
		}
		rows = append(rows, item)
		if filter.Limit > 0 && len(rows) >= filter.Limit {
			break
		}
	}
	return rows, nil
}

type auditStore struct{ now time.Time }

func (s *auditStore) Record(context.Context, string, string, string, string, string, map[string]any) (audit.Event, error) {
	return audit.Event{}, nil
}

func fixtureAuditEvents(now time.Time) []audit.Event {
	events := []struct {
		actor        string
		action       string
		resourceType string
		resourceID   string
		details      map[string]any
	}{
		{"admin", "create", "devices", "device-001", map[string]any{"name": "warehouse-tablet-001"}},
		{"ops@example.com", "update", "policies", "policy-baseline", map[string]any{"version": 5}},
		{"system", "assign", "policy_apps", "policy-baseline:app-agent", map[string]any{"appId": "app-agent", "enabled": true}},
		{"auditor@example.com", "enqueue", "commands", "cmd-04", map[string]any{"type": "rotate_cert"}},
		{"admin", "retire", "devices", "device-011", map[string]any{"status": "retired"}},
		{"ops@example.com", "revoke", "enrollment_tokens", "token-3", map[string]any{"reason": "duplicate enrollment"}},
		{"system", "create", "managed_files", "managed-file-kiosk", map[string]any{"path": "/sdcard/xmdm/kiosk.json"}},
		{"admin", "create", "certificates", "cert-wifi", map[string]any{"name": "Corporate Wi-Fi"}},
		{"ops@example.com", "update", "devices", "device-010", map[string]any{"groups": []string{"group-warehouse", "group-rugged"}}},
		{"system", "enqueue", "commands", "cmd-09", map[string]any{"type": "install_app"}},
		{"admin", "assign", "policy_managed_files", "policy-kiosk:managed-file-kiosk", map[string]any{"enabled": true}},
		{"ops@example.com", "update", "apps", "app-kiosk", map[string]any{"package": "com.xmdm.kiosk"}},
		{"system", "create", "enrollment_tokens", "token-4", map[string]any{"expiresInHours": 8}},
		{"admin", "update", "policies", "policy-kiosk", map[string]any{"kioskMode": true, "version": 8}},
		{"ops@example.com", "enqueue", "commands", "cmd-14", map[string]any{"type": "collect_info"}},
		{"system", "assign", "policy_certificates", "policy-kiosk:cert-wifi", map[string]any{"certificateId": "cert-wifi"}},
		{"admin", "update", "devices", "device-015", map[string]any{"name": "screening-kiosk-015"}},
		{"system", "create", "logs", "log-18", map[string]any{"level": "warn"}},
		{"ops@example.com", "download", "certificates", "cert-root", map[string]any{"format": "pem"}},
		{"admin", "create", "apps", "app-datawedge", map[string]any{"name": "DataWedge"}},
		{"system", "update", "managed_files", "managed-file-field", map[string]any{"replaceVariables": false}},
		{"auditor@example.com", "download", "audit", "audit-export-01", map[string]any{"window": "7d"}},
		{"ops@example.com", "enqueue", "commands", "cmd-21", map[string]any{"type": "sync_policy"}},
		{"system", "update", "devices", "device-007", map[string]any{"lastSeen": "stale"}},
		{"admin", "create", "devices", "device-008", map[string]any{"status": "pending"}},
		{"ops@example.com", "create", "devices", "device-009", map[string]any{"status": "pending"}},
		{"system", "fail", "commands", "cmd-18", map[string]any{"error": "device offline"}},
		{"admin", "update", "groups", "group-rugged", map[string]any{"name": "Rugged Handhelds"}},
		{"system", "ack", "commands", "cmd-01", map[string]any{"latencyMs": 437}},
		{"ops@example.com", "update", "certificates", "cert-vpn", map[string]any{"name": "VPN Client CA"}},
		{"admin", "create", "policies", "policy-rugged", map[string]any{"version": 3}},
		{"system", "expire", "enrollment_tokens", "token-2", map[string]any{"status": "consumed"}},
	}

	rows := make([]audit.Event, 0, len(events))
	for i, event := range events {
		rows = append(rows, audit.Event{
			ID:           fmt.Sprintf("audit-%02d", i+1),
			TenantID:     tenantID,
			Actor:        event.actor,
			Action:       event.action,
			ResourceType: event.resourceType,
			ResourceID:   event.resourceID,
			CreatedAt:    now.Add(-time.Duration(i*2+(i%4)*2) * time.Hour),
			Details:      event.details,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows
}

func (s *auditStore) List(_ context.Context, _ string, params pagination.Params) ([]audit.Event, error) {
	items := fixtureAuditEvents(s.now)
	if params.Offset > 0 {
		if params.Offset >= len(items) {
			return []audit.Event{}, nil
		}
		items = items[params.Offset:]
	}
	if params.Limit > 0 && params.Limit < len(items) {
		items = items[:params.Limit]
	}
	return items, nil
}

func (s *auditStore) CountSince(context.Context, string, time.Time) (int, error) {
	return 2, nil
}

type enrollmentStore struct{ now time.Time }

func (s *enrollmentStore) IssueToken(context.Context, string, time.Time) (enrollment.IssuedToken, error) {
	return enrollment.IssuedToken{Token: enrollment.Token{ID: "token-1", TenantID: tenantID, Status: enrollment.TokenStatusIssued, ExpiresAt: s.now.Add(2 * time.Hour), CreatedAt: s.now, UpdatedAt: s.now}, Secret: "sample-enrollment-token"}, nil
}
func (s *enrollmentStore) ValidateToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{ID: "token-1", TenantID: tenantID, Status: enrollment.TokenStatusIssued, ExpiresAt: s.now.Add(2 * time.Hour), CreatedAt: s.now, UpdatedAt: s.now}, nil
}
func (s *enrollmentStore) ConsumeToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{}, nil
}
func (s *enrollmentStore) BindDevice(context.Context, string, string, string, map[string]any) (enrollment.BoundDevice, error) {
	return enrollment.BoundDevice{}, nil
}
func (s *enrollmentStore) ListTokens(context.Context, string, pagination.Params) ([]enrollment.Token, error) {
	items := []enrollment.Token{
		{ID: "token-1", TenantID: tenantID, Status: enrollment.TokenStatusIssued, ExpiresAt: s.now.Add(2 * time.Hour), CreatedAt: s.now.Add(-10 * time.Minute), UpdatedAt: s.now},
		{ID: "token-2", TenantID: tenantID, Status: enrollment.TokenStatusConsumed, ExpiresAt: s.now.Add(-30 * time.Minute), ConsumedAt: timePtr(s.now.Add(-25 * time.Minute)), CreatedAt: s.now.Add(-90 * time.Minute), UpdatedAt: s.now},
		{ID: "token-3", TenantID: tenantID, Status: enrollment.TokenStatusRevoked, ExpiresAt: s.now.Add(4 * time.Hour), RevokedAt: timePtr(s.now.Add(-40 * time.Minute)), CreatedAt: s.now.Add(-2 * time.Hour), UpdatedAt: s.now},
		{ID: "token-4", TenantID: tenantID, Status: enrollment.TokenStatusIssued, ExpiresAt: s.now.Add(8 * time.Hour), CreatedAt: s.now.Add(-3 * time.Hour), UpdatedAt: s.now},
		{ID: "token-5", TenantID: tenantID, Status: enrollment.TokenStatusConsumed, ExpiresAt: s.now.Add(12 * time.Hour), ConsumedAt: timePtr(s.now.Add(-4 * time.Hour)), CreatedAt: s.now.Add(-5 * time.Hour), UpdatedAt: s.now.Add(-4 * time.Hour)},
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (s *enrollmentStore) RevokeToken(context.Context, string, string) (enrollment.Token, error) {
	return enrollment.Token{ID: "token-1", TenantID: tenantID, Status: enrollment.TokenStatusRevoked, ExpiresAt: s.now.Add(2 * time.Hour), CreatedAt: s.now, UpdatedAt: s.now}, nil
}
func (s *enrollmentStore) ExpireTokens(context.Context, time.Time) (int64, error) { return 0, nil }

func timePtr(t time.Time) *time.Time { return &t }

type artifactStore struct{}

func (artifactStore) Put(context.Context, string, io.Reader, string, int64) error { return nil }
func (artifactStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("artifact"))), nil
}
func (artifactStore) Delete(context.Context, string) error { return nil }

var _ users.Repository = (*identityStore)(nil)
var _ roles.Repository = (*identityStore)(nil)
var _ group.Repository = (*groupStore)(nil)
var _ policy.Repository = (*policyStore)(nil)
var _ device.Repository = (*deviceStore)(nil)
var _ apps.Repository = (*appStore)(nil)
var _ files.Repository = (*fileStore)(nil)
var _ managedfiles.Repository = (*managedFileStore)(nil)
var _ certificates.Repository = (*certificateStore)(nil)
var _ commands.Repository = (*commandStore)(nil)
var _ audit.Store = (*auditStore)(nil)
var _ enrollment.Repository = (*enrollmentStore)(nil)
var _ artifacts.Store = artifactStore{}
