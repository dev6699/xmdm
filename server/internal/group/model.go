package group

import "time"

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Group struct {
	RecordBase
	Name string `json:"name"`
}

type GroupUpsert struct {
	Name string `json:"name"`
}

func (g Group) RecordID() string {
	return g.ID
}

func (g Group) RecordStatus() string {
	return g.Status
}
