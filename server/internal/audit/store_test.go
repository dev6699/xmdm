package audit

import (
	"testing"
	"time"
)

func TestStoreRecordsAppendOnlyEvents(t *testing.T) {
	store := NewStore()
	now := time.Now()
	store.SetNow(func() time.Time { return now })

	details := map[string]any{"kind": "users"}
	event := store.Record("tenant-1", "admin", "create", "users", "users-1", details)
	details["kind"] = "changed"

	if event.ID != "1" {
		t.Fatalf("expected first event id 1, got %s", event.ID)
	}
	if event.Details["kind"] != "users" {
		t.Fatalf("expected copied details, got %v", event.Details["kind"])
	}
	if got := store.List("tenant-1"); len(got) != 1 {
		t.Fatalf("expected one event, got %d", len(got))
	}
}
