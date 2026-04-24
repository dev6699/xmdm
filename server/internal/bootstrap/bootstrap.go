package bootstrap

const DefaultPostgresDSN = "postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable"

const DefaultAdminUsername = "admin"
const DefaultAdminPassword = "admin"

const SeedTenantID = "11111111-1111-1111-1111-111111111111"
const SeedTenantName = "Default tenant"

const SeedAdminRoleID = "22222222-2222-2222-2222-222222222222"
const SeedAdminRoleName = "admins"

var SeedAdminPermissions = []string{"admin.read", "admin.write", "devices.read", "devices.write"}
