package v1

import (
	"net/http"

	adminhttp "xmdm/server/internal/admin/http"
	apps "xmdm/server/internal/apps"
	apphttp "xmdm/server/internal/apps/http"
	"xmdm/server/internal/artifacts"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/certificates"
	certificatehttp "xmdm/server/internal/certificates/http"
	"xmdm/server/internal/commands"
	commandhttp "xmdm/server/internal/commands/http"
	"xmdm/server/internal/device"
	devicehttp "xmdm/server/internal/device/http"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	files "xmdm/server/internal/files"
	filehttp "xmdm/server/internal/files/http"
	"xmdm/server/internal/group"
	grouphttp "xmdm/server/internal/group/http"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/identity"
	identityhttp "xmdm/server/internal/identity/http"
	managedfiles "xmdm/server/internal/managedfiles"
	managedfilehttp "xmdm/server/internal/managedfiles/http"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
	policyhttp "xmdm/server/internal/policy/http"
	"xmdm/server/internal/push"
	"xmdm/server/internal/telemetry"
	telemetryhttp "xmdm/server/internal/telemetry/http"
)

type Dependencies struct {
	Identity      identity.Repository
	Apps          apps.Repository
	Files         files.Repository
	ManagedFiles  managedfiles.Repository
	Commands      commands.Repository
	Certificates  certificates.Repository
	Artifacts     artifacts.Store
	Groups        group.Repository
	Policies      policy.Repository
	Devices       device.Repository
	Enrollment    enrollment.Repository
	Telemetry     telemetry.Repository
	Audit         audit.Store
	Push          push.Publisher
	PluginManager *plugins.Manager
	TenantID      string
}

// NewMux builds the versioned HTTP surface under /api/v1.
func NewMux(svc *auth.Service, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	apiMux := httpx.WithPrefix(mux, "/api/v1")
	enrollmenthttp.Register(apiMux, svc, deps.Enrollment, deps.Apps, deps.ManagedFiles, deps.Certificates, deps.TenantID)
	telemetryhttp.Register(apiMux, deps.Telemetry, deps.TenantID)
	adminhttp.Register(apiMux, svc, deps.PluginManager, deps.Audit, deps.Commands, deps.TenantID)
	commandhttp.Register(apiMux, deps.Devices, deps.Commands, deps.TenantID)
	apphttp.Register(apiMux, svc, deps.Apps, deps.Devices, deps.Artifacts, deps.Audit, deps.TenantID)
	filehttp.Register(apiMux, svc, deps.Files, deps.Artifacts, deps.Audit, deps.TenantID)
	managedfilehttp.Register(apiMux, svc, deps.ManagedFiles, deps.Devices, deps.Artifacts, deps.TenantID)
	certificatehttp.Register(apiMux, svc, deps.Certificates, deps.Artifacts, deps.Audit, deps.TenantID)
	identityhttp.Register(apiMux, svc, deps.Identity, deps.Audit, deps.TenantID)
	grouphttp.Register(apiMux, svc, deps.Groups, deps.Audit, deps.TenantID)
	policyhttp.Register(apiMux, svc, deps.Policies, deps.Audit, deps.TenantID)
	devicehttp.Register(apiMux, svc, deps.Devices, deps.Audit, deps.TenantID)
	return mux
}
