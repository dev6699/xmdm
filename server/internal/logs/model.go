package logs

import (
	"time"

	"xmdm/server/internal/pagination"
)

type EntryUpsert struct {
	ObservedAt time.Time      `json:"observedAt,omitempty"`
	Source     string         `json:"source,omitempty"`
	Level      string         `json:"level,omitempty"`
	Message    string         `json:"message,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type UploadRequest struct {
	ObservedAt time.Time     `json:"observedAt,omitempty"`
	Entries    []EntryUpsert `json:"entries"`
}

type Record struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	DeviceID   string         `json:"deviceId"`
	ObservedAt time.Time      `json:"observedAt"`
	Source     string         `json:"source,omitempty"`
	Level      string         `json:"level,omitempty"`
	Message    string         `json:"message,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type SearchFilter struct {
	DeviceID string
	Source   string
	Level    string
	Query    string
	Since    *time.Time
	Until    *time.Time
	Limit    int
	Offset   int
	Pagination pagination.Params
}
