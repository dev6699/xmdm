package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := State{
		BaseURL:     "https://mdm.example/api/v1",
		ProfileName: "local",
		Username:    "admin",
		CookieValue: "abc123",
		ExpiresAt:   time.Now().Add(time.Hour).UTC(),
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.BaseURL != want.BaseURL || got.Username != want.Username || got.CookieValue != want.CookieValue {
		t.Fatalf("unexpected state: %#v", got)
	}
	if got.CookieName != CookieName {
		t.Fatalf("unexpected cookie name: %q", got.CookieName)
	}

	if err := Clear(path); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, stat err=%v", err)
	}
}
