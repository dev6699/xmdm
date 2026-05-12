package httpx

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithRateLimitsBlocksAfterBurst(t *testing.T) {
	var calls int32
	handler := WithRateLimits(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}), RateLimitRule{
		Name:           "login",
		Method:         http.MethodPost,
		Prefix:         "/api/v1/admin/login",
		Burst:          1,
		RefillInterval: time.Hour,
		RetryAfter:     time.Second,
	})

	first := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	first.RemoteAddr = "203.0.113.10:1234"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, first)
	if rr1.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", rr1.Code, http.StatusNoContent)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", nil)
	second.RemoteAddr = "203.0.113.10:5678"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, second)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", rr2.Code, http.StatusTooManyRequests)
	}
	if got := rr2.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After header = %q, want %q", got, "1")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
}
