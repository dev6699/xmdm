package host

import (
	"context"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	coremigrate "xmdm/server"
	internalv1 "xmdm/server/internal/api/v1"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	"xmdm/server/internal/config"
	"xmdm/server/internal/pagination"
	internalplugins "xmdm/server/internal/plugins"
	"xmdm/server/internal/roles"
)

type RouteSpec = internalplugins.RouteSpec
type DeviceAction = internalplugins.DeviceAction
type CommandType = internalplugins.CommandType
type Plugin = internalplugins.Plugin
type Config = config.Config

type HostedPlugin interface {
	Mount(*http.ServeMux)
	CorePlugin() Plugin
}

type DatabaseAware interface {
	SetDatabase(*pgxpool.Pool) error
}

type Migratable interface {
	Migrate() error
}

func Run(cfg *config.Config, plugins ...HostedPlugin) error {
	if cfg == nil {
		return &configError{message: "config is nil"}
	}

	if err := coremigrate.MigrateDSN(cfg.Postgres.DSN); err != nil {
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
			if dbAware, ok := plugin.(DatabaseAware); ok && deps.Database != nil {
				if err := dbAware.SetDatabase(deps.Database); err != nil {
					return err
				}
			}
			if migratable, ok := plugin.(Migratable); ok {
				if err := migratable.Migrate(); err != nil {
					return err
				}
			}
			defs = append(defs, plugin.CorePlugin())
			mounts = append(mounts, plugin.Mount)
		}
		if len(defs) > 0 {
			deps.PluginManager = internalplugins.New(defs...)
			if err := syncSeedRolePermissions(context.Background(), deps.Roles, deps.TenantID, deps.PluginManager.PermissionCatalog()); err != nil {
				return err
			}
		}
		deps.ExtraRootMounts = append(deps.ExtraRootMounts, mounts...)
	}

	svc := auth.NewServiceWithPermissions(cfg.Admin.Username, cfg.Admin.Password, cfg.Server.SessionTTL, mergedAuthPermissions(deps.PluginManager))
	handler := internalv1.NewMux(svc, deps)
	log.Printf("xmdm server listening on %s", cfg.Server.Address)
	return http.ListenAndServe(cfg.Server.Address, handler)
}

// LoadConfig loads the core server configuration using the same defaults and
// environment overrides as the main server entrypoint.
func LoadConfig(configPath string) (*config.Config, error) {
	return config.LoadConfig(configPath)
}

type configError struct {
	message string
}

func (e *configError) Error() string {
	return e.message
}

func mergedAuthPermissions(pluginManager *internalplugins.Manager) []auth.Permission {
	seen := make(map[auth.Permission]struct{})
	perms := make([]auth.Permission, 0)
	for _, perm := range auth.AllPermissions() {
		if _, ok := seen[perm]; ok {
			continue
		}
		seen[perm] = struct{}{}
		perms = append(perms, perm)
	}
	if pluginManager == nil {
		return perms
	}
	for _, perm := range pluginManager.PermissionCatalog() {
		if _, ok := seen[perm]; ok {
			continue
		}
		seen[perm] = struct{}{}
		perms = append(perms, perm)
	}
	return perms
}

func syncSeedRolePermissions(ctx context.Context, repo roles.Repository, tenantID string, catalog []auth.Permission) error {
	if repo == nil || len(catalog) == 0 {
		return nil
	}
	items, err := repo.ListRoles(ctx, tenantID, pagination.Params{Limit: pagination.DefaultLimit})
	if err != nil {
		return err
	}
	var seed *roles.Role
	for i := range items {
		if items[i].ID == bootstrap.SeedAdminRoleID {
			seed = &items[i]
			break
		}
	}
	if seed == nil || seed.Status != "active" {
		return nil
	}
	merged := mergePermissions(seed.Permissions, catalog)
	if slices.Equal(merged, seed.Permissions) {
		return nil
	}
	_, err = repo.UpdateRole(ctx, tenantID, seed.ID, roles.RoleUpsert{Name: seed.Name, Permissions: merged})
	return err
}

func mergePermissions(existing []string, catalog []auth.Permission) []string {
	seen := make(map[string]struct{}, len(existing))
	merged := make([]string, 0, len(existing)+len(catalog))
	for _, perm := range existing {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			continue
		}
		if _, ok := seen[perm]; ok {
			continue
		}
		seen[perm] = struct{}{}
		merged = append(merged, perm)
	}
	for _, perm := range catalog {
		value := strings.TrimSpace(string(perm))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}
