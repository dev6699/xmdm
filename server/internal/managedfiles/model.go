package managedfiles

import (
	"time"

	files "xmdm/server/internal/files"
)

const (
	StatusActive  = "active"
	StatusRetired = "retired"
)

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type ManagedFile struct {
	RecordBase
	FileID           string      `json:"fileId"`
	Path             string      `json:"path"`
	ReplaceVariables bool        `json:"replaceVariables"`
	File             *files.File `json:"file,omitempty"`
}

type ManagedFileUpsert struct {
	FileID           string `json:"fileId"`
	Path             string `json:"path"`
	ReplaceVariables bool   `json:"replaceVariables"`
}

func (f ManagedFile) RecordID() string {
	return f.ID
}

func (f ManagedFile) RecordStatus() string {
	return f.Status
}
