package audit

import "time"

type Event struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	CreatedAt    time.Time      `json:"createdAt"`
	Details      map[string]any `json:"details,omitempty"`
}
