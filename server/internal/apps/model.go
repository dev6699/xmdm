package apps

import "time"

import files "xmdm/server/internal/files"

const (
	StatusActive  = "active"
	StatusRetired = "retired"

	VersionStatusUploaded  = "uploaded"
	VersionStatusPublished = "published"
)

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type App struct {
	RecordBase
	PackageName string `json:"packageName"`
	Name        string `json:"name"`
}

type AppUpsert struct {
	PackageName string `json:"packageName"`
	Name        string `json:"name"`
}

type Version struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenantId"`
	AppID       string          `json:"appId"`
	Status      string          `json:"status"`
	VersionName string          `json:"versionName"`
	VersionCode int64           `json:"versionCode"`
	ArtifactID  *string         `json:"artifactId,omitempty"`
	Artifact    *files.Artifact `json:"artifact,omitempty"`
	Checksum    string          `json:"checksum"`
	PublishedAt *time.Time      `json:"publishedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type VersionUpsert struct {
	VersionName string  `json:"versionName"`
	VersionCode int64   `json:"versionCode"`
	ArtifactID  *string `json:"artifactId,omitempty"`
	Checksum    string  `json:"checksum"`
	Publish     bool    `json:"publish"`
}

func (a App) RecordID() string {
	return a.ID
}

func (a App) RecordStatus() string {
	return a.Status
}

func (v Version) RecordID() string {
	return v.ID
}

func (v Version) RecordStatus() string {
	return v.Status
}
