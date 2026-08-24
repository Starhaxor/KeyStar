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

var onboardingNow = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

type fakeOnboardingConsole struct {
	httpapi.AdminConsoleStore

	applications []domain.Application
	credentials  []domain.ApplicationCredential
	products     []domain.Product
	plans        map[string][]domain.Plan
	licenses     []domain.ConsoleLicense

	credentialApplicationID string
	productApplicationID    string
	licenseApplicationID    string
	licenseStatus           string
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

func (fake *fakeOnboardingConsole) ListConsoleLicenses(_ context.Context, applicationID string, offset, limit int, _, status string) ([]domain.ConsoleLicense, int64, error) {
	fake.licenseApplicationID = applicationID
	fake.licenseStatus = status
	if offset >= len(fake.licenses) {
		return []domain.ConsoleLicense{}, int64(len(fake.licenses)), nil
	}
	end := min(offset+limit, len(fake.licenses))
	return fake.licenses[offset:end], int64(len(fake.licenses)), nil
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
		Now:                  func() time.Time { return onboardingNow },
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
			{ID: "publishable-time-expired", CredentialType: domain.CredentialPublishable, Environment: domain.CredentialEnvironmentLive, Status: domain.CredentialStatusActive, ExpiresAt: timePointer(onboardingNow.Add(-time.Minute))},
			{ID: "secret-active", CredentialType: domain.CredentialSecret, Environment: domain.CredentialEnvironmentLive, Status: domain.CredentialStatusActive, KeyHash: []byte("must-not-leak-either")},
			{ID: "publishable-revoked", CredentialType: domain.CredentialPublishable, Environment: domain.CredentialEnvironmentLive, Status: domain.CredentialStatusRevoked},
		},
		products: []domain.Product{
			{ID: "product-active", ApplicationID: onboardingApplicationID, Name: "Desktop", Status: domain.CatalogStatusActive},
			{ID: "product-other", ApplicationID: onboardingApplicationID, Name: "Other", Status: domain.CatalogStatusActive},
			{ID: "product-archived", ApplicationID: onboardingApplicationID, Name: "Old", Status: domain.CatalogStatusArchived},
		},
		plans: map[string][]domain.Plan{
			"product-active": {
				{ID: "plan-active", ProductID: "product-active", Name: "Test", Status: domain.CatalogStatusActive},
				{ID: "plan-archived", ProductID: "product-active", Name: "Old", Status: domain.CatalogStatusArchived},
			},
			"product-other": {
				{ID: "plan-other", ProductID: "product-other", Name: "Other", Status: domain.CatalogStatusActive},
			},
		},
		licenses: []domain.ConsoleLicense{
			{ID: "license-valid", ProductID: "product-active", PlanID: "plan-active", Status: domain.LicenseStatusActive, ExpiresAt: onboardingNow.Add(time.Hour)},
			{ID: "license-revoked", ProductID: "product-active", PlanID: "plan-active", Status: domain.LicenseStatusRevoked, ExpiresAt: onboardingNow.Add(time.Hour)},
			{ID: "license-status-expired", ProductID: "product-active", PlanID: "plan-active", Status: domain.LicenseStatusExpired, ExpiresAt: onboardingNow.Add(time.Hour)},
			{ID: "license-time-expired", ProductID: "product-active", PlanID: "plan-active", Status: domain.LicenseStatusActive, ExpiresAt: onboardingNow.Add(-time.Minute)},
			{ID: "license-archived-product", ProductID: "product-archived", PlanID: "plan-active", Status: domain.LicenseStatusActive, ExpiresAt: onboardingNow.Add(time.Hour)},
			{ID: "license-archived-plan", ProductID: "product-active", PlanID: "plan-archived", Status: domain.LicenseStatusActive, ExpiresAt: onboardingNow.Add(time.Hour)},
			{ID: "license-mismatched-plan", ProductID: "product-active", PlanID: "plan-other", Status: domain.LicenseStatusActive, ExpiresAt: onboardingNow.Add(time.Hour)},
		},
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
		`"product_count":2`,
		`"plan_count":2`,
		`"license_count":1`,
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
	if console.licenseStatus != "active" {
		t.Fatalf("license status filter = %q, want active", console.licenseStatus)
	}
}

func TestAdminOnboardingProgressRequiresAllReadPermissions(t *testing.T) {
	required := []string{
		domain.PermApplicationsRead,
		domain.PermCredentialsRead,
		domain.PermCatalogRead,
		domain.PermLicensesRead,
	}
	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			account := testOwnerAccount()
			account.Permissions = nil
			for _, permission := range required {
				if permission != missing {
					account.Permissions = append(account.Permissions, permission)
				}
			}
			router := newOnboardingTestRouter(t, &fakeOnboardingConsole{}, account)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, onboardingRequest("/v1/admin/onboarding/progress"))

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("missing %s: status = %d, body = %s", missing, recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"code":"PERMISSION_DENIED"`) {
				t.Fatalf("missing %s: body = %s", missing, recorder.Body.String())
			}
		})
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
