package v1

import (
	"net/http"

	"xmdm/server/internal/admin"
	adminhttp "xmdm/server/internal/admin/http"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	devicehttp "xmdm/server/internal/device/http"
	enrollmenthttp "xmdm/server/internal/enrollment/http"
	grouphttp "xmdm/server/internal/group/http"
	"xmdm/server/internal/httpx"
	identityhttp "xmdm/server/internal/identity/http"
	"xmdm/server/internal/plugins"
	policyhttp "xmdm/server/internal/policy/http"
)

// NewMux builds the versioned HTTP surface under /api/v1.
func NewMux(svc *auth.Service, store admin.Repository, auditStore audit.Store, pluginManager *plugins.Manager, tenantID string) http.Handler {
	mux := http.NewServeMux()
	apiMux := httpx.WithPrefix(mux, "/api/v1")
	enrollmentMux := httpx.WithPrefix(apiMux, "/enrollment")
	enrollmenthttp.Register(enrollmentMux, svc)
	adminMux := httpx.WithPrefix(apiMux, "/admin")
	adminhttp.Register(adminMux, svc)
	identityhttp.Register(apiMux, svc, store, auditStore, tenantID)
	grouphttp.Register(apiMux, svc, store, auditStore, tenantID)
	policyhttp.Register(apiMux, svc, store, auditStore, tenantID)
	devicehttp.Register(apiMux, svc, store, auditStore, tenantID)
	if pluginManager != nil {
		pluginManager.Register(adminMux)
	}
	return mux
}
