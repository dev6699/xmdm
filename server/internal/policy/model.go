package policy

import (
	"encoding/json"
	"time"
)

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Policy struct {
	RecordBase
	Name         string          `json:"name"`
	Version      int             `json:"version"`
	KioskMode    bool            `json:"kioskMode"`
	Restrictions json.RawMessage `json:"restrictions"`
}

type PolicyUpsert struct {
	Name         string          `json:"name"`
	Version      int             `json:"version"`
	KioskMode    bool            `json:"kioskMode"`
	Restrictions json.RawMessage `json:"restrictions"`
}

func (p Policy) RecordID() string {
	return p.ID
}

func (p Policy) RecordStatus() string {
	return p.Status
}
