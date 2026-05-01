package deviceinfo

import "time"

type UploadRequest struct {
	ObservedAt time.Time      `json:"observedAt,omitempty"`
	Payload    map[string]any `json:"payload"`
}

type Record struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	DeviceID   string         `json:"deviceId"`
	ObservedAt time.Time      `json:"observedAt"`
	Payload    map[string]any `json:"payload"`
}

type SearchFilter struct {
	DeviceID string
	Query    string
	Since    *time.Time
	Until    *time.Time
	Limit    int
}
