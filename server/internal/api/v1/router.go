package v1

import (
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	deviceinfo "xmdm/server/internal/deviceinfo"
	deviceinfohttp "xmdm/server/internal/deviceinfo/http"
	"xmdm/server/internal/enrollment"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	files "xmdm/server/internal/files"
	filehttp "xmdm/server/internal/files/http"
	"xmdm/server/internal/group"
	"xmdm/server/internal/httpx"
	logs "xmdm/server/internal/logs"
	loghttp "xmdm/server/internal/logs/http"
	managedfiles "xmdm/server/internal/managedfiles"
	managedfilehttp "xmdm/server/internal/managedfiles/http"
	"xmdm/server/internal/observability"
	"xmdm/server/internal/plugins"
	"xmdm/server/internal/policy"
	"xmdm/server/internal/push"
	"xmdm/server/internal/roles"
	"xmdm/server/internal/telemetry"
	telemetryhttp "xmdm/server/internal/telemetry/http"
	"xmdm/server/internal/users"
)

type Dependencies struct {
	Database           *pgxpool.Pool
	Users              users.Repository
	Roles              roles.Repository
	Apps               apps.Repository
	Files              files.Repository
	ManagedFiles       managedfiles.Repository
	Logs               logs.Repository
	Commands           commands.Repository
	DeviceInfo         deviceinfo.Repository
	Certificates       certificates.Repository
	Artifacts          artifacts.Store
	Groups             group.Repository
	Policies           policy.Repository
	Devices            device.Repository
	Enrollment         enrollment.Repository
	Telemetry          telemetry.Repository
	Audit              audit.Store
	Push               push.Publisher
	PushHealth         push.HealthChecker
	Runtime            enrollment.RuntimeSnapshot
	DisableRequestLogs bool
	ServerPublicURL    string
	PluginManager      *plugins.Manager
	ExtraRootMounts    []func(*http.ServeMux)
	TenantID           string
}

// NewMux builds the versioned HTTP surface under /api/v1.
func NewMux(svc *auth.Service, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	apiMux := httpx.WithPrefix(mux, "/api/v1")
	adminhttp.RegisterDashboard(mux, svc, adminhttp.DashboardDependencies{
		Database:        deps.Database,
		Users:           deps.Users,
		Roles:           deps.Roles,
		Apps:            deps.Apps,
		Files:           deps.Files,
		ManagedFiles:    deps.ManagedFiles,
		Logs:            deps.Logs,
		Commands:        deps.Commands,
		DeviceInfo:      deps.DeviceInfo,
		Certificates:    deps.Certificates,
		Artifacts:       deps.Artifacts,
		Groups:          deps.Groups,
		Policies:        deps.Policies,
		Devices:         deps.Devices,
		Enrollment:      deps.Enrollment,
		Runtime:         deps.Runtime,
		ServerPublicURL: deps.ServerPublicURL,
		PluginManager:   deps.PluginManager,
		Audit:           deps.Audit,
		TenantID:        deps.TenantID,
		PushHealth:      deps.PushHealth,
	})
	enrollmenthttp.Register(apiMux, svc, deps.Devices, deps.Enrollment, deps.Apps, deps.ManagedFiles, deps.Artifacts, deps.Certificates, deps.Policies, deps.Runtime, deps.TenantID)
	telemetryhttp.Register(apiMux, deps.Telemetry, deps.TenantID)
	loghttp.Register(apiMux, svc, deps.Devices, deps.Logs, deps.TenantID)
	deviceinfohttp.Register(apiMux, svc, deps.Devices, deps.DeviceInfo, deps.TenantID)
	commandhttp.Register(apiMux, deps.Devices, deps.Commands, deps.TenantID)
	apphttp.Register(apiMux, svc, deps.Apps, deps.Devices, deps.Artifacts, deps.Audit, deps.TenantID)
	filehttp.Register(apiMux, svc, deps.Files, deps.Artifacts, deps.Audit, deps.TenantID)
	managedfilehttp.Register(apiMux, svc, deps.ManagedFiles, deps.Devices, deps.Artifacts, deps.TenantID)
	certificatehttp.Register(apiMux, svc, deps.Devices, deps.Certificates, deps.Artifacts, deps.Audit, deps.TenantID)
	for _, mount := range deps.ExtraRootMounts {
		if mount != nil {
			mount(mux)
		}
	}
	return observability.NewHandler(httpx.WithRateLimits(mux, defaultRateLimitRules()...), observability.Config{
		Logger:            log.Default(),
		DisableRequestLog: deps.DisableRequestLogs,
	})
}

func defaultRateLimitRules() []httpx.RateLimitRule {
	return []httpx.RateLimitRule{
		{
			Name:           "admin-login",
			Method:         http.MethodPost,
			Prefix:         "/admin/login",
			Burst:          100,
			RefillInterval: 10 * time.Second,
			RetryAfter:     10 * time.Second,
		},
		{
			Name:           "admin-command-create",
			Method:         http.MethodPost,
			Prefix:         "/admin/commands/create",
			Burst:          100,
			RefillInterval: 10 * time.Second,
			RetryAfter:     10 * time.Second,
		},
		{
			Name:           "enrollment",
			Method:         http.MethodPost,
			Prefix:         "/api/v1/enrollment",
			Burst:          20,
			RefillInterval: 2 * time.Second,
			RetryAfter:     2 * time.Second,
		},
	}
}
