package identity

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

type User struct {
	RecordBase
	Email  string `json:"email"`
	RoleID string `json:"roleId"`
}

type Role struct {
	RecordBase
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type UserUpsert struct {
	Email        string `json:"email"`
	PasswordHash string `json:"passwordHash"`
	RoleID       string `json:"roleId"`
}

type RoleUpsert struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

func (u User) RecordID() string {
	return u.ID
}

func (u User) RecordStatus() string {
	return u.Status
}

func (r Role) RecordID() string {
	return r.ID
}

func (r Role) RecordStatus() string {
	return r.Status
}

func (r Role) ClonePermissions() []string {
	if r.Permissions == nil {
		return nil
	}
	out := make([]string, len(r.Permissions))
	copy(out, r.Permissions)
	return out
}

func PermissionsFromJSON(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var permissions []string
	if err := json.Unmarshal(raw, &permissions); err != nil {
		return nil, err
	}
	return permissions, nil
}
