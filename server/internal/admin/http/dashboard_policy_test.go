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
	"xmdm/server/internal/policy"
)

func TestPolicyMutationsRecordAudit(t *testing.T) {
	svc := auth.NewServiceWithPermissions("admin", "secret", time.Hour, []auth.Permission{auth.PermissionAdminWrite})
	session := svc.IssueSession("admin", []auth.Permission{auth.PermissionAdminWrite})
	store := &recordingPolicyStore{
		policy: policy.Policy{
			RecordBase: policy.RecordBase{ID: "policy-1", TenantID: "tenant-1", Status: policy.StatusActive},
			Name:       "Default",
			Version:    1,
		},
	}
	auditStore := &recordingAuditStore{}
	mux := http.NewServeMux()
	RegisterDashboard(mux, svc, DashboardDependencies{
		Policies: store,
		Audit:    auditStore,
		TenantID: "tenant-1",
	})

	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session.ID})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	t.Run("create policy", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/policies/create", "name=Default&csrfToken=token")
		assertRedirect(t, rr, "/admin/policies?ok=policy+created")
		assertAuditRecord(t, auditStore, "create", "policies", "policy-1")
	})

	t.Run("retire policy", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/policies/policy-1/retire", "csrfToken=token")
		assertRedirect(t, rr, "/admin/policies/policy-1?ok=policy+retired")
		assertAuditRecord(t, auditStore, "retire", "policies", "policy-1")
	})

	t.Run("toggle policy app", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/policies/policy-1/apps/app-1/toggle", "csrfToken=token")
		assertRedirect(t, rr, "/admin/policies/policy-1?ok=app+enabled")
		assertAuditRecord(t, auditStore, "update", "policy_apps", "policy-1:app-1")
	})

	t.Run("toggle policy certificate", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/policies/policy-1/certificates/cert-1/toggle", "csrfToken=token")
		assertRedirect(t, rr, "/admin/policies/policy-1?ok=certificate+enabled")
		assertAuditRecord(t, auditStore, "update", "policy_certificates", "policy-1:cert-1")
	})

	t.Run("toggle policy managed file", func(t *testing.T) {
		auditStore.records = nil
		rr := post("/admin/policies/policy-1/managed-files/file-1/toggle", "csrfToken=token")
		assertRedirect(t, rr, "/admin/policies/policy-1?ok=managed+file+enabled")
		assertAuditRecord(t, auditStore, "update", "policy_managed_files", "policy-1:file-1")
	})
}

type recordingPolicyStore struct {
	policy policy.Policy
}

func (s *recordingPolicyStore) ListPolicies(context.Context, string, pagination.Params) ([]policy.Policy, error) {
	return []policy.Policy{s.policy}, nil
}

func (s *recordingPolicyStore) ListActivePolicies(context.Context, string) ([]policy.Policy, error) {
	return []policy.Policy{s.policy}, nil
}

func (s *recordingPolicyStore) GetOverviewStats(context.Context, string) (policy.OverviewStats, error) {
	return policy.OverviewStats{}, nil
}

func (s *recordingPolicyStore) GetPolicy(context.Context, string, string) (policy.Policy, error) {
	return s.policy, nil
}

func (s *recordingPolicyStore) CreatePolicy(context.Context, string, policy.PolicyUpsert) (policy.Policy, error) {
	return s.policy, nil
}

func (s *recordingPolicyStore) UpdatePolicy(context.Context, string, string, policy.PolicyUpsert) (policy.Policy, error) {
	return s.policy, nil
}

func (s *recordingPolicyStore) RetirePolicy(context.Context, string, string) (policy.Policy, error) {
	return s.policy, nil
}

func (s *recordingPolicyStore) ListPolicyApps(context.Context, string, string, pagination.Params) ([]policy.PolicyApp, error) {
	return nil, nil
}

func (s *recordingPolicyStore) ListActivePolicyApps(context.Context, string, string) ([]policy.PolicyApp, error) {
	return nil, nil
}

func (s *recordingPolicyStore) GetPolicyApp(context.Context, string, string, string) (policy.PolicyApp, error) {
	return policy.PolicyApp{}, httpx.ErrNotFound
}

func (s *recordingPolicyStore) AddPolicyApp(context.Context, string, string, string) (policy.PolicyApp, error) {
	return policy.PolicyApp{RecordBase: policy.RecordBase{ID: "binding-1", TenantID: "tenant-1", Status: policy.StatusActive}}, nil
}

func (s *recordingPolicyStore) RemovePolicyApp(context.Context, string, string, string) error {
	return nil
}

func (s *recordingPolicyStore) ListPolicyCertificates(context.Context, string, string, pagination.Params) ([]policy.PolicyCertificate, error) {
	return nil, nil
}

func (s *recordingPolicyStore) ListActivePolicyCertificates(context.Context, string, string) ([]policy.PolicyCertificate, error) {
	return nil, nil
}

func (s *recordingPolicyStore) GetPolicyCertificate(context.Context, string, string, string) (policy.PolicyCertificate, error) {
	return policy.PolicyCertificate{}, httpx.ErrNotFound
}

func (s *recordingPolicyStore) AddPolicyCertificate(context.Context, string, string, string) (policy.PolicyCertificate, error) {
	return policy.PolicyCertificate{RecordBase: policy.RecordBase{ID: "binding-1", TenantID: "tenant-1", Status: policy.StatusActive}}, nil
}

func (s *recordingPolicyStore) RemovePolicyCertificate(context.Context, string, string, string) error {
	return nil
}

func (s *recordingPolicyStore) ListPolicyManagedFiles(context.Context, string, string, pagination.Params) ([]policy.PolicyManagedFile, error) {
	return nil, nil
}

func (s *recordingPolicyStore) ListActivePolicyManagedFiles(context.Context, string, string) ([]policy.PolicyManagedFile, error) {
	return nil, nil
}

func (s *recordingPolicyStore) GetPolicyManagedFile(context.Context, string, string, string) (policy.PolicyManagedFile, error) {
	return policy.PolicyManagedFile{}, httpx.ErrNotFound
}

func (s *recordingPolicyStore) AddPolicyManagedFile(context.Context, string, string, string) (policy.PolicyManagedFile, error) {
	return policy.PolicyManagedFile{RecordBase: policy.RecordBase{ID: "binding-1", TenantID: "tenant-1", Status: policy.StatusActive}}, nil
}

func (s *recordingPolicyStore) RemovePolicyManagedFile(context.Context, string, string, string) error {
	return nil
}
