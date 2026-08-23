package adminapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

// fakeListConsole captures the filter arguments the console list handlers
// forward so server-side search can be tested without a database.
type fakeListConsole struct {
	httpapi.AdminConsoleStore
	licenseSearch string
	licenseStatus string
	deviceSearch  string
	deviceStatus  string
	sessionSearch string
	sessionStatus string
	auditSearch   string
	eventSearch   string
	eventSeverity string
}

func (fake *fakeListConsole) ListConsoleLicenses(_ context.Context, _ string, _, _ int, search, status string) ([]domain.ConsoleLicense, int64, error) {
	fake.licenseSearch = search
	fake.licenseStatus = status
	return []domain.ConsoleLicense{}, 0, nil
}

func (fake *fakeListConsole) ListConsoleDevices(_ context.Context, _ string, _, _ int, search, status string) ([]domain.ConsoleDevice, int64, error) {
	fake.deviceSearch = search
	fake.deviceStatus = status
	return []domain.ConsoleDevice{}, 0, nil
}

func (fake *fakeListConsole) ListConsoleSessions(_ context.Context, _ string, _, _ int, search, status string) ([]domain.ConsoleSession, int64, error) {
	fake.sessionSearch = search
	fake.sessionStatus = status
	return []domain.ConsoleSession{}, 0, nil
}

func (fake *fakeListConsole) ListAuditLogs(_ context.Context, _, _ int, search string) ([]domain.AuditLog, int64, error) {
	fake.auditSearch = search
	return []domain.AuditLog{}, 0, nil
}

func (fake *fakeListConsole) ListSecurityEvents(_ context.Context, _, _ int, search, severity string) ([]domain.SecurityEvent, int64, error) {
	fake.eventSearch = search
	fake.eventSeverity = severity
	return []domain.SecurityEvent{}, 0, nil
}

func newListTestRouter(t *testing.T, console httpapi.AdminConsoleStore) *Router {
	t.Helper()
	auth := &fakeAdminAuth{token: "session-token", account: testOwnerAccount()}
	core := httpapi.NewRouter(httpapi.RouterConfig{
		Admin: httpapi.AdminConfig{
			Auth: auth, Console: console, AllowedOrigins: []string{"http://localhost:3000"},
			CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour,
		},
		DefaultApplicationID: "019c1111-1111-7111-8111-111111111111",
	})
	core.MountAdmin(New(core))
	return &Router{Router: core}
}

func TestAdminListEndpointsForwardSearchAndFilters(t *testing.T) {
	console := &fakeListConsole{}
	router := newListTestRouter(t, console)

	cases := []struct {
		name   string
		path   string
		assert func(t *testing.T, console *fakeListConsole)
	}{
		{
			name: "licenses",
			path: "/v1/admin/licenses?search=alice&status=revoked",
			assert: func(t *testing.T, console *fakeListConsole) {
				if console.licenseSearch != "alice" || console.licenseStatus != "revoked" {
					t.Fatalf("license filters = (%q, %q)", console.licenseSearch, console.licenseStatus)
				}
			},
		},
		{
			name: "devices",
			path: "/v1/admin/devices?search=bob&status=active",
			assert: func(t *testing.T, console *fakeListConsole) {
				if console.deviceSearch != "bob" || console.deviceStatus != "active" {
					t.Fatalf("device filters = (%q, %q)", console.deviceSearch, console.deviceStatus)
				}
			},
		},
		{
			name: "sessions",
			path: "/v1/admin/sessions?search=carol&status=pending",
			assert: func(t *testing.T, console *fakeListConsole) {
				if console.sessionSearch != "carol" || console.sessionStatus != "pending" {
					t.Fatalf("session filters = (%q, %q)", console.sessionSearch, console.sessionStatus)
				}
			},
		},
		{
			name: "audit-logs",
			path: "/v1/admin/audit-logs?search=LICENSE",
			assert: func(t *testing.T, console *fakeListConsole) {
				if console.auditSearch != "LICENSE" {
					t.Fatalf("audit search = %q", console.auditSearch)
				}
			},
		},
		{
			name: "security-events",
			path: "/v1/admin/security-events?search=mfa&severity=critical",
			assert: func(t *testing.T, console *fakeListConsole) {
				if console.eventSearch != "mfa" || console.eventSeverity != "critical" {
					t.Fatalf("event filters = (%q, %q)", console.eventSearch, console.eventSeverity)
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodGet, testCase.path, ""))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			testCase.assert(t, console)
		})
	}
}
