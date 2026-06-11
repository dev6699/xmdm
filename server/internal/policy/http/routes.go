package policyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"
	policy "xmdm/server/internal/policy"
)

func Register(mux httpx.Router, svc *auth.Service, store policy.Repository, auditStore audit.Store, tenantID string) {
	httpx.RegisterCRUDFor(mux, svc, auditStore, tenantID, httpx.ResourceSpec[policy.PolicyUpsert, policy.Policy]{
		Kind:      "policies",
		ReadPerm:  auth.PermissionAdminRead,
		WritePerm: auth.PermissionAdminWrite,
		Decode:    decodePolicyRequest,
		List: func(ctx context.Context, params pagination.Params) ([]policy.Policy, error) {
			return store.ListPolicies(ctx, tenantID, params)
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
	if payload.KioskMode && !kioskExitPasscodeConfigured(payload.Restrictions) {
		return policy.PolicyUpsert{}, httpx.ErrInvalidInput
	}
	return payload, nil
}

func kioskExitPasscodeConfigured(restrictions json.RawMessage) bool {
	if len(restrictions) == 0 || string(restrictions) == "null" {
		return false
	}
	var parsed struct {
		KioskExitPasscode string `json:"kioskExitPasscode,omitempty"`
	}
	if err := json.Unmarshal(restrictions, &parsed); err != nil {
		return false
	}
	return strings.TrimSpace(parsed.KioskExitPasscode) != ""
}
