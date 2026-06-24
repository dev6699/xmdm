package bootstrap

import (
	"sync"

	"xmdm/server/internal/auth"
)

const DefaultPostgresDSN = "postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable"

const DefaultAdminUsername = "admin"
const DefaultAdminPassword = "admin"

const SeedTenantID = "11111111-1111-1111-1111-111111111111"
const SeedTenantName = "Default tenant"

const SeedAdminRoleID = "22222222-2222-2222-2222-222222222222"
const SeedAdminRoleName = "admins"

const SeedAgentAppPackage = "com.xmdm.launcher"
const SeedAgentAppName = "XMDM Agent"

var SeedAdminPermissions = permissionsToStrings(auth.AllPermissions())

type AppSeed struct {
	PackageName string
	Name        string
}

var (
	appSeedMu   sync.Mutex
	appSeedList []AppSeed
)

func RegisterAppSeed(seed AppSeed) {
	if seed.PackageName == "" || seed.Name == "" {
		return
	}
	appSeedMu.Lock()
	defer appSeedMu.Unlock()
	for i := range appSeedList {
		if appSeedList[i].PackageName == seed.PackageName {
			appSeedList[i] = seed
			return
		}
	}
	appSeedList = append(appSeedList, seed)
}

func AppSeeds() []AppSeed {
	appSeedMu.Lock()
	defer appSeedMu.Unlock()
	return append([]AppSeed(nil), appSeedList...)
}

func permissionsToStrings(perms []auth.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, string(perm))
	}
	return out
}
