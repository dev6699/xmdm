package policy

import "context"

type Repository interface {
	ListPolicies(ctx context.Context, tenantID string) ([]Policy, error)
	CreatePolicy(ctx context.Context, tenantID string, req PolicyUpsert) (Policy, error)
	UpdatePolicy(ctx context.Context, tenantID, id string, req PolicyUpsert) (Policy, error)
	RetirePolicy(ctx context.Context, tenantID, id string) (Policy, error)
}
