package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/starloader/backend/internal/metrics"
)

const (
	metricRequestsTotal   = "keystar_http_requests_total"
	metricRequestDuration = "keystar_http_request_duration_seconds"
)

// RequestObserver instruments one HTTP round trip: a per-route/status
// counter plus a latency histogram. Route labels use the normalized path so
// identifiers never explode label cardinality.
func RequestObserver(registry *metrics.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/metrics" {
			next.ServeHTTP(writer, request)
			return
		}
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)

		labels := metrics.Labels{
			"method": request.Method,
			"route":  NormalizeRoute(request.URL.Path),
			"status": strconv.Itoa(recorder.status),
		}
		registry.IncCounter(metricRequestsTotal, labels)
		registry.ObserveHistogram(metricRequestDuration,
			metrics.Labels{"method": request.Method, "route": NormalizeRoute(request.URL.Path)},
			time.Since(started).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if !recorder.wroteHeader {
		recorder.status = status
		recorder.wroteHeader = true
	}
	recorder.ResponseWriter.WriteHeader(status)
}

// NormalizeRoute collapses identifier-like path segments into ":id" so the
// /v1/admin/users/019c...-... and /v1/admin/users/<other> share one series.
func NormalizeRoute(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, segment := range segments {
		if looksLikeIdentifier(segment) {
			segments[i] = ":id"
		}
	}
	return "/" + strings.Join(segments, "/")
}

func looksLikeIdentifier(segment string) bool {
	if segment == "" || len(segment) < 16 {
		return false
	}
	dashes := strings.Count(segment, "-")
	if dashes == 4 && len(segment) == 36 { // canonical UUID
		return true
	}
	if dashes > 0 && isHexy(segment) { // UUIDv7 without dashes or other hex ids
		return true
	}
	return false
}

func isHexy(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') && r != '-' {
			return false
		}
	}
	return true
}
