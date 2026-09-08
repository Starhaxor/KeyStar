package adminapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/httpapi"
)

func TestAdminAuthenticationRejectsForeignOriginBeforeSettingCookies(t *testing.T) {
	for _, endpoint := range []string{"login", "mfa"} {
		t.Run(endpoint, func(t *testing.T) {
			core := httpapi.NewRouter(httpapi.RouterConfig{Admin: httpapi.AdminConfig{
				Auth:    &fakeAdminAuth{token: "test-session", account: testOwnerAccount()},
				Console: &fakeAdminConsole{}, AllowedOrigins: []string{"https://console.example.com"},
				CookieSecure: true, SessionTTL: time.Hour,
			}})
			core.MountAdmin(New(core))
			body := `{"email":"attacker@example.com","password":"known-password"}`
			if endpoint == "mfa" {
				body = `{"mfa_token":"attacker-challenge","code":"123456"}`
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/admin/auth/"+endpoint, strings.NewReader(body))
			request.Header.Set("Origin", "https://attacker.example")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			core.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || len(response.Result().Cookies()) != 0 {
				t.Fatalf("foreign origin authenticated: status=%d cookies=%d", response.Code, len(response.Result().Cookies()))
			}
		})
	}
}
