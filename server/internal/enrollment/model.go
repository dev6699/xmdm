package enrollment

import "time"

const (
	TokenStatusIssued   = "issued"
	TokenStatusConsumed = "consumed"
	TokenStatusExpired  = "expired"
	TokenStatusRevoked  = "revoked"
)

type Token struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ConsumedAt *time.Time `json:"consumedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type IssuedToken struct {
	Token
	Secret string `json:"token"`
}

type BoundDevice struct {
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
	Status       string `json:"status"`
}

type DeviceSnapshot struct {
	DeviceID    string `json:"deviceId"`
	DeviceIDUse string `json:"deviceIdUse"`
}

type PolicySnapshot struct {
	Name         string             `json:"name,omitempty"`
	Version      int                `json:"version,omitempty"`
	KioskMode    bool               `json:"kioskMode"`
	Restrictions PolicyRestrictions `json:"restrictions,omitempty"`
}

type PolicyRestrictions struct {
	AllowPackages   []string `json:"allowPackages,omitempty"`
	BlockPackages   []string `json:"blockPackages,omitempty"`
	SuspendPackages []string `json:"suspendPackages,omitempty"`
}

type AppSnapshot struct {
	AppID        string `json:"appId"`
	PackageName  string `json:"packageName"`
	Name         string `json:"name,omitempty"`
	VersionID    string `json:"versionId"`
	VersionName  string `json:"versionName"`
	VersionCode  int64  `json:"versionCode"`
	ArtifactID   string `json:"artifactId"`
	Checksum     string `json:"checksum"`
	DownloadPath string `json:"downloadPath"`
}

type CertificateSnapshot struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ArtifactID string `json:"artifactId"`
	Checksum   string `json:"checksum"`
}

type ConfigSnapshot struct {
	Version      string                `json:"version"`
	Device       DeviceSnapshot        `json:"device"`
	Policy       PolicySnapshot        `json:"policy"`
	Apps         []AppSnapshot         `json:"apps"`
	Files        []ManagedFileSnapshot `json:"files"`
	Certificates []CertificateSnapshot `json:"certificates"`
	Signature    string                `json:"signature"`
}
