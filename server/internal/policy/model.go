package policy

import (
	"encoding/json"
	"time"
)

const (
	StatusActive  = "active"
	StatusRetired = "retired"
)

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Policy struct {
	RecordBase
	Name            string          `json:"name"`
	Version         int             `json:"version"`
	KioskMode       bool            `json:"kioskMode"`
	KioskAppPackage string          `json:"kioskAppPackage,omitempty"`
	Restrictions    json.RawMessage `json:"restrictions"`
}

type PolicyUpsert struct {
	Name            string          `json:"name"`
	Version         int             `json:"version"`
	KioskMode       bool            `json:"kioskMode"`
	KioskAppPackage string          `json:"kioskAppPackage,omitempty"`
	Restrictions    json.RawMessage `json:"restrictions"`
}

type PolicyApp struct {
	RecordBase
	PolicyID string `json:"policyId"`
	AppID    string `json:"appId"`
}

type PolicyCertificate struct {
	RecordBase
	PolicyID      string `json:"policyId"`
	CertificateID string `json:"certificateId"`
}

type PolicyManagedFile struct {
	RecordBase
	PolicyID      string `json:"policyId"`
	ManagedFileID string `json:"managedFileId"`
}

type OverviewStats struct {
	Total   int
	Active  int
	Retired int
}

func (p Policy) RecordID() string {
	return p.ID
}

func (p Policy) RecordStatus() string {
	return p.Status
}

func (p PolicyApp) RecordID() string {
	return p.ID
}

func (p PolicyApp) RecordStatus() string {
	return p.Status
}

func (p PolicyCertificate) RecordID() string {
	return p.ID
}

func (p PolicyCertificate) RecordStatus() string {
	return p.Status
}

func (p PolicyManagedFile) RecordID() string {
	return p.ID
}

func (p PolicyManagedFile) RecordStatus() string {
	return p.Status
}
