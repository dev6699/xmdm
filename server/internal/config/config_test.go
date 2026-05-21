package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Clear any existing environment variables
	unsetEnvVars()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify defaults
	if cfg.Server.Address != ":8080" {
		t.Errorf("Expected Server.Address ':8080', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.PublicURL != "http://127.0.0.1:8080" {
		t.Errorf("Expected Server.PublicURL 'http://127.0.0.1:8080', got '%s'", cfg.Server.PublicURL)
	}
	if cfg.Server.SessionTTL != 24*time.Hour {
		t.Errorf("Expected Server.SessionTTL 24h, got %v", cfg.Server.SessionTTL)
	}
	if cfg.Postgres.DSN != "postgres://xmdm:xmdm@127.0.0.1:5432/xmdm?sslmode=disable" {
		t.Errorf("Unexpected Postgres.DSN: %s", cfg.Postgres.DSN)
	}
	if cfg.MQTT.Address != "127.0.0.1:1883" {
		t.Errorf("Expected MQTT.Address '127.0.0.1:1883', got '%s'", cfg.MQTT.Address)
	}
	if cfg.Device.CommandPollInterval != 30*time.Second {
		t.Errorf("Expected Device.CommandPollInterval 30s, got %v", cfg.Device.CommandPollInterval)
	}
	if cfg.Device.ConfigSyncInterval != 15*time.Minute {
		t.Errorf("Expected Device.ConfigSyncInterval 15m, got %v", cfg.Device.ConfigSyncInterval)
	}
	if cfg.Device.AgentAppPackage != "com.xmdm.launcher" {
		t.Errorf("Expected Device.AgentAppPackage 'com.xmdm.launcher', got '%s'", cfg.Device.AgentAppPackage)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	unsetEnvVars()

	os.Setenv("XMDM_ADDR", ":9090")
	os.Setenv("XMDM_SERVER_PUBLIC_URL", "https://mdm.example.com")
	os.Setenv("XMDM_ADMIN_USERNAME", "customuser")
	os.Setenv("XMDM_ADMIN_PASSWORD", "custompass")
	os.Setenv("XMDM_POSTGRES_DSN", "postgres://custom:custom@localhost:5432/test")
	os.Setenv("XMDM_DEVICE_COMMAND_POLL_INTERVAL", "5s")
	os.Setenv("XMDM_DEVICE_CONFIG_SYNC_INTERVAL", "2m")
	os.Setenv("XMDM_DEVICE_AGENT_APP_PACKAGE", "com.example.agent")
	defer unsetEnvVars()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Address != ":9090" {
		t.Errorf("Expected Server.Address ':9090', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.PublicURL != "https://mdm.example.com" {
		t.Errorf("Expected Server.PublicURL 'https://mdm.example.com', got '%s'", cfg.Server.PublicURL)
	}
	if cfg.Admin.Username != "customuser" {
		t.Errorf("Expected Admin.Username 'customuser', got '%s'", cfg.Admin.Username)
	}
	if cfg.Admin.Password != "custompass" {
		t.Errorf("Expected Admin.Password 'custompass', got '%s'", cfg.Admin.Password)
	}
	if cfg.Postgres.DSN != "postgres://custom:custom@localhost:5432/test" {
		t.Errorf("Expected Postgres.DSN 'postgres://custom:custom@localhost:5432/test', got '%s'", cfg.Postgres.DSN)
	}
	if cfg.Device.CommandPollInterval != 5*time.Second {
		t.Errorf("Expected Device.CommandPollInterval 5s, got %v", cfg.Device.CommandPollInterval)
	}
	if cfg.Device.ConfigSyncInterval != 2*time.Minute {
		t.Errorf("Expected Device.ConfigSyncInterval 2m, got %v", cfg.Device.ConfigSyncInterval)
	}
	if cfg.Device.AgentAppPackage != "com.example.agent" {
		t.Errorf("Expected Device.AgentAppPackage 'com.example.agent', got '%s'", cfg.Device.AgentAppPackage)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	unsetEnvVars()

	// Create a temporary YAML file
	content := `
server:
  address: ":9091"
  publicURL: "https://yaml.example.com"
  sessionTTL: 48h

device:
  agentAppPackage: "com.yaml.agent"

postgres:
  dsn: "postgres://yamluser:yamlpass@localhost:5432/yamltest"

admin:
  username: "yamluser"
  password: "yamlpass"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig from YAML failed: %v", err)
	}

	if cfg.Server.Address != ":9091" {
		t.Errorf("Expected Server.Address ':9091', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.PublicURL != "https://yaml.example.com" {
		t.Errorf("Expected Server.PublicURL 'https://yaml.example.com', got '%s'", cfg.Server.PublicURL)
	}
	if cfg.Server.SessionTTL != 48*time.Hour {
		t.Errorf("Expected Server.SessionTTL 48h, got %v", cfg.Server.SessionTTL)
	}
	if cfg.Admin.Username != "yamluser" {
		t.Errorf("Expected Admin.Username 'yamluser', got '%s'", cfg.Admin.Username)
	}
	if cfg.Device.AgentAppPackage != "com.yaml.agent" {
		t.Errorf("Expected Device.AgentAppPackage 'com.yaml.agent', got '%s'", cfg.Device.AgentAppPackage)
	}
}

func TestEnvOverrideYAML(t *testing.T) {
	unsetEnvVars()

	// Create YAML with admin user
	content := `
admin:
  username: "yamluser"
  password: "yamlpass"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	// Override username with env var
	os.Setenv("XMDM_ADMIN_USERNAME", "envuser")
	defer unsetEnvVars()

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig from YAML failed: %v", err)
	}

	// Username should be overridden by env var
	if cfg.Admin.Username != "envuser" {
		t.Errorf("Expected Admin.Username 'envuser' (from env), got '%s'", cfg.Admin.Username)
	}
	// Password should remain from YAML
	if cfg.Admin.Password != "yamlpass" {
		t.Errorf("Expected Admin.Password 'yamlpass' (from YAML), got '%s'", cfg.Admin.Password)
	}
}

func unsetEnvVars() {
	envVars := []string{
		"XMDM_ADDR",
		"XMDM_SERVER_PUBLIC_URL",
		"XMDM_ADMIN_USERNAME",
		"XMDM_ADMIN_PASSWORD",
		"XMDM_SESSION_TTL",
		"XMDM_POSTGRES_DSN",
		"XMDM_MQTT_ADDRESS",
		"XMDM_MQTT_CLIENT_ID",
		"XMDM_MQTT_USERNAME",
		"XMDM_MQTT_PASSWORD",
		"XMDM_MQTT_DYNSEC_ADDRESS",
		"XMDM_MQTT_DYNSEC_CLIENT_ID",
		"XMDM_MQTT_DYNSEC_ADMIN_USER",
		"XMDM_MQTT_DYNSEC_PASSWORD",
		"XMDM_MQTT_KEEPALIVE",
		"XMDM_MQTT_DIAL_TIMEOUT",
		"XMDM_MQTT_DYNSEC_KEEPALIVE",
		"XMDM_MQTT_DYNSEC_DIAL_TIMEOUT",
		"XMDM_DEVICE_COMMAND_POLL_INTERVAL",
		"XMDM_DEVICE_CONFIG_SYNC_INTERVAL",
		"XMDM_DEVICE_AGENT_APP_PACKAGE",
		"XMDM_OBJECT_STORAGE_ENDPOINT",
		"XMDM_OBJECT_STORAGE_REGION",
		"XMDM_OBJECT_STORAGE_ACCESS_KEY",
		"XMDM_OBJECT_STORAGE_SECRET_KEY",
		"XMDM_OBJECT_STORAGE_BUCKET",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}
