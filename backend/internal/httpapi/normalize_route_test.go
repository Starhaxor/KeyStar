package httpapi

import "testing"

func TestNormalizeRouteCollapsesIdentifiers(t *testing.T) {
	routes := []struct{ input, want string }{
		{"/healthz", "/healthz"},
		{"/metrics", "/metrics"},
		{"/v1/admin/users", "/v1/admin/users"},
		{"/v1/admin/users/019c1111-1111-7111-8111-111111111111", "/v1/admin/users/:id"},
		{"/v1/admin/users/019c1111-1111-7111-8111-111111111111/sessions", "/v1/admin/users/:id/sessions"},
		{"/v1/server/licenses/019cabcd-1111-7111-8111-111111111111/revoke", "/v1/server/licenses/:id/revoke"},
	}
	for _, testCase := range routes {
		if got := NormalizeRoute(testCase.input); got != testCase.want {
			t.Fatalf("NormalizeRoute(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}
}
