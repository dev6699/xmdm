package policyhttp

import (
	"context"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	policy "xmdm/server/internal/policy"
)

func Register(mux httpx.Router, svc *auth.Service, store policy.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[policy.PolicyUpsert, policy.Policy]{
		Kind:      "policies",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodePolicyRequest,
		List: func(ctx context.Context) ([]policy.Policy, error) {
			return store.ListPolicies(ctx, tenantID)
		},
		Create: func(ctx context.Context, req policy.PolicyUpsert) (policy.Policy, error) {
			return store.CreatePolicy(ctx, tenantID, req)
		},
		Update: func(ctx context.Context, id string, req policy.PolicyUpsert) (policy.Policy, error) {
			return store.UpdatePolicy(ctx, tenantID, id, req)
		},
		Retire: func(ctx context.Context, id string) (policy.Policy, error) {
			return store.RetirePolicy(ctx, tenantID, id)
		},
		Audit: func(rec policy.Policy) map[string]any {
			return map[string]any{"name": rec.Name}
		},
	})
}

func decodePolicyRequest(r *http.Request) (policy.PolicyUpsert, error) {
	var payload policy.PolicyUpsert
	if err := httpx.DecodeJSONBody(r, &payload); err != nil {
		return policy.PolicyUpsert{}, err
	}
	if payload.Name == "" {
		return policy.PolicyUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}
