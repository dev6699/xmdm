package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xmdm/server/internal/auth"
)

func TestNewMuxExposesMetrics(t *testing.T) {
	svc := auth.NewService("admin", "secret", time.Hour)
	handler := NewMux(svc, Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("unexpected content type: %q", got)
	}
}
