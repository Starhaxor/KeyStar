package adminapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

const onboardingApplicationID = "019c2222-2222-7222-8222-222222222222"

type fakeOnboardingConsole struct {
	httpapi.AdminConsoleStore

	applications []domain.Application
	credentials  []domain.ApplicationCredential
	products     []domain.Product
	plans        map[string][]domain.Plan
	licenseCount int64

	credentialApplicationID string
	productApplicationID    string
	licenseApplicationID    string
}

func (*fakeOnboardingConsole) AppendAuditLog(context.Context, domain.NewAuditLog) error {
	return nil
}

func (*fakeOnboardingConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (fake *fakeOnboardingConsole) ListApplications(context.Context) ([]domain.Application, error) {
	return fake.applications, nil
}

func (fake *fakeOnboardingConsole) ListCredentials(_ context.Context, applicationID string) ([]domain.ApplicationCredential, error) {
	fake.credentialApplicationID = applicationID
	return fake.credentials, nil
}

func (fake *fakeOnboardingConsole) ListProducts(_ context.Context, applicationID string) ([]domain.Product, error) {
	fake.productApplicationID = applicationID
	return fake.products, nil
}

func (fake *fakeOnboardingConsole) ListPlans(_ context.Context, productID string) ([]domain.Plan, error) {
	return fake.plans[productID], nil
}

func (fake *fakeOnboardingConsole) ListConsoleLicenses(_ context.Context, applicationID string, _, _ int, _, _ string) ([]domain.ConsoleLicense, int64, error) {
	fake.licenseApplicationID = applicationID
	return nil, fake.licenseCount, nil
}

type onboardingApplicationResolver struct{}

func (onboardingApplicationResolver) FindApplicationByID(_ context.Context, applicationID string) (*domain.Application, error) {
	return &domain.Application{
		ID:              applicationID,
		OrganizationID:  "org-selected",
		Name:            "Selected app",
		Slug:            "selected-app",
		Status:          domain.ApplicationStatusActive,
		EnvironmentMode: "separate",
	}, nil
}

func newOnboardingTestRouter(t *testing.T, console *fakeOnboardingConsole, account *domain.AdminAccount) *Router {
	t.Helper()
	auth := &fakeAdminAuth{token: "session-token", account: account}
	core := httpapi.NewRouter(httpapi.RouterConfig{
		Applications:         onboardingApplicationResolver{},
		DefaultApplicationID: onboardingApplicationID,
		Admin: httpapi.AdminConfig{
			Auth: auth, Console: console,
			AllowedOrigins: []string{"http://localhost:3000"},
			CSRFSecret:     []byte("test-csrf-secret"),
			SessionTTL:     time.Hour,
		},
	})
	core.MountAdmin(New(core))
	return &Router{Router: core}
}

func onboardingRequest(path string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: httpapi.AdminSessionCookieName, Value: "session-token"})
	request.Header.Set("X-KeyStar-App", onboardingApplicationID)
	return request
}

func TestAdminOnboardingProgressIsApplicationScopedAndSecretFree(t *testing.T) {
	console := &fakeOnboardingConsole{
		applications: []domain.Application{
			{ID: "another-app", Name: "Another app", Status: domain.ApplicationStatusActive},
			{ID: onboardingApplicationID, OrganizationID: "org-selected", Name: "Selected app", Slug: "selected-app", Status: domain.ApplicationStatusActive, EnvironmentMode: "separate"},
		},
		credentials: []domain.ApplicationCredential{
			{ID: "publishable-active", CredentialType: domain.CredentialPublishable, Environment: domain.CredentialEnvironmentTest, Status: domain.CredentialStatusActive, KeyHash: []byte("must-not-leak")},
			{ID: "secret-active", CredentialType: domain.CredentialSecret, Environment: domain.CredentialEnvironmentLive, Status: domain.CredentialStatusActive, KeyHash: []byte("must-not-leak-either")},
			{ID: "publishable-revoked", CredentialType: domain.CredentialPublishable, Environment: domain.CredentialEnvironmentLive, Status: domain.CredentialStatusRevoked},
		},
		products: []domain.Product{
			{ID: "product-active", ApplicationID: onboardingApplicationID, Name: "Desktop", Status: domain.CatalogStatusActive},
			{ID: "product-archived", ApplicationID: onboardingApplicationID, Name: "Old", Status: domain.CatalogStatusArchived},
		},
		plans: map[string][]domain.Plan{
			"product-active": {
				{ID: "plan-active", ProductID: "product-active", Name: "Test", Status: domain.CatalogStatusActive},
				{ID: "plan-archived", ProductID: "product-active", Name: "Old", Status: domain.CatalogStatusArchived},
			},
		},
		licenseCount: 2,
	}
	router := newOnboardingTestRouter(t, console, testOwnerAccount())
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, onboardingRequest("/v1/admin/onboarding/progress"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`"id":"` + onboardingApplicationID + `"`,
		`"credential_count":1`,
		`"credential_environment":"test"`,
		`"product_count":1`,
		`"plan_count":1`,
		`"license_count":2`,
		`"product":{"id":"product-active","name":"Desktop"}`,
		`"plan":{"id":"plan-active","name":"Test"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body %s does not contain %s", body, expected)
		}
	}
	for _, forbidden := range []string{"must-not-leak", "key_hash", "key", "secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("body contains forbidden credential data %q: %s", forbidden, body)
		}
	}
	if console.credentialApplicationID != onboardingApplicationID || console.productApplicationID != onboardingApplicationID || console.licenseApplicationID != onboardingApplicationID {
		t.Fatalf("scope = credentials %q, products %q, licenses %q", console.credentialApplicationID, console.productApplicationID, console.licenseApplicationID)
	}
}

func TestAdminOnboardingProgressRequiresAllReadPermissions(t *testing.T) {
	account := testOwnerAccount()
	account.Permissions = []string{
		domain.PermApplicationsRead,
		domain.PermCredentialsRead,
		domain.PermCatalogRead,
	}
	router := newOnboardingTestRouter(t, &fakeOnboardingConsole{}, account)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, onboardingRequest("/v1/admin/onboarding/progress"))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"PERMISSION_DENIED"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
