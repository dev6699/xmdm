package files

import "time"

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

type Artifact struct {
	RecordBase
	StorageKey string `json:"storageKey"`
	Checksum   string `json:"checksum"`
	SizeBytes  int64  `json:"sizeBytes"`
	MimeType   string `json:"mimeType"`
}

type File struct {
	RecordBase
	Name       string    `json:"name"`
	ArtifactID string    `json:"artifactId"`
	Checksum   string    `json:"checksum"`
	MimeType   string    `json:"mimeType"`
	Artifact   *Artifact `json:"artifact,omitempty"`
}

type FileUpsert struct {
	Name       string `json:"name"`
	StorageKey string `json:"storageKey"`
	Checksum   string `json:"checksum"`
	SizeBytes  int64  `json:"sizeBytes"`
	MimeType   string `json:"mimeType"`
}

func (f File) RecordID() string {
	return f.ID
}

func (f File) RecordStatus() string {
	return f.Status
}
