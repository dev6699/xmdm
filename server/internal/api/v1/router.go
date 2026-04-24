package v1

import (
	"net/http"

	adminhttp "xmdm/server/internal/admin/http"
	apps "xmdm/server/internal/apps"
	apphttp "xmdm/server/internal/apps/http"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/device"
	devicehttp "xmdm/server/internal/device/http"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	"xmdm/server/internal/group"
	grouphttp "xmdm/server/internal/group/http"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	identityhttp "xmdm/server/internal/identity/http"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
	policyhttp "xmdm/server/internal/policy/http"
	"xmdm/server/internal/telemetry"
	telemetryhttp "xmdm/server/internal/telemetry/http"
)

type Dependencies struct {
	Identity      identity.Repository
	Apps          apps.Repository
	Groups        group.Repository
	Policies      policy.Repository
	Devices       device.Repository
	Enrollment    enrollment.Repository
	Telemetry     telemetry.Repository
	Audit         audit.Store
	PluginManager *plugins.Manager
	TenantID      string
}

// NewMux builds the versioned HTTP surface under /api/v1.
func NewMux(svc *auth.Service, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	apiMux := httpx.WithPrefix(mux, "/api/v1")
	enrollmenthttp.Register(apiMux, svc, deps.Enrollment, deps.TenantID)
	telemetryhttp.Register(apiMux, deps.Telemetry, deps.TenantID)
	adminhttp.Register(apiMux, svc, deps.PluginManager)
	apphttp.Register(apiMux, svc, deps.Apps, deps.Audit, deps.TenantID)
	identityhttp.Register(apiMux, svc, deps.Identity, deps.Audit, deps.TenantID)
	grouphttp.Register(apiMux, svc, deps.Groups, deps.Audit, deps.TenantID)
	policyhttp.Register(apiMux, svc, deps.Policies, deps.Audit, deps.TenantID)
	devicehttp.Register(apiMux, svc, deps.Devices, deps.Audit, deps.TenantID)
	return mux
}
