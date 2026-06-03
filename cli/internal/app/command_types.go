package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/session"
)

var builtinCommandTypes = map[string]struct{}{
	"ping":                 {},
	"reboot":               {},
	"sync_config":          {},
	"exit_kiosk":           {},
	"launch_companion_app": {},
}

type pluginCatalogResponse struct {
	Plugins []pluginCatalogPlugin `json:"plugins"`
}

type pluginCatalogPlugin struct {
	Enabled      bool                `json:"enabled"`
	CommandTypes []pluginCatalogType `json:"commandTypes"`
}

type pluginCatalogType struct {
	Type               string `json:"type"`
	RequiredPermission string `json:"requiredPermission,omitempty"`
}

func isBuiltinCommandType(commandType string) bool {
	_, ok := builtinCommandTypes[strings.TrimSpace(commandType)]
	return ok
}

func (a *app) validateCommandTypeBody(ctx context.Context, resolved config.Resolved, state session.State, body []byte) error {
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	commandType := strings.TrimSpace(payload.Type)
	if commandType == "" {
		return fmt.Errorf("command type is required")
	}
	if isBuiltinCommandType(commandType) {
		return nil
	}
	allowed, err := a.allowedPluginCommandTypes(ctx, resolved, state)
	if err != nil {
		if !strings.Contains(err.Error(), "Not Found") && !strings.Contains(err.Error(), "404") {
			return err
		}
		allowed = map[string]struct{}{}
	}
	if _, ok := allowed[commandType]; !ok {
		return fmt.Errorf("unsupported command type")
	}
	return nil
}

func (a *app) allowedPluginCommandTypes(ctx context.Context, resolved config.Resolved, state session.State) (map[string]struct{}, error) {
	items, err := a.fetchResourceItems(ctx, resolved, resourceSpec{Name: "plugins", Path: "/admin/plugins", ListField: "plugins"}, state)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{})
	for _, raw := range items {
		var plugin pluginCatalogPlugin
		if err := json.Unmarshal(raw, &plugin); err != nil {
			return nil, err
		}
		if !plugin.Enabled {
			continue
		}
		for _, commandType := range plugin.CommandTypes {
			commandType.Type = strings.TrimSpace(commandType.Type)
			if commandType.Type == "" {
				continue
			}
			allowed[commandType.Type] = struct{}{}
		}
	}
	return allowed, nil
}
