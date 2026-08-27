package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starloader/backend/internal/metrics"
)

func TestMetricsEndpointRequiresConfiguredBearerToken(t *testing.T) {
	registry := metrics.NewRegistry()
	router := NewRouter(RouterConfig{Metrics: registry, MetricsToken: "metrics-token-0123456789abcdef0123456789"})
	for _, test := range []struct {
		auth string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong", http.StatusUnauthorized},
		{"Bearer metrics-token-0123456789abcdef0123456789", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		request.Header.Set("Authorization", test.auth)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Fatalf("auth=%q status=%d body=%s", test.auth, recorder.Code, recorder.Body.String())
		}
	}
}
