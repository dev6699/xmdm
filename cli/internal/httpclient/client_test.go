package httpclient

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestResolveURL(t *testing.T) {
	client, err := New("https://mdm.example/api/v1", 5*time.Second)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got, err := client.ResolveURL("devices/device-1")
	if err != nil {
		t.Fatalf("ResolveURL failed: %v", err)
	}

	want := "https://mdm.example/api/v1/devices/device-1"
	if got != want {
		t.Fatalf("unexpected url: got %q want %q", got, want)
	}
}

func TestNewRequestSetsUserAgent(t *testing.T) {
	client, err := New("https://mdm.example", 5*time.Second)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/api/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if got := req.Header.Get("User-Agent"); got != "xmdm-cli" {
		t.Fatalf("unexpected user agent: %q", got)
	}
}
