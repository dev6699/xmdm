package device

import "time"

const (
	StatusPending   = "pending"
	StatusEnrolled  = "enrolled"
	StatusActive    = "active"
	StatusLocked    = "locked"
	StatusSuspended = "suspended"
	StatusRetired   = "retired"
	StatusWiped     = "wiped"
)

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Device struct {
	RecordBase
	Name            string         `json:"name"`
	PolicyID        *string        `json:"policyId,omitempty"`
	GroupIDs        []string       `json:"groupIds,omitempty"`
	BootstrapExtras map[string]any `json:"bootstrapExtras,omitempty"`
}

type DeviceUpsert struct {
	Name       string   `json:"name"`
	SecretHash string   `json:"secretHash"`
	PolicyID   string   `json:"policyId"`
	GroupIDs   []string `json:"groupIds,omitempty"`
}

type OverviewStats struct {
	Total          int
	Active         int
	Pending        int
	RetiredOrWiped int
	AssignedPolicy int
}

type StatusCounts struct {
	Pending   int
	Enrolled  int
	Active    int
	Locked    int
	Suspended int
	Retired   int
	Wiped     int
}

func (d Device) RecordID() string {
	return d.ID
}

func (d Device) RecordStatus() string {
	return d.Status
}
