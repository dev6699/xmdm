package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
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
	"xmdm/server/internal/identity"
	"xmdm/server/internal/logs"
	managedfiles "xmdm/server/internal/managedfiles"
	"xmdm/server/internal/policy"
)

const tenantID = "00000000-0000-0000-0000-000000000000"

func main() {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	fileArtifact := files.Artifact{RecordBase: files.RecordBase{ID: "artifact-apk", TenantID: tenantID, Status: "active", UpdatedAt: now}, StorageKey: "artifacts/launcher.apk", Checksum: "sha256-apk", SizeBytes: 2048, MimeType: "application/vnd.android.package-archive"}
	certArtifact := files.Artifact{RecordBase: files.RecordBase{ID: "artifact-cert", TenantID: tenantID, Status: "active", UpdatedAt: now}, StorageKey: "certs/root.pem", Checksum: "sha256-cert", SizeBytes: 512, MimeType: "application/x-pem-file"}
	policyID := "policy-baseline"
	svc := auth.NewService("admin", "admin", time.Hour)
	mux := http.NewServeMux()
	adminhttp.RegisterDashboard(mux, svc, adminhttp.DashboardDependencies{
		Identity:     &identityStore{now: now},
		Groups:       &groupStore{now: now},
		Policies:     &policyStore{now: now},
		Devices:      &deviceStore{now: now, policyID: policyID},
		Apps:         &appStore{now: now, artifact: fileArtifact},
		Files:        &fileStore{now: now, artifact: fileArtifact},
		ManagedFiles: &managedFileStore{now: now, artifact: fileArtifact},
		Certificates: &certificateStore{now: now, artifact: certArtifact},
		Commands:     &commandStore{now: now},
		Logs:         &logStore{now: now},
		DeviceInfo:   &deviceInfoStore{now: now},
		Audit:        &auditStore{now: now},
		Enrollment:   &enrollmentStore{now: now},
		Runtime:      enrollment.RuntimeSnapshot{},
		Artifacts:    artifactStore{},
		TenantID:     tenantID,
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:39091", mux))
}

type identityStore struct{ now time.Time }

func (s *identityStore) ListUsers(context.Context, string) ([]identity.User, error) {
	return []identity.User{
		{RecordBase: identity.RecordBase{ID: "user-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-3 * time.Hour), UpdatedAt: s.now}, Email: "admin@example.com", RoleID: "role-admin"},
		{RecordBase: identity.RecordBase{ID: "user-ops", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-2 * time.Hour), UpdatedAt: s.now}, Email: "ops@example.com", RoleID: "role-operator"},
	}, nil
}
func (s *identityStore) CreateUser(context.Context, string, identity.UserUpsert) (identity.User, error) {
	return identity.User{}, nil
}
func (s *identityStore) UpdateUser(context.Context, string, string, identity.UserUpsert) (identity.User, error) {
	return identity.User{}, nil
}
func (s *identityStore) RetireUser(context.Context, string, string) (identity.User, error) {
	return identity.User{}, nil
}
func (s *identityStore) AuthenticateUser(context.Context, string, string, string) (identity.User, identity.Role, error) {
	return identity.User{}, identity.Role{}, nil
}
func (s *identityStore) ListRoles(context.Context, string) ([]identity.Role, error) {
	return []identity.Role{
		{RecordBase: identity.RecordBase{ID: "role-admin", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-4 * time.Hour), UpdatedAt: s.now}, Name: "Administrators", Permissions: []string{"admin.read", "admin.write"}},
		{RecordBase: identity.RecordBase{ID: "role-read", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-3 * time.Hour), UpdatedAt: s.now}, Name: "Read Only", Permissions: []string{"admin.read"}},
	}, nil
}
func (s *identityStore) CreateRole(context.Context, string, identity.RoleUpsert) (identity.Role, error) {
	return identity.Role{}, nil
}
func (s *identityStore) UpdateRole(context.Context, string, string, identity.RoleUpsert) (identity.Role, error) {
	return identity.Role{}, nil
}
func (s *identityStore) RetireRole(context.Context, string, string) (identity.Role, error) {
	return identity.Role{}, nil
}

type groupStore struct{ now time.Time }

func (s *groupStore) ListGroups(context.Context, string) ([]group.Group, error) {
	return []group.Group{{RecordBase: group.RecordBase{ID: "group-field", TenantID: tenantID, Status: "active", CreatedAt: s.now, UpdatedAt: s.now}, Name: "Field Devices"}, {RecordBase: group.RecordBase{ID: "group-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now, UpdatedAt: s.now}, Name: "Kiosk Fleet"}}, nil
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

func (s *policyStore) ListPolicies(context.Context, string) ([]policy.Policy, error) {
	return []policy.Policy{
		{RecordBase: policy.RecordBase{ID: "policy-baseline", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-5 * time.Hour), UpdatedAt: s.now}, Name: "Baseline", Version: 2, KioskMode: false, Restrictions: []byte(`{"allowPackages":["com.android.chrome"]}`)},
		{RecordBase: policy.RecordBase{ID: "policy-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-4 * time.Hour), UpdatedAt: s.now}, Name: "Kiosk", Version: 4, KioskMode: true, KioskAppPackage: "com.android.chrome", Restrictions: []byte(`{"kioskExitPasscode":"1234"}`)},
	}, nil
}
func (s *policyStore) GetPolicy(_ context.Context, _ string, id string) (policy.Policy, error) {
	for _, item := range []policy.Policy{
		{RecordBase: policy.RecordBase{ID: "policy-baseline", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-5 * time.Hour), UpdatedAt: s.now}, Name: "Baseline", Version: 2, KioskMode: false, Restrictions: []byte(`{"allowPackages":["com.android.chrome"]}`)},
		{RecordBase: policy.RecordBase{ID: "policy-kiosk", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-4 * time.Hour), UpdatedAt: s.now}, Name: "Kiosk", Version: 4, KioskMode: true, KioskAppPackage: "com.android.chrome", Restrictions: []byte(`{"kioskExitPasscode":"1234"}`)},
	} {
		if item.ID == id {
			return item, nil
		}
	}
	return policy.Policy{}, nil
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
func (s *policyStore) ListPolicyApps(context.Context, string, string) ([]policy.PolicyApp, error) {
	return nil, nil
}
func (s *policyStore) AddPolicyApp(context.Context, string, string, string) (policy.PolicyApp, error) {
	return policy.PolicyApp{}, nil
}
func (s *policyStore) RemovePolicyApp(context.Context, string, string, string) error { return nil }
func (s *policyStore) ListPolicyCertificates(context.Context, string, string) ([]policy.PolicyCertificate, error) {
	return nil, nil
}
func (s *policyStore) AddPolicyCertificate(context.Context, string, string, string) (policy.PolicyCertificate, error) {
	return policy.PolicyCertificate{}, nil
}
func (s *policyStore) RemovePolicyCertificate(context.Context, string, string, string) error {
	return nil
}
func (s *policyStore) ListPolicyManagedFiles(context.Context, string, string) ([]policy.PolicyManagedFile, error) {
	return nil, nil
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

func (s *deviceStore) ListDevices(context.Context, string) ([]device.Device, error) {
	return []device.Device{
		{RecordBase: device.RecordBase{ID: "device-001", TenantID: tenantID, Status: "active", CreatedAt: s.now, UpdatedAt: s.now}, DeviceID: "device-uid-001", Name: "warehouse-tablet-001", PolicyID: &s.policyID, GroupIDs: []string{"group-field"}},
		{RecordBase: device.RecordBase{ID: "device-002", TenantID: tenantID, Status: "pending", CreatedAt: s.now, UpdatedAt: s.now}, DeviceID: "device-uid-002", Name: "field-phone-002", PolicyID: &s.policyID},
	}, nil
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

func (s *appStore) ListApps(context.Context, string) ([]apps.App, error) {
	return []apps.App{
		{RecordBase: apps.RecordBase{ID: "app-chrome", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-2 * time.Hour), UpdatedAt: s.now}, PackageName: "com.android.chrome", Name: "Chrome"},
		{RecordBase: apps.RecordBase{ID: "app-viewer", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-90 * time.Minute), UpdatedAt: s.now}, PackageName: "com.example.viewer", Name: "Document Viewer"},
	}, nil
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
func (s *appStore) GetApp(context.Context, string, string) (apps.App, error) {
	return apps.App{RecordBase: apps.RecordBase{ID: "app-chrome", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-2 * time.Hour), UpdatedAt: s.now}, PackageName: "com.android.chrome", Name: "Chrome"}, nil
}
func (s *appStore) ListVersions(context.Context, string, string) ([]apps.Version, error) {
	artifactID := s.artifact.ID
	return []apps.Version{{ID: "version-100", TenantID: tenantID, AppID: "app-chrome", Status: apps.VersionStatusPublished, VersionName: "1.0.0", VersionCode: 100, ArtifactID: &artifactID, Artifact: &s.artifact, Checksum: s.artifact.Checksum, CreatedAt: s.now.Add(-75 * time.Minute)}}, nil
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

func (s *fileStore) ListFiles(context.Context, string) ([]files.File, error) {
	return []files.File{{RecordBase: files.RecordBase{ID: "file-apk", TenantID: tenantID, Status: "active", UpdatedAt: s.now}, Name: "launcher.apk", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: s.artifact.MimeType, Artifact: &s.artifact}}, nil
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

func (s *managedFileStore) ListManagedFiles(context.Context, string) ([]managedfiles.ManagedFile, error) {
	f := files.File{RecordBase: files.RecordBase{ID: "file-config", TenantID: tenantID, Status: "active", UpdatedAt: s.now}, Name: "device-config.txt", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: "text/plain", Artifact: &s.artifact}
	return []managedfiles.ManagedFile{{RecordBase: managedfiles.RecordBase{ID: "managed-file-1", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-100 * time.Minute), UpdatedAt: s.now}, FileID: f.ID, Path: "/sdcard/xmdm/config.txt", ReplaceVariables: true, File: &f}}, nil
}
func (s *managedFileStore) GetManagedFile(context.Context, string, string) (managedfiles.ManagedFile, error) {
	f := files.File{RecordBase: files.RecordBase{ID: "file-config", TenantID: tenantID, Status: "active", UpdatedAt: s.now}, Name: "device-config.txt", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, MimeType: "text/plain", Artifact: &s.artifact}
	return managedfiles.ManagedFile{RecordBase: managedfiles.RecordBase{ID: "managed-file-1", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-100 * time.Minute), UpdatedAt: s.now}, FileID: f.ID, Path: "/sdcard/xmdm/config.txt", ReplaceVariables: true, File: &f}, nil
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

func (s *certificateStore) ListCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return []certificates.Certificate{{RecordBase: certificates.RecordBase{ID: "cert-root", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-80 * time.Minute), UpdatedAt: s.now}, Name: "MDM Root", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, Artifact: &s.artifact}}, nil
}
func (s *certificateStore) ListActiveCertificates(context.Context, string) ([]certificates.Certificate, error) {
	return s.ListCertificates(context.Background(), tenantID)
}
func (s *certificateStore) GetCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{RecordBase: certificates.RecordBase{ID: "cert-root", TenantID: tenantID, Status: "active", CreatedAt: s.now.Add(-80 * time.Minute), UpdatedAt: s.now}, Name: "MDM Root", ArtifactID: s.artifact.ID, Checksum: s.artifact.Checksum, Artifact: &s.artifact}, nil
}
func (s *certificateStore) CreateCertificate(context.Context, string, certificates.CertificateUpsert) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}
func (s *certificateStore) RetireCertificate(context.Context, string, string) (certificates.Certificate, error) {
	return certificates.Certificate{}, nil
}

type commandStore struct{ now time.Time }

func (s *commandStore) Enqueue(context.Context, string, commands.Upsert) ([]commands.Command, error) {
	return []commands.Command{{ID: "cmd-new", Type: "ping", Status: commands.StatusQueued, DeviceID: "device-001"}}, nil
}
func (s *commandStore) ListRecent(context.Context, string, int) ([]commands.Command, error) {
	return []commands.Command{
		{ID: "cmd-1", Type: "ping", Status: commands.StatusAcked, DeviceID: "device-001", CreatedAt: s.now.Add(-20 * time.Minute), UpdatedAt: s.now.Add(-15 * time.Minute)},
		{ID: "cmd-2", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-002", CreatedAt: s.now.Add(-10 * time.Minute), UpdatedAt: s.now.Add(-5 * time.Minute)},
	}, nil
}
func (s *commandStore) Get(_ context.Context, _ string, commandID string) (commands.Command, error) {
	switch commandID {
	case "cmd-1":
		return commands.Command{ID: "cmd-1", Type: "ping", Status: commands.StatusAcked, DeviceID: "device-001", Payload: map[string]any{"reason": "fixture"}, Result: map[string]any{"status": commands.StatusAcked}, CreatedAt: s.now.Add(-20 * time.Minute), UpdatedAt: s.now.Add(-15 * time.Minute)}, nil
	case "cmd-2":
		return commands.Command{ID: "cmd-2", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-002", Payload: map[string]any{"force": true}, CreatedAt: s.now.Add(-10 * time.Minute), UpdatedAt: s.now.Add(-5 * time.Minute)}, nil
	default:
		return commands.Command{}, httpx.ErrNotFound
	}
}
func (s *commandStore) ListPending(context.Context, string, string) ([]commands.Command, error) {
	return []commands.Command{{ID: "cmd-2", Type: "reboot", Status: commands.StatusQueued, DeviceID: "device-002", CreatedAt: s.now.Add(-10 * time.Minute), UpdatedAt: s.now.Add(-5 * time.Minute)}}, nil
}
func (s *commandStore) Acknowledge(context.Context, string, string, string, commands.Ack) (commands.Command, error) {
	return commands.Command{}, nil
}

type logStore struct{ now time.Time }

func (s *logStore) Search(context.Context, string, logs.SearchFilter) ([]logs.Record, error) {
	return []logs.Record{{ID: "log-1", TenantID: tenantID, DeviceID: "device-uid-001", ObservedAt: s.now, Source: "launcher", Level: "info", Message: "config applied"}, {ID: "log-2", TenantID: tenantID, DeviceID: "device-uid-002", ObservedAt: s.now, Source: "commands", Level: "warn", Message: "waiting for acknowledgement"}}, nil
}

type deviceInfoStore struct{ now time.Time }

func (s *deviceInfoStore) Search(context.Context, string, deviceinfo.SearchFilter) ([]deviceinfo.Record, error) {
	return []deviceinfo.Record{{ID: "info-1", TenantID: tenantID, DeviceID: "device-uid-001", ObservedAt: s.now, Payload: map[string]any{"model": "Pixel 8", "battery": 86, "network": "wifi"}}}, nil
}

type auditStore struct{ now time.Time }

func (s *auditStore) Record(context.Context, string, string, string, string, string, map[string]any) (audit.Event, error) {
	return audit.Event{}, nil
}
func (s *auditStore) List(context.Context, string) ([]audit.Event, error) {
	return []audit.Event{{ID: "audit-1", TenantID: tenantID, Actor: "admin", Action: "create", ResourceType: "devices", ResourceID: "device-001", CreatedAt: s.now, Details: map[string]any{"name": "warehouse-tablet-001"}}, {ID: "audit-2", TenantID: tenantID, Actor: "admin", Action: "create", ResourceType: "commands", ResourceID: "cmd-2", CreatedAt: s.now, Details: map[string]any{"type": "reboot"}}}, nil
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
func (s *enrollmentStore) ListTokens(context.Context, string) ([]enrollment.Token, error) {
	return []enrollment.Token{
		{ID: "token-1", TenantID: tenantID, Status: enrollment.TokenStatusIssued, ExpiresAt: s.now.Add(2 * time.Hour), CreatedAt: s.now.Add(-10 * time.Minute), UpdatedAt: s.now},
		{ID: "token-2", TenantID: tenantID, Status: enrollment.TokenStatusConsumed, ExpiresAt: s.now.Add(-30 * time.Minute), ConsumedAt: timePtr(s.now.Add(-25 * time.Minute)), CreatedAt: s.now.Add(-90 * time.Minute), UpdatedAt: s.now},
		{ID: "token-3", TenantID: tenantID, Status: enrollment.TokenStatusRevoked, ExpiresAt: s.now.Add(4 * time.Hour), RevokedAt: timePtr(s.now.Add(-40 * time.Minute)), CreatedAt: s.now.Add(-2 * time.Hour), UpdatedAt: s.now},
	}, nil
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

var _ identity.Repository = (*identityStore)(nil)
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
