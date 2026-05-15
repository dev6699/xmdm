package certificates

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
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Certificate struct {
	RecordBase
	Name       string          `json:"name"`
	ArtifactID string          `json:"artifactId"`
	Checksum   string          `json:"checksum"`
	Artifact   *files.Artifact `json:"artifact,omitempty"`
}

type CertificateUpsert struct {
	Name       string `json:"name"`
	StorageKey string `json:"storageKey"`
	Checksum   string `json:"checksum"`
	SizeBytes  int64  `json:"sizeBytes"`
	MimeType   string `json:"mimeType"`
}

func (c Certificate) RecordID() string {
	return c.ID
}

func (c Certificate) RecordStatus() string {
	return c.Status
}
