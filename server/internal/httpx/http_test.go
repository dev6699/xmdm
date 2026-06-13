package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestPaginationParamsClampsLimitBeforeOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=2&limit=1000", nil)
	params := RequestPaginationParams(req, 100)
	if params.Limit != 100 {
		t.Fatalf("unexpected limit: %d", params.Limit)
	}
	if params.Offset != 100 {
		t.Fatalf("unexpected offset: %d", params.Offset)
	}
}
