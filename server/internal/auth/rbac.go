package auth

type Permission string

const (
	PermissionAdminRead    Permission = "admin.read"
	PermissionAdminWrite   Permission = "admin.write"
	PermissionDevicesRead  Permission = "devices.read"
	PermissionDevicesWrite Permission = "devices.write"
)

func AllPermissions() []Permission {
	return []Permission{
		PermissionAdminRead,
		PermissionAdminWrite,
		PermissionDevicesRead,
		PermissionDevicesWrite,
	}
}

func HasPermission(perms []Permission, target Permission) bool {
	for _, perm := range perms {
		if perm == target {
			return true
		}
	}
	return false
}
