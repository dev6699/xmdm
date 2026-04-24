package telemetry

import "time"

type UploadRequest struct {
	ObservedAt time.Time      `json:"observedAt"`
	Heartbeat  map[string]any `json:"heartbeat,omitempty"`
	Battery    map[string]any `json:"battery,omitempty"`
	Network    map[string]any `json:"network,omitempty"`
	Location   map[string]any `json:"location,omitempty"`
	AppState   map[string]any `json:"appState,omitempty"`
}

type Record struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	DeviceID   string         `json:"deviceId"`
	ObservedAt time.Time      `json:"observedAt"`
	Payload    map[string]any `json:"payload"`
}
