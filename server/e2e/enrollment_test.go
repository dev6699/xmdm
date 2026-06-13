package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "xmdm/server/internal/api/v1"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	device "xmdm/server/internal/device"
	devicepg "xmdm/server/internal/device/postgres"
	"xmdm/server/internal/pagination"
	"xmdm/server/internal/plugins"

	"github.com/google/uuid"
)

func TestEnrollmentE2E(t *testing.T) {
	pool := openTestPool(t)
	resetTestDB(t, pool)

	svc := auth.NewService("admin", "secret", time.Minute)
	now := time.Now()
	svc.SetNow(func() time.Time { return now })

	auditStore := auditpg.NewDBStore(pool)
	handler := v1.NewMux(svc, testDeps(t, pool, auditStore, plugins.Disabled(), newTestArtifactStore(t), false))
	client := newE2EClient(t, handler)
	baseURL := "http://xmdm.local"
	deviceID := uuid.NewString()

	login(client, t, baseURL, "admin", "secret")

	policy := mustCreatePolicy(t, pool, `{"name":"enrollment-policy","kioskMode":false,"restrictions":{}}`)
	policyID, _ := policy["id"].(string)
	if policyID == "" {
		t.Fatalf("expected policy id in enrollment policy response: %#v", policy)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO devices (id, tenant_id, display_name, secret_hash, policy_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, deviceID, bootstrap.SeedTenantID, deviceID, "hash-"+deviceID, policyID); err != nil {
		t.Fatalf("seed pending device: %v", err)
	}
	issued := postJSON(t, client, baseURL+"/api/v1/enrollment/tokens", `{"ttlSeconds":3600}`)
	token, _ := issued["token"].(string)
	if token == "" {
		t.Fatalf("expected enrollment token secret in response: %#v", issued)
	}

	bound := postJSON(t, client, baseURL+"/api/v1/enrollment", `{
		"enrollmentToken":"`+token+`",
		"deviceIdentityPolicy":{"deviceId":"`+deviceID+`"},
		"bootstrapExtras":{"customer":"Acme"}
	}`)
	deviceSecret, _ := bound["deviceSecret"].(string)
	if deviceSecret == "" {
		t.Fatalf("expected device secret in response: %#v", bound)
	}
	if bound["status"] != device.StatusEnrolled {
		t.Fatalf("expected enrolled status, got %#v", bound["status"])
	}

	configReq, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/devices/"+deviceID+"/config", nil)
	if err != nil {
		t.Fatalf("build config request: %v", err)
	}
	configReq.Header.Set("X-XMDM-Device-Secret", deviceSecret)
	configRes, err := client.Do(configReq)
	if err != nil {
		t.Fatalf("config request: %v", err)
	}
	defer configRes.Body.Close()
	if configRes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(configRes.Body)
		t.Fatalf("expected config ok, got %d: %s", configRes.StatusCode, strings.TrimSpace(string(body)))
	}
	var config map[string]any
	if err := json.NewDecoder(configRes.Body).Decode(&config); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if config["signature"] == "" {
		t.Fatalf("expected signed config in response: %#v", config)
	}

	reqBody := strings.NewReader(`{"heartbeat":{"online":true}}`)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/devices/"+deviceID+"/telemetry", reqBody)
	if err != nil {
		t.Fatalf("build telemetry request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-XMDM-Device-Secret", deviceSecret)
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("telemetry request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected telemetry ok, got %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var rec map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rec); err != nil {
		t.Fatalf("decode telemetry response: %v", err)
	}
	if rec["deviceId"] == "" {
		t.Fatalf("expected telemetry device id in response: %#v", rec)
	}

	devices, err := devicepg.New(pool).ListDevices(context.Background(), bootstrap.SeedTenantID, pagination.Params{})
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	found := false
	for _, item := range devices {
		if item.ID == deviceID {
			found = true
			if item.Status != device.StatusActive {
				t.Fatalf("expected active status after telemetry, got %#v", item.Status)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected enrolled device to appear in device list: %#v", devices)
	}

	dupToken := postJSON(t, client, baseURL+"/api/v1/enrollment/tokens", `{"ttlSeconds":3600}`)
	dupSecret, _ := dupToken["token"].(string)
	if dupSecret == "" {
		t.Fatalf("expected duplicate enrollment token secret in response: %#v", dupToken)
	}
	assertStatus(t, client, http.MethodPost, baseURL+"/api/v1/enrollment", `{
		"enrollmentToken":"`+dupSecret+`",
		"deviceIdentityPolicy":{"deviceId":"`+deviceID+`"}
	}`, http.StatusConflict)
}
