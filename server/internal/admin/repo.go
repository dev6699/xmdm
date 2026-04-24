package admin

import (
	device "xmdm/server/internal/device"
	group "xmdm/server/internal/group"
	"xmdm/server/internal/identity"
	policy "xmdm/server/internal/policy"
)

type Repository interface {
	identity.Repository
	group.Repository
	policy.Repository
	device.Repository

	// Admin-specific methods can be added here if the console grows beyond
	// the core managed objects.
}

type IdentityRepository struct {
	identity.Repository
}

type GroupsRepository struct {
	group.Repository
}

type PoliciesRepository struct {
	policy.Repository
}

type DevicesRepository struct {
	device.Repository
}

type CompositeRepository struct {
	IdentityRepository
	GroupsRepository
	PoliciesRepository
	DevicesRepository
}

func NewRepository(identityRepo identity.Repository, groupsRepo group.Repository, policiesRepo policy.Repository, devicesRepo device.Repository) Repository {
	return CompositeRepository{
		IdentityRepository: IdentityRepository{Repository: identityRepo},
		GroupsRepository:   GroupsRepository{Repository: groupsRepo},
		PoliciesRepository: PoliciesRepository{Repository: policiesRepo},
		DevicesRepository:  DevicesRepository{Repository: devicesRepo},
	}
}
