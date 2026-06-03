package host

import (
	"net/http"
	"time"

	internalv1 "xmdm/server/internal/api/v1"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/config"
	internalplugins "xmdm/server/internal/plugins"
)

type Config = config.Config
type ServerConfig = config.ServerConfig
type PostgresConfig = config.PostgresConfig
type MQTTConfig = config.MQTTConfig
type DeviceConfig = config.DeviceConfig
type ObjectStoreConfig = config.ObjectStoreConfig
type AdminConfig = config.AdminConfig

type Permission = auth.Permission
type Session = auth.Session
type Service = auth.Service

type Dependencies = internalv1.Dependencies
type RouteSpec = internalplugins.RouteSpec
type DeviceAction = internalplugins.DeviceAction
type CommandType = internalplugins.CommandType
type Plugin = internalplugins.Plugin
type Manager = internalplugins.Manager

const SessionCookieName = auth.SessionCookieName

const (
	PermissionAdminRead    = auth.PermissionAdminRead
	PermissionAdminWrite   = auth.PermissionAdminWrite
	PermissionDevicesRead  = auth.PermissionDevicesRead
	PermissionDevicesWrite = auth.PermissionDevicesWrite
)

var ErrInvalidCredentials = auth.ErrInvalidCredentials

func LoadConfig(path string) (*Config, error) { return config.LoadConfig(path) }

func AllPermissions() []Permission { return auth.AllPermissions() }

func HasPermission(perms []Permission, target Permission) bool {
	return auth.HasPermission(perms, target)
}

func NewService(username, password string, sessionTTL time.Duration) *Service {
	return auth.NewService(username, password, sessionTTL)
}

func NewServiceWithPermissions(username, password string, sessionTTL time.Duration, permissions []Permission) *Service {
	return auth.NewServiceWithPermissions(username, password, sessionTTL, permissions)
}

func NewDeps(cfg *Config) Dependencies { return internalv1.NewDeps(cfg) }

func NewMux(svc *Service, deps Dependencies) http.Handler { return internalv1.NewMux(svc, deps) }

func Disabled() *Manager              { return internalplugins.Disabled() }
func New(defs ...Plugin) *Manager     { return internalplugins.New(defs...) }
func Enabled(defs ...Plugin) *Manager { return internalplugins.Enabled(defs...) }

func Mount(mux *http.ServeMux, prefix string, handler http.Handler) {
	if mux == nil || handler == nil {
		return
	}
	mux.Handle(prefix+"/", http.StripPrefix(prefix, handler))
	mux.Handle(prefix, http.StripPrefix(prefix, handler))
}
