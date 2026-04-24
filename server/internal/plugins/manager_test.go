package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xmdm/server/internal/httpx"
)

func TestDisabledManagerRegistersNoRoutes(t *testing.T) {
	mux := http.NewServeMux()
	Disabled().Register(httpx.WithPrefix(mux, "/api/v1/admin"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled plugins, got %d", res.Code)
	}
}

func TestEnabledManagerRegistersOptionalRoute(t *testing.T) {
	mux := http.NewServeMux()
	Enabled().Register(httpx.WithPrefix(mux, "/api/v1/admin"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for enabled plugins, got %d", res.Code)
	}
}
