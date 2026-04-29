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
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Device struct {
	RecordBase
	Name            string         `json:"name"`
	PolicyID        *string        `json:"policyId,omitempty"`
	BootstrapExtras map[string]any `json:"bootstrapExtras,omitempty"`
}

type DeviceUpsert struct {
	Name       string `json:"name"`
	SecretHash string `json:"secretHash"`
	PolicyID   string `json:"policyId"`
}

func (d Device) RecordID() string {
	return d.ID
}

func (d Device) RecordStatus() string {
	return d.Status
}
