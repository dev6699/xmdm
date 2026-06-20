package observability

import (
	"bytes"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRecordsRequestMetadataAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Config{Logger: log.New(&logs, "", 0)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/config", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", res.Code)
	}
	if got := res.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("unexpected request id: %q", got)
	}
	if got := res.Header().Get("traceparent"); !strings.HasPrefix(got, "00-0123456789abcdef0123456789abcdef-") {
		t.Fatalf("unexpected traceparent: %q", got)
	}
	if got := logs.String(); !strings.Contains(got, "request_completed") || !strings.Contains(got, "route=/api/v1/devices/{id}/config") {
		t.Fatalf("unexpected log line: %s", got)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRes := httptest.NewRecorder()
	handler.ServeHTTP(metricsRes, metricsReq)

	if metricsRes.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", metricsRes.Code, http.StatusOK)
	}
	body, err := io.ReadAll(metricsRes.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	metrics := string(body)
	if !strings.Contains(metrics, `xmdm_http_requests_total{method="GET",route="/api/v1/devices/{id}/config",status="204"} 1`) {
		t.Fatalf("missing request counter in metrics: %s", metrics)
	}
	if !strings.Contains(metrics, `xmdm_http_request_duration_seconds_count{method="GET",route="/api/v1/devices/{id}/config",status="204"} 1`) {
		t.Fatalf("missing duration counter in metrics: %s", metrics)
	}
}

func TestHandlerServesHealthEndpoint(t *testing.T) {
	handler := NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Config{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := strings.TrimSpace(res.Body.String()); got != "ok" {
		t.Fatalf("unexpected health body: %q", got)
	}
}

func TestHandlerCanDisableRequestLogging(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Config{Logger: log.New(&logs, "", 0), DisableRequestLog: true})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-123/config", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", res.Code)
	}
	if got := logs.String(); got != "" {
		t.Fatalf("expected no request log output, got %q", got)
	}
}

func TestHandlerRejectsMalformedTraceparent(t *testing.T) {
	handler := NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), Config{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("traceparent", "00-xyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxy-0123456789abcdef-01")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	got := res.Header().Get("traceparent")
	if got == "" {
		t.Fatalf("expected traceparent response header")
	}
	parts := strings.Split(got, "-")
	if len(parts) != 4 {
		t.Fatalf("unexpected traceparent format: %q", got)
	}
	if parts[1] == "xyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxy" {
		t.Fatalf("malformed trace id was propagated: %q", got)
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		t.Fatalf("response trace id is not hex: %q", got)
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		t.Fatalf("response span id is not hex: %q", got)
	}
}
