package policy

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListPolicies(ctx context.Context, tenantID string, page pagination.Params) ([]Policy, error)
	ListActivePolicies(ctx context.Context, tenantID string) ([]Policy, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	GetPolicy(ctx context.Context, tenantID, id string) (Policy, error)
	CreatePolicy(ctx context.Context, tenantID string, req PolicyUpsert) (Policy, error)
	UpdatePolicy(ctx context.Context, tenantID, id string, req PolicyUpsert) (Policy, error)
	RetirePolicy(ctx context.Context, tenantID, id string) (Policy, error)
	ListPolicyApps(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]PolicyApp, error)
	ListActivePolicyApps(ctx context.Context, tenantID, policyID string) ([]PolicyApp, error)
	GetPolicyApp(ctx context.Context, tenantID, policyID, appID string) (PolicyApp, error)
	AddPolicyApp(ctx context.Context, tenantID, policyID, appID string) (PolicyApp, error)
	RemovePolicyApp(ctx context.Context, tenantID, policyID, appID string) error
	ListPolicyCertificates(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]PolicyCertificate, error)
	ListActivePolicyCertificates(ctx context.Context, tenantID, policyID string) ([]PolicyCertificate, error)
	GetPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (PolicyCertificate, error)
	AddPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (PolicyCertificate, error)
	RemovePolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) error
	ListPolicyManagedFiles(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]PolicyManagedFile, error)
	ListActivePolicyManagedFiles(ctx context.Context, tenantID, policyID string) ([]PolicyManagedFile, error)
	GetPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (PolicyManagedFile, error)
	AddPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (PolicyManagedFile, error)
	RemovePolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) error
}
