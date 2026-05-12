package observability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Logger *log.Logger
	Now    func() time.Time
}

type Handler struct {
	next   http.Handler
	logger *log.Logger
	now    func() time.Time

	mu      sync.Mutex
	metrics map[metricKey]*metricSeries
}

type metricKey struct {
	Method string
	Route  string
	Status string
}

type metricSeries struct {
	Count   int64
	Sum     time.Duration
	Buckets []int64
}

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5}

func NewHandler(next http.Handler, cfg Config) http.Handler {
	if next == nil {
		next = http.DefaultServeMux
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Handler{
		next:    next,
		logger:  logger,
		now:     now,
		metrics: make(map[metricKey]*metricSeries),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/metrics" {
		h.serveMetrics(w)
		return
	}

	start := h.now()
	requestID := requestIDFromHeader(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = newID(16)
	}
	traceID, parentSpanID := traceContextFromHeader(r.Header.Get("traceparent"))
	if traceID == "" {
		traceID = newID(16)
	}
	spanID := newID(8)

	w.Header().Set("X-Request-Id", requestID)
	w.Header().Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	if parentSpanID != "" {
		w.Header().Set("X-Trace-Parent", parentSpanID)
	}

	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
	h.next.ServeHTTP(rec, r)

	duration := h.now().Sub(start)
	route := normalizeRoute(r.URL.Path)
	h.record(r.Method, route, rec.status, duration)
	h.logger.Printf("request_completed request_id=%s trace_id=%s method=%s route=%s status=%d duration_ms=%.3f remote_addr=%s", requestID, traceID, r.Method, route, rec.status, float64(duration.Microseconds())/1000.0, r.RemoteAddr)
}

func (h *Handler) serveMetrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	h.mu.Lock()
	defer h.mu.Unlock()

	type metricLine struct {
		key  metricKey
		kind string
		le   string
		val  float64
	}

	var lines []metricLine
	for key, series := range h.metrics {
		lines = append(lines, metricLine{key: key, kind: "count", val: float64(series.Count)})
		lines = append(lines, metricLine{key: key, kind: "sum", val: series.Sum.Seconds()})
		running := int64(0)
		for i, bucket := range durationBuckets {
			running += series.Buckets[i]
			lines = append(lines, metricLine{key: key, kind: "bucket", le: fmt.Sprintf("%g", bucket), val: float64(running)})
		}
		lines = append(lines, metricLine{key: key, kind: "bucket", le: "+Inf", val: float64(series.Count)})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].key.Method != lines[j].key.Method {
			return lines[i].key.Method < lines[j].key.Method
		}
		if lines[i].key.Route != lines[j].key.Route {
			return lines[i].key.Route < lines[j].key.Route
		}
		if lines[i].key.Status != lines[j].key.Status {
			return lines[i].key.Status < lines[j].key.Status
		}
		if lines[i].kind != lines[j].kind {
			return lines[i].kind < lines[j].kind
		}
		return lines[i].le < lines[j].le
	})

	_, _ = fmt.Fprintln(w, "# HELP xmdm_http_request_duration_seconds Request duration histogram.")
	_, _ = fmt.Fprintln(w, "# TYPE xmdm_http_request_duration_seconds histogram")
	_, _ = fmt.Fprintln(w, "# HELP xmdm_http_requests_total Total HTTP requests handled by XMDM.")
	_, _ = fmt.Fprintln(w, "# TYPE xmdm_http_requests_total counter")
	for _, line := range lines {
		labels := fmt.Sprintf(`method="%s",route="%s",status="%s"`, line.key.Method, line.key.Route, line.key.Status)
		switch line.kind {
		case "count":
			_, _ = fmt.Fprintf(w, "xmdm_http_requests_total{%s} %.0f\n", labels, line.val)
		case "sum":
			_, _ = fmt.Fprintf(w, "xmdm_http_request_duration_seconds_sum{%s} %.6f\n", labels, line.val)
		case "bucket":
			_, _ = fmt.Fprintf(w, "xmdm_http_request_duration_seconds_bucket{%s,le=\"%s\"} %.0f\n", labels, line.le, line.val)
		}
	}
	for key, series := range h.metrics {
		labels := fmt.Sprintf(`method="%s",route="%s",status="%s"`, key.Method, key.Route, key.Status)
		_, _ = fmt.Fprintf(w, "xmdm_http_request_duration_seconds_count{%s} %d\n", labels, series.Count)
	}
}

func (h *Handler) record(method, route string, status int, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := metricKey{
		Method: method,
		Route:  route,
		Status: fmt.Sprintf("%d", status),
	}
	series, ok := h.metrics[key]
	if !ok {
		series = &metricSeries{Buckets: make([]int64, len(durationBuckets))}
		h.metrics[key] = series
	}
	series.Count++
	series.Sum += duration
	seconds := duration.Seconds()
	for i, bucket := range durationBuckets {
		if seconds <= bucket {
			series.Buckets[i]++
		}
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func newID(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func requestIDFromHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func traceContextFromHeader(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	parts := strings.Split(value, "-")
	if len(parts) < 4 {
		return "", ""
	}
	traceID := strings.TrimSpace(parts[1])
	spanID := strings.TrimSpace(parts[2])
	if len(traceID) != 32 || len(spanID) != 16 {
		return "", ""
	}
	if !isHex(traceID) || !isHex(spanID) {
		return "", ""
	}
	return traceID, spanID
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeRoute(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	for i := 1; i < len(parts)-1; i++ {
		if shouldRedactSegment(parts[i], parts[i+1]) {
			parts[i+1] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func shouldRedactSegment(current, next string) bool {
	switch current {
	case "devices", "apps", "versions", "certificates", "managed-files", "commands", "files", "groups", "users", "roles", "policies", "logs", "info":
		if next == "" {
			return false
		}
		switch next {
		case "ack", "artifact", "config", "telemetry", "login", "logout", "me", "tokens", "validate", "consume":
			return false
		}
		return true
	default:
		return false
	}
}
