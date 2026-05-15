package policy

import "context"

type Repository interface {
	ListPolicies(ctx context.Context, tenantID string) ([]Policy, error)
	GetPolicy(ctx context.Context, tenantID, id string) (Policy, error)
	CreatePolicy(ctx context.Context, tenantID string, req PolicyUpsert) (Policy, error)
	UpdatePolicy(ctx context.Context, tenantID, id string, req PolicyUpsert) (Policy, error)
	RetirePolicy(ctx context.Context, tenantID, id string) (Policy, error)
	ListPolicyApps(ctx context.Context, tenantID, policyID string) ([]PolicyApp, error)
	AddPolicyApp(ctx context.Context, tenantID, policyID, appID string) (PolicyApp, error)
	RemovePolicyApp(ctx context.Context, tenantID, policyID, appID string) error
	ListPolicyCertificates(ctx context.Context, tenantID, policyID string) ([]PolicyCertificate, error)
	AddPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (PolicyCertificate, error)
	RemovePolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) error
	ListPolicyManagedFiles(ctx context.Context, tenantID, policyID string) ([]PolicyManagedFile, error)
	AddPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (PolicyManagedFile, error)
	RemovePolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) error
}
