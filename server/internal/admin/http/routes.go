package adminhttp

import (
	"net/http"

	"xmdm/server/internal/admin"
	adminv1 "xmdm/server/internal/api/v1/admin"
	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/plugins"
)

func NewMux(svc *auth.Service, store admin.Repository, auditStore audit.Store, pluginManager *plugins.Manager, tenantID string) http.Handler {
	mux := http.NewServeMux()
	adminMux := httpx.WithPrefix(mux, "/admin")
	adminv1.Register(adminMux, svc, store, auditStore, tenantID)
	if pluginManager != nil {
		pluginManager.Register(adminMux)
	}
	return mux
}
