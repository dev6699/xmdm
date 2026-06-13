package users

import (
	"errors"
	"time"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type RecordBase struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type User struct {
	RecordBase
	Email  string `json:"email"`
	RoleID string `json:"roleId"`
}

type UserUpsert struct {
	Email        string `json:"email"`
	PasswordHash string `json:"passwordHash"`
	RoleID       string `json:"roleId"`
}

func (u User) RecordID() string {
	return u.ID
}

func (u User) RecordStatus() string {
	return u.Status
}
