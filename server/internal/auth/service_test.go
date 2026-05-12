package auth

import (
	"testing"
	"time"
)

func TestLoginLogoutAndExpiry(t *testing.T) {
	svc := NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if _, ok := svc.Authenticate(session.ID); !ok {
		t.Fatalf("session should authenticate")
	}

	svc.Logout(session.ID)
	if _, ok := svc.Authenticate(session.ID); ok {
		t.Fatalf("session should be removed after logout")
	}

	session, err = svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("second login failed: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, ok := svc.Authenticate(session.ID); ok {
		t.Fatalf("session should expire")
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	svc := NewService("admin", "secret", time.Minute)
	if _, err := svc.Login("admin", "wrong"); err == nil {
		t.Fatalf("expected invalid credentials")
	}
	if _, err := svc.Login("root", "secret"); err == nil {
		t.Fatalf("expected invalid credentials for bad username")
	}
}

func TestAuthorizeChecksPermissions(t *testing.T) {
	svc := NewServiceWithPermissions("admin", "secret", time.Minute, []Permission{PermissionAdminRead})
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	session, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if _, ok := svc.Authorize(session.ID, PermissionAdminRead); !ok {
		t.Fatalf("expected admin.read permission")
	}
	if _, ok := svc.Authorize(session.ID, PermissionDevicesWrite); ok {
		t.Fatalf("did not expect devices.write permission")
	}
}
