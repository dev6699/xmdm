package admin

import (
	"testing"
	"time"
)

func TestStoreCrudAndRetire(t *testing.T) {
	store := NewStore()
	now := time.Now()
	store.SetNow(func() time.Time { return now })

	role := store.Create("roles", "tenant-1", "admins", map[string]any{"permissions": []string{"admin.read"}})
	if role.Name != "admins" {
		t.Fatalf("unexpected role name: %s", role.Name)
	}
	updated, err := store.Update("roles", "tenant-1", role.ID, "owners", nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "owners" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}
	retired, err := store.Retire("roles", "tenant-1", role.ID)
	if err != nil {
		t.Fatalf("retire failed: %v", err)
	}
	if retired.Status != "retired" || retired.DeletedAt == nil {
		t.Fatalf("expected retired record with deleted_at")
	}
	if got := store.List("roles", "tenant-1"); len(got) != 1 {
		t.Fatalf("expected one role in list, got %d", len(got))
	}
}
