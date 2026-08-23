package metrics

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryRendersPrometheusText(t *testing.T) {
	registry := NewRegistry()
	registry.DeclareCounter("keystar_http_requests_total", "HTTP requests processed.")
	registry.DeclareHistogram("keystar_http_request_duration_seconds", "HTTP request latency in seconds.")
	registry.DeclareGauge("keystar_webhook_backlog", "Webhook deliveries waiting to be sent.")

	registry.IncCounter("keystar_http_requests_total", Labels{"method": "GET", "route": "/v1/me", "status": "200"})
	registry.IncCounter("keystar_http_requests_total", Labels{"method": "POST", "route": "/v1/auth/login", "status": "429"})
	registry.SetGauge("keystar_webhook_backlog", nil, 7)
	registry.ObserveHistogram("keystar_http_request_duration_seconds", Labels{"method": "GET"}, 0.02)
	registry.ObserveHistogram("keystar_http_request_duration_seconds", Labels{"method": "GET"}, 3)

	var output bytes.Buffer
	registry.WritePrometheus(&output)
	text := output.String()

	for _, expected := range []string{
		`# TYPE keystar_http_requests_total counter`,
		`keystar_http_requests_total{method="GET",route="/v1/me",status="200"} 1`,
		`# TYPE keystar_webhook_backlog gauge`,
		"keystar_webhook_backlog 7",
		`keystar_http_request_duration_seconds_count{method="GET"} 2`,
		`keystar_http_request_duration_seconds_sum{method="GET"} 3.02`,
		`keystar_http_request_duration_seconds_bucket{le="0.05",method="GET"} 1`,
		`keystar_http_request_duration_seconds_bucket{le="+Inf",method="GET"} 2`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output missing %q:\n%s", expected, text)
		}
	}
}

func TestRegistryHandlerServesContentType(t *testing.T) {
	registry := NewRegistry()
	registry.DeclareCounter("test_total", "Test counter.")
	registry.IncCounter("test_total", nil)

	recorder := httptest.NewRecorder()
	registry.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "test_total 1") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
