package httpx

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitRule describes a simple token bucket gate for a request scope.
type RateLimitRule struct {
	Name           string
	Method         string
	Prefix         string
	Burst          int
	RefillInterval time.Duration
	RetryAfter     time.Duration
}

type rateLimitHandler struct {
	next    http.Handler
	rules   []RateLimitRule
	mu      sync.Mutex
	buckets map[string]*rateLimitBucket
}

type rateLimitBucket struct {
	tokens float64
	last   time.Time
}

// WithRateLimits wraps next with a per-IP token bucket limiter for matching rules.
func WithRateLimits(next http.Handler, rules ...RateLimitRule) http.Handler {
	return &rateLimitHandler{
		next:    next,
		rules:   rules,
		buckets: make(map[string]*rateLimitBucket),
	}
}

func (h *rateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.matchRule(r)
	if ok && !h.allow(rule, r) {
		retryAfter := rule.RetryAfter
		if retryAfter <= 0 {
			retryAfter = time.Second
		}
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconvDurationSeconds(retryAfter))
		}
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	h.next.ServeHTTP(w, r)
}

func (h *rateLimitHandler) matchRule(r *http.Request) (RateLimitRule, bool) {
	path := r.URL.Path
	var (
		matched   RateLimitRule
		matchedOK bool
	)
	for _, rule := range h.rules {
		if rule.Method != "" && rule.Method != r.Method {
			continue
		}
		if rule.Prefix != "" && !strings.HasPrefix(path, rule.Prefix) {
			continue
		}
		if !matchedOK || len(rule.Prefix) > len(matched.Prefix) {
			matched = rule
			matchedOK = true
		}
	}
	return matched, matchedOK
}

func (h *rateLimitHandler) allow(rule RateLimitRule, r *http.Request) bool {
	if rule.Burst <= 0 {
		return true
	}
	if rule.RefillInterval <= 0 {
		rule.RefillInterval = time.Second
	}
	now := time.Now()
	key := rateLimitKey(rule.Name, r)

	h.mu.Lock()
	defer h.mu.Unlock()

	bucket, ok := h.buckets[key]
	if !ok {
		h.buckets[key] = &rateLimitBucket{
			tokens: float64(rule.Burst - 1),
			last:   now,
		}
		return true
	}

	if bucket.last.IsZero() {
		bucket.last = now
	}
	elapsed := now.Sub(bucket.last)
	if elapsed > 0 {
		refill := elapsed.Seconds() / rule.RefillInterval.Seconds()
		if refill > 0 {
			bucket.tokens = math.Min(float64(rule.Burst), bucket.tokens+refill)
		}
		bucket.last = now
	}

	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func rateLimitKey(scope string, r *http.Request) string {
	host := r.RemoteAddr
	if parsedHost, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsedHost
	}
	if scope == "" {
		scope = r.Method + " " + r.URL.Path
	}
	return scope + "|" + host
}

func strconvDurationSeconds(d time.Duration) string {
	seconds := int64(math.Ceil(d.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}
