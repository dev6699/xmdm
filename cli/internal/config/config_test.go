package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePrefersFlagsOverEnvAndFile(t *testing.T) {
	path := writeTempConfig(t, `
defaultProfile: local
defaultFormat: table
defaultTimeout: 45s
profiles:
  local:
    baseUrl: "https://file.example"
    authMode: "session"
    outputFormat: "table"
    timeout: "20s"
`)

	t.Setenv("XMDM_CONFIG", path)
	t.Setenv("XMDM_BASE_URL", "https://env.example")
	t.Setenv("XMDM_OUTPUT_FORMAT", "json")
	t.Setenv("XMDM_TIMEOUT", "10s")

	resolved, err := Resolve(Options{
		BaseURL:    "https://flag.example",
		AuthMode:   "token",
		Format:     "yaml",
		Timeout:    3 * time.Second,
		ConfigPath: path,
		Profile:    "local",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.BaseURL != "https://flag.example" {
		t.Fatalf("unexpected base url: %q", resolved.BaseURL)
	}
	if resolved.AuthMode != "token" {
		t.Fatalf("unexpected auth mode: %q", resolved.AuthMode)
	}
	if resolved.OutputFormat != "yaml" {
		t.Fatalf("unexpected format: %q", resolved.OutputFormat)
	}
	if resolved.Timeout != 3*time.Second {
		t.Fatalf("unexpected timeout: %v", resolved.Timeout)
	}
}

func TestResolveUsesProfileAndDefaults(t *testing.T) {
	path := writeTempConfig(t, `
defaultProfile: local
profiles:
  local:
    baseUrl: "https://file.example"
    authMode: "session"
    outputFormat: "json"
    timeout: "20s"
`)

	t.Setenv("XMDM_CONFIG", path)
	resolved, err := Resolve(Options{ConfigPath: path})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.ProfileName != "local" {
		t.Fatalf("unexpected profile: %q", resolved.ProfileName)
	}
	if resolved.BaseURL != "https://file.example" {
		t.Fatalf("unexpected base url: %q", resolved.BaseURL)
	}
	if resolved.OutputFormat != "json" {
		t.Fatalf("unexpected format: %q", resolved.OutputFormat)
	}
	if resolved.Timeout != 20*time.Second {
		t.Fatalf("unexpected timeout: %v", resolved.Timeout)
	}
}

func TestResolveFallsBackToEnvProfile(t *testing.T) {
	path := writeTempConfig(t, `
profiles:
  prod:
    baseUrl: "https://prod.example"
`)

	t.Setenv("XMDM_CONFIG", path)
	t.Setenv("XMDM_PROFILE", "prod")

	resolved, err := Resolve(Options{ConfigPath: path})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolved.ProfileName != "prod" {
		t.Fatalf("unexpected profile: %q", resolved.ProfileName)
	}
	if resolved.BaseURL != "https://prod.example" {
		t.Fatalf("unexpected base url: %q", resolved.BaseURL)
	}
}

func TestRequireTarget(t *testing.T) {
	if err := RequireTarget(Resolved{}); err == nil {
		t.Fatal("expected error")
	}
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
