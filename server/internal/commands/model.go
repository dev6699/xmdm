package commands

import "time"

const (
	StatusQueued    = "queued"
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusAcked     = "acked"
	StatusFailed    = "failed"
	StatusExpired   = "expired"
)

const (
	TargetDevice    = "device"
	TargetGroup     = "group"
	TargetBroadcast = "broadcast"
)

type Command struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Status    string         `json:"status"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty"`

	TenantID  string    `json:"-"`
	DeviceID  string    `json:"-"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type Target struct {
	Type     string `json:"type"`
	DeviceID string `json:"deviceId,omitempty"`
	GroupID  string `json:"groupId,omitempty"`
}

type Upsert struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty"`
	Target    Target         `json:"target"`
}
