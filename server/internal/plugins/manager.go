package plugins

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"xmdm/server/internal/auth"
	"xmdm/server/internal/httpx"
)

type RouteSpec struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type DeviceAction struct {
	PluginID           string `json:"pluginId"`
	ActionID           string `json:"actionId"`
	Label              string `json:"label"`
	Href               string `json:"href"`
	RequiredPermission string `json:"requiredPermission,omitempty"`
	Enabled            bool   `json:"enabled"`
	DisabledReason     string `json:"disabledReason,omitempty"`
}

type CommandType struct {
	Type               string `json:"type"`
	Label              string `json:"label"`
	TargetScope        string `json:"targetScope,omitempty"`
	PayloadSchema      string `json:"payloadSchema,omitempty"`
	RequiredPermission string `json:"requiredPermission,omitempty"`
}

type Plugin struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled"`
	Permissions   []string       `json:"permissions,omitempty"`
	Routes        []RouteSpec    `json:"routes,omitempty"`
	DeviceActions []DeviceAction `json:"deviceActions,omitempty"`
	CommandTypes  []CommandType  `json:"commandTypes,omitempty"`
}

type Manager struct {
	enabled bool
	plugins []Plugin
}

type catalogResponse struct {
	Plugins []Plugin `json:"plugins"`
}

func Disabled() *Manager {
	return &Manager{}
}

func New(defs ...Plugin) *Manager {
	m := &Manager{enabled: true}
	m.plugins = normalizePlugins(defs)
	return m
}

func Enabled(defs ...Plugin) *Manager {
	return New(defs...)
}

func (m *Manager) Register(mux httpx.Router, svc *auth.Service) {
	if m == nil || !m.enabled {
		return
	}
	mux.HandleFunc("/plugins", func(w http.ResponseWriter, r *http.Request) {
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasPermission(session.Permissions, auth.PermissionAdminRead) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeCatalog(w, m.plugins)
	})
}

func (m *Manager) DeviceActionsFor(session *auth.Session, deviceID string) []DeviceAction {
	if m == nil || !m.enabled || session == nil {
		return nil
	}
	actions := make([]DeviceAction, 0)
	for _, plugin := range m.plugins {
		if !plugin.Enabled {
			continue
		}
		for _, action := range plugin.DeviceActions {
			if !action.Enabled {
				continue
			}
			if perm := strings.TrimSpace(action.RequiredPermission); perm != "" && !auth.HasPermission(session.Permissions, auth.Permission(perm)) {
				continue
			}
			if strings.TrimSpace(action.Href) == "" {
				continue
			}
			action.PluginID = plugin.ID
			action.Href = resolveDeviceActionHref(action.Href, deviceID)
			action.Enabled = true
			actions = append(actions, action)
		}
	}
	return actions
}

func (m *Manager) CommandTypesFor(session *auth.Session) []CommandType {
	if m == nil || !m.enabled || session == nil {
		return nil
	}
	types := make([]CommandType, 0)
	for _, plugin := range m.plugins {
		if !plugin.Enabled {
			continue
		}
		for _, commandType := range plugin.CommandTypes {
			if strings.TrimSpace(commandType.Type) == "" {
				continue
			}
			if perm := strings.TrimSpace(commandType.RequiredPermission); perm != "" && !auth.HasPermission(session.Permissions, auth.Permission(perm)) {
				continue
			}
			commandType.Type = strings.TrimSpace(commandType.Type)
			if strings.TrimSpace(commandType.Label) == "" {
				commandType.Label = commandType.Type
			}
			types = append(types, commandType)
		}
	}
	return types
}

func (m *Manager) PermissionCatalog() []auth.Permission {
	if m == nil {
		return append([]auth.Permission(nil), auth.AllPermissions()...)
	}
	return permissionCatalog(m.plugins)
}

func (m *Manager) SupportsCommandType(session *auth.Session, commandType string) (CommandType, bool) {
	commandType = strings.TrimSpace(commandType)
	if commandType == "" || m == nil || !m.enabled || session == nil {
		return CommandType{}, false
	}
	for _, spec := range m.CommandTypesFor(session) {
		if spec.Type == commandType {
			return spec, true
		}
	}
	return CommandType{}, false
}

func normalizePlugins(defs []Plugin) []Plugin {
	if len(defs) == 0 {
		return nil
	}
	pluginsByID := make(map[string]Plugin, len(defs))
	for _, def := range defs {
		id := strings.TrimSpace(def.ID)
		if id == "" {
			continue
		}
		def.ID = id
		pluginsByID[id] = clonePlugin(def)
	}
	ids := make([]string, 0, len(pluginsByID))
	for id := range pluginsByID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	plugins := make([]Plugin, 0, len(ids))
	for _, id := range ids {
		plugins = append(plugins, pluginsByID[id])
	}
	return plugins
}

func permissionCatalog(plugins []Plugin) []auth.Permission {
	catalog := make(map[auth.Permission]struct{})
	for _, perm := range auth.AllPermissions() {
		catalog[perm] = struct{}{}
	}
	for _, plugin := range plugins {
		for _, perm := range plugin.Permissions {
			perm = strings.TrimSpace(perm)
			if perm != "" {
				catalog[auth.Permission(perm)] = struct{}{}
			}
		}
		for _, action := range plugin.DeviceActions {
			if perm := strings.TrimSpace(action.RequiredPermission); perm != "" {
				catalog[auth.Permission(perm)] = struct{}{}
			}
		}
		for _, commandType := range plugin.CommandTypes {
			if perm := strings.TrimSpace(commandType.RequiredPermission); perm != "" {
				catalog[auth.Permission(perm)] = struct{}{}
			}
		}
	}
	perms := make([]auth.Permission, 0, len(catalog))
	for perm := range catalog {
		perms = append(perms, perm)
	}
	slices.SortFunc(perms, func(a, b auth.Permission) int {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		default:
			return 0
		}
	})
	return perms
}

func clonePlugin(def Plugin) Plugin {
	def.Permissions = append([]string(nil), def.Permissions...)
	for i := range def.DeviceActions {
		if strings.TrimSpace(def.DeviceActions[i].PluginID) == "" {
			def.DeviceActions[i].PluginID = def.ID
		}
	}
	def.Routes = append([]RouteSpec(nil), def.Routes...)
	def.DeviceActions = append([]DeviceAction(nil), def.DeviceActions...)
	def.CommandTypes = append([]CommandType(nil), def.CommandTypes...)
	return def
}

func resolveDeviceActionHref(href, deviceID string) string {
	if strings.TrimSpace(href) == "" {
		return ""
	}
	escaped := url.PathEscape(deviceID)
	href = strings.ReplaceAll(href, "{{deviceId}}", escaped)
	href = strings.ReplaceAll(href, "{deviceId}", escaped)
	return href
}

func sessionFromRequest(r *http.Request, svc *auth.Service) (*auth.Session, bool) {
	if svc == nil {
		return nil, false
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, false
	}
	session, ok := svc.Authenticate(cookie.Value)
	if !ok {
		return nil, false
	}
	return session, true
}

func writeCatalog(w http.ResponseWriter, plugins []Plugin) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(catalogResponse{Plugins: plugins})
}
