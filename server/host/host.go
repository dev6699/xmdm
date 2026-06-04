package host

import (
	"log"
	"net/http"

	internalv1 "xmdm/server/internal/api/v1"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/config"
	internalplugins "xmdm/server/internal/plugins"
)

type RouteSpec = internalplugins.RouteSpec
type DeviceAction = internalplugins.DeviceAction
type CommandType = internalplugins.CommandType
type Plugin = internalplugins.Plugin

type HostedPlugin interface {
	Mount(*http.ServeMux)
	CorePlugin() Plugin
}

func Run(configPath string, plugins ...HostedPlugin) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}

	deps := internalv1.NewDeps(cfg)
	if len(plugins) > 0 {
		defs := make([]internalplugins.Plugin, 0, len(plugins))
		mounts := make([]func(*http.ServeMux), 0, len(plugins))
		for _, plugin := range plugins {
			if plugin == nil {
				continue
			}
			defs = append(defs, plugin.CorePlugin())
			mounts = append(mounts, plugin.Mount)
		}
		if len(defs) > 0 {
			deps.PluginManager = internalplugins.New(defs...)
		}
		deps.ExtraRootMounts = append(deps.ExtraRootMounts, mounts...)
	}

	svc := auth.NewService(cfg.Admin.Username, cfg.Admin.Password, cfg.Server.SessionTTL)
	handler := internalv1.NewMux(svc, deps)
	log.Printf("xmdm server listening on %s", cfg.Server.Address)
	return http.ListenAndServe(cfg.Server.Address, handler)
}
