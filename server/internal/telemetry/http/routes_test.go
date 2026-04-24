package telemetryhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/httpx"
	"xmdm/server/internal/telemetry"
)

func TestRegisterTelemetryUpload(t *testing.T) {
	store := &fakeTelemetryStore{
		record: telemetry.Record{
			ID:         "telemetry-1",
			TenantID:   "tenant-1",
			DeviceID:   "device-123",
			ObservedAt: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
			Payload:    map[string]any{"heartbeat": map[string]any{"online": true}},
		},
	}

	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/telemetry", bytes.NewBufferString(`{
		"heartbeat":{"online":true},
		"battery":{"level":82}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(deviceSecretHeader, "device-secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d", res.Code)
	}
	if store.tenant != "tenant-1" {
		t.Fatalf("unexpected tenant: %q", store.tenant)
	}
	if store.device != "device-123" {
		t.Fatalf("unexpected device: %q", store.device)
	}
	if store.secret != "device-secret" {
		t.Fatalf("unexpected secret: %q", store.secret)
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode telemetry response: %v", err)
	}
	if payload["deviceId"] != "device-123" {
		t.Fatalf("unexpected device id: %#v", payload["deviceId"])
	}
	if payload["id"] != "telemetry-1" {
		t.Fatalf("unexpected telemetry id: %#v", payload["id"])
	}
}

func TestRegisterTelemetryUploadValidation(t *testing.T) {
	store := &fakeTelemetryStore{}
	mux := http.NewServeMux()
	Register(httpx.WithPrefix(mux, "/api/v1"), store, "tenant-1")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/telemetry", bytes.NewBufferString(`{"heartbeat":{"online":true}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", res.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-123/telemetry", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(deviceSecretHeader, "device-secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", res.Code)
	}
}

type fakeTelemetryStore struct {
	record telemetry.Record
	req    telemetry.UploadRequest
	tenant string
	device string
	secret string
}

func (s *fakeTelemetryStore) Upload(_ context.Context, tenantID, deviceID, secret string, req telemetry.UploadRequest) (telemetry.Record, error) {
	s.tenant = tenantID
	s.device = deviceID
	s.secret = secret
	s.req = req
	return s.record, nil
}
