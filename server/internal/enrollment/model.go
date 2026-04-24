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
	DeviceID     string         `json:"deviceId"`
	DeviceSecret string         `json:"deviceSecret"`
	Status       string         `json:"status"`
	Config       ConfigSnapshot `json:"config"`
}

type ConfigSnapshot struct {
	Version      string         `json:"version"`
	Device       map[string]any `json:"device"`
	Policy       map[string]any `json:"policy"`
	Apps         []any          `json:"apps"`
	Files        []any          `json:"files"`
	Certificates []any          `json:"certificates"`
	Commands     []any          `json:"commands"`
	Signature    string         `json:"signature"`
}
