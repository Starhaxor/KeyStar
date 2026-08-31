package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type fakeLifecycleConsole struct {
	httpapi.AdminConsoleStore
	auditEntries []domain.NewAuditLog

	application  *domain.Application
	applications []domain.Application
	product      *domain.Product
	plan         *domain.Plan
	signingKeys  []domain.ApplicationSigningKey

	transitionApplicationErr error
	archivePlanErr           error
	createApplicationErr     error

	transitionApplicationID   string
	signingKeyApplicationID   string
	archiveProductAppID       string
	updatePlanAppID           string
	createApplicationCalls    int
	provisionApplicationCalls int
	provisionInput            domain.NewApplication
	updateApplicationCalls    int
	updateProductCalls        int
	archiveProductCalls       int
	updatePlanCalls           int
	archivePlanCalls          int
}

func (f *fakeLifecycleConsole) AppendAuditLog(_ context.Context, input domain.NewAuditLog) error {
	f.auditEntries = append(f.auditEntries, input)
	return nil
}

func (*fakeLifecycleConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (f *fakeLifecycleConsole) ListApplications(context.Context) ([]domain.Application, error) {
	if f.applications != nil {
		return f.applications, nil
	}
	if f.application == nil {
		return nil, nil
	}
	return []domain.Application{*f.application}, nil
}

func (*fakeLifecycleConsole) ListOrganizations(context.Context) ([]domain.Organization, error) {
	return []domain.Organization{{ID: "org-1", Name: "Default", Slug: "default"}}, nil
}

func (f *fakeLifecycleConsole) UpdateApplication(_ context.Context, _ string, input domain.UpdateApplication) (*domain.Application, error) {
	f.updateApplicationCalls++
	updated := *f.application
	if input.Name != nil {
		updated.Name = *input.Name
	}
	if input.Slug != nil {
		updated.Slug = *input.Slug
	}
	return &updated, nil
}

func (f *fakeLifecycleConsole) TransitionApplication(_ context.Context, applicationID string, status domain.ApplicationStatus) (*domain.Application, error) {
	f.transitionApplicationID = applicationID
	if f.transitionApplicationErr != nil {
		return nil, f.transitionApplicationErr
	}
	updated := *f.application
	updated.Status = status
	return &updated, nil
}

func (f *fakeLifecycleConsole) FindProductByID(_ context.Context, applicationID, _ string) (*domain.Product, error) {
	if f.product == nil || f.product.ApplicationID != applicationID {
		return nil, domain.ErrProductNotFound
	}
	return f.product, nil
}

func (f *fakeLifecycleConsole) UpdateProduct(_ context.Context, applicationID, _ string, input domain.UpdateProduct) (*domain.Product, error) {
	f.updateProductCalls++
	updated := *f.product
	if input.Name != nil {
		updated.Name = *input.Name
	}
	if input.Status != nil {
		updated.Status = *input.Status
	}
	if applicationID != updated.ApplicationID {
		return nil, domain.ErrProductNotFound
	}
	return &updated, nil
}

func (f *fakeLifecycleConsole) ArchiveProduct(_ context.Context, applicationID, _ string) (*domain.Product, error) {
	f.archiveProductCalls++
	f.archiveProductAppID = applicationID
	updated := *f.product
	updated.Status = domain.CatalogStatusArchived
	return &updated, nil
}

func (f *fakeLifecycleConsole) ListPlans(_ context.Context, productID string) ([]domain.Plan, error) {
	if f.plan == nil || f.plan.ProductID != productID {
		return nil, nil
	}
	return []domain.Plan{*f.plan}, nil
}

func (f *fakeLifecycleConsole) UpdatePlan(_ context.Context, applicationID, _ string, _ string, input domain.UpdatePlan) (*domain.Plan, error) {
	f.updatePlanCalls++
	f.updatePlanAppID = applicationID
	updated := *f.plan
	if input.MaxDevices != nil {
		updated.MaxDevices = *input.MaxDevices
	}
	return &updated, nil
}

func (f *fakeLifecycleConsole) ArchivePlan(_ context.Context, _ string, _ string, _ string) (*domain.Plan, error) {
	f.archivePlanCalls++
	if f.archivePlanErr != nil {
		return nil, f.archivePlanErr
	}
	updated := *f.plan
	updated.Status = domain.CatalogStatusArchived
	return &updated, nil
}

func (f *fakeLifecycleConsole) CreateApplication(_ context.Context, _ domain.NewApplication) (*domain.Application, error) {
	f.createApplicationCalls++
	if f.createApplicationErr != nil {
		return nil, f.createApplicationErr
	}
	return f.application, nil
}

func (f *fakeLifecycleConsole) Create(_ context.Context, input domain.NewApplication) (*domain.Application, error) {
	f.provisionApplicationCalls++
	f.provisionInput = input
	if f.createApplicationErr != nil {
		return nil, f.createApplicationErr
	}
	return f.application, nil
}

func (f *fakeLifecycleConsole) ListApplicationSigningKeys(_ context.Context, applicationID string) ([]domain.ApplicationSigningKey, error) {
	f.signingKeyApplicationID = applicationID
	return f.signingKeys, nil
}

func newAdminLifecycleTestRouter(auth *fakeAdminAuth, console *fakeLifecycleConsole) *Router {
	return newAdminLifecycleTestRouterWithResolver(auth, console, fakeAdminApplicationResolver{})
}

type lifecycleStatusResolver struct {
	status domain.ApplicationStatus
}

func (resolver lifecycleStatusResolver) FindApplicationByID(_ context.Context, applicationID string) (*domain.Application, error) {
	return &domain.Application{ID: applicationID, OrganizationID: "org-id", Status: resolver.status}, nil
}

func newAdminLifecycleTestRouterWithResolver(auth *fakeAdminAuth, console *fakeLifecycleConsole, resolver httpapi.ApplicationResolver) *Router {
	core := httpapi.NewRouter(httpapi.RouterConfig{
		DefaultApplicationID: "019c1111-1111-7111-8111-111111111111",
		Applications:         resolver,
		Admin: httpapi.AdminConfig{
			Auth: auth, Console: console, AllowedOrigins: []string{"http://localhost:3000"},
			CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour,
			ApplicationProvisioner: console, ApplicationSigningKeys: console,
		},
	})
	return &Router{Router: core}
}

func lifecycleRequest(t *testing.T, router *Router, method, path string, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: httpapi.AdminSessionCookieName, Value: "session-token"})
	request.Header.Set(httpapi.AdminCSRFHeader, router.AdminCSRFToken("session-token"))
	return request
}

func lifecycleRequestForApplication(t *testing.T, router *Router, method, path, body, applicationID string, includeCSRF bool) *http.Request {
	t.Helper()
	request := lifecycleRequest(t, router, method, path, body)
	request.Header.Set("X-KeyStar-App", applicationID)
	if !includeCSRF {
		request.Header.Del(httpapi.AdminCSRFHeader)
	}
	return request
}

func assertLifecycleAudit(t *testing.T, console *fakeLifecycleConsole, action, resourceID string) {
	t.Helper()
	if len(console.auditEntries) != 1 || console.auditEntries[0].Action != action || console.auditEntries[0].ResourceID != resourceID || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"before"`)) || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"after"`)) {
		t.Fatalf("audit entries = %#v", console.auditEntries)
	}
}

func TestAdminApplicationCreatePreservesLegacyErrorContract(t *testing.T) {
	console := &fakeLifecycleConsole{createApplicationErr: domain.ErrApplicationExists}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications", `{"organization_id":"org-1","name":"Portal"}`))

	var body struct{ Code string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusInternalServerError || body.Code != "SERVER_ERROR" {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
}

func TestAdminApplicationCreationUsesSigningKeyProvisioner(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", OrganizationID: "org-1", Name: "Portal", Slug: "portal", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications", `{"organization_id":"org-1","name":"Portal","slug":"portal"}`))

	if recorder.Code != http.StatusCreated || console.provisionApplicationCalls != 1 || console.createApplicationCalls != 0 {
		t.Fatalf("status = %d, provision calls = %d, console create calls = %d, body = %s", recorder.Code, console.provisionApplicationCalls, console.createApplicationCalls, recorder.Body.String())
	}
	if console.provisionInput.OrganizationID != "org-1" || console.provisionInput.Name != "Portal" || console.provisionInput.Slug != "portal" {
		t.Fatalf("provision input = %#v", console.provisionInput)
	}
}

func TestAdminApplicationSigningKeysRequireApplicationsRead(t *testing.T) {
	account := testOwnerAccount()
	account.Permissions = []string{domain.PermCatalogRead}
	console := &fakeLifecycleConsole{applications: []domain.Application{{ID: "application-2", Status: domain.ApplicationStatusActive}}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: account}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodGet, "/v1/admin/applications/application-2/signing-keys", ""))

	if recorder.Code != http.StatusForbidden || console.signingKeyApplicationID != "" {
		t.Fatalf("status = %d, signing-key application = %q, body = %s", recorder.Code, console.signingKeyApplicationID, recorder.Body.String())
	}
}

func TestAdminApplicationSigningKeysUseRequestedApplicationAndExposeOnlyPublicMetadata(t *testing.T) {
	createdAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	activatedAt := createdAt
	publicKey := make([]byte, 32)
	console := &fakeLifecycleConsole{
		applications: []domain.Application{
			{ID: "019c1111-1111-7111-8111-111111111111", Status: domain.ApplicationStatusActive},
			{ID: "019c2222-2222-7222-8222-222222222222", Status: domain.ApplicationStatusActive},
		},
		signingKeys: []domain.ApplicationSigningKey{{
			ID: "internal-key-id", KID: "ksk_AAAAAAAAAAAAAAAAAAAAAA", ApplicationID: "019c2222-2222-7222-8222-222222222222",
			Algorithm: "Ed25519", PublicKey: publicKey, EncryptedPrivateKey: []byte("ciphertext"), EncryptionNonce: []byte("secret-nonce"),
			EncryptionKeyVersion: 7, Status: domain.ApplicationSigningKeyActive, CreatedAt: createdAt, ActivatedAt: &activatedAt,
		}},
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodGet, "/v1/admin/applications/019c2222-2222-7222-8222-222222222222/signing-keys", ""))

	if recorder.Code != http.StatusOK || console.signingKeyApplicationID != "019c2222-2222-7222-8222-222222222222" {
		t.Fatalf("status = %d, signing-key application = %q, body = %s", recorder.Code, console.signingKeyApplicationID, recorder.Body.String())
	}
	var body struct {
		OK    bool             `json:"ok"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	var topLevel map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &topLevel); err != nil || len(topLevel) != 2 {
		t.Fatalf("top-level fields = %#v, err = %v", topLevel, err)
	}
	if !body.OK || len(body.Items) != 1 {
		t.Fatalf("response = %#v", body)
	}
	want := map[string]any{
		"kid": "ksk_AAAAAAAAAAAAAAAAAAAAAA", "algorithm": "Ed25519", "status": "active",
		"public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "created_at": "2026-08-31T12:00:00Z",
		"activated_at": "2026-08-31T12:00:00Z", "retire_at": nil, "revoked_at": nil,
	}
	if len(body.Items[0]) != len(want) {
		t.Fatalf("public metadata fields = %#v", body.Items[0])
	}
	for key, wantValue := range want {
		if gotValue, exists := body.Items[0][key]; !exists || gotValue != wantValue {
			t.Fatalf("field %q = %#v, want %#v (item = %#v)", key, gotValue, wantValue, body.Items[0])
		}
	}
	response := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"ciphertext", "encrypted", "nonce", "encryption_key_version", "private", "seed"} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("response contains %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestAdminApplicationSigningKeysRemainReadableWhenApplicationDisabled(t *testing.T) {
	const applicationID = "019c1111-1111-7111-8111-111111111111"
	console := &fakeLifecycleConsole{
		applications: []domain.Application{{ID: applicationID, Status: domain.ApplicationStatusDisabled}},
		signingKeys:  []domain.ApplicationSigningKey{{KID: "ksk_AAAAAAAAAAAAAAAAAAAAAA", ApplicationID: applicationID, Algorithm: "Ed25519", Status: domain.ApplicationSigningKeyActive}},
	}
	router := newAdminLifecycleTestRouterWithResolver(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console, lifecycleStatusResolver{status: domain.ApplicationStatusDisabled})

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodGet, "/v1/admin/applications/"+applicationID+"/signing-keys", ""))

	if recorder.Code != http.StatusOK || console.signingKeyApplicationID != applicationID {
		t.Fatalf("status = %d, signing-key application = %q, body = %s", recorder.Code, console.signingKeyApplicationID, recorder.Body.String())
	}
}

func TestAdminApplicationTransitionRequiresApplicationsWrite(t *testing.T) {
	account := testOwnerAccount()
	account.Permissions = []string{domain.PermCatalogWrite}
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", Name: "Portal", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: account}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications/application-2/transition", `{"status":"maintenance"}`))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.transitionApplicationID != "" {
		t.Fatal("application transition ran without applications.write")
	}
}

func TestAdminApplicationTransitionMapsDependencyConflict(t *testing.T) {
	console := &fakeLifecycleConsole{
		application:              &domain.Application{ID: "application-2", Name: "Portal", Status: domain.ApplicationStatusActive},
		transitionApplicationErr: domain.ErrApplicationInUse,
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications/application-2/transition", `{"status":"disabled"}`))

	var body struct{ Code, Message string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusConflict || body.Code != "APPLICATION_IN_USE" || body.Message != "application has active dependent records" {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
}

func TestAdminApplicationPatchReturnsUpdatedPayloadAndAuditState(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", OrganizationID: "org-1", Name: "Portal", Slug: "portal", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/applications/application-2", `{"name":"New Portal"}`))

	var body struct {
		OK          bool            `json:"ok"`
		Application applicationJSON `json:"application"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || !body.OK || body.Application.Name != "New Portal" {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
	if len(console.auditEntries) != 1 || console.auditEntries[0].Action != "APPLICATION_UPDATED" || console.auditEntries[0].ResourceID != "application-2" || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"before"`)) || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"after"`)) {
		t.Fatalf("audit entries = %#v", console.auditEntries)
	}
}

func TestAdminProductArchiveUsesSelectedApplicationAndAudits(t *testing.T) {
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Name: "Pro", Slug: "pro", Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/products/product-2/archive", ""))

	var body struct {
		OK      bool           `json:"ok"`
		Product domain.Product `json:"product"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || !body.OK || body.Product.Status != domain.CatalogStatusArchived {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
	if console.archiveProductAppID != "019c1111-1111-7111-8111-111111111111" {
		t.Fatalf("application scope = %q, audit entries = %#v", console.archiveProductAppID, console.auditEntries)
	}
	assertLifecycleAudit(t, console, "PRODUCT_ARCHIVED", "product-2")
}

func TestAdminPlanPatchUsesSelectedApplicationAndAudits(t *testing.T) {
	const applicationID = "019c3333-3333-7333-8333-333333333333"
	console := &fakeLifecycleConsole{
		product: &domain.Product{ID: "product-2", ApplicationID: applicationID, Status: domain.CatalogStatusActive},
		plan:    &domain.Plan{ID: "plan-2", ProductID: "product-2", Name: "Pro", Code: "pro", MaxDevices: 2, Status: domain.CatalogStatusActive},
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequestForApplication(t, router, http.MethodPatch, "/v1/admin/products/product-2/plans/plan-2", `{"max_devices":5}`, applicationID, true))

	var body struct {
		OK   bool        `json:"ok"`
		Plan domain.Plan `json:"plan"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || !body.OK || body.Plan.MaxDevices != 5 {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
	if console.updatePlanAppID != applicationID {
		t.Fatalf("application scope = %q, audit entries = %#v", console.updatePlanAppID, console.auditEntries)
	}
	assertLifecycleAudit(t, console, "PLAN_UPDATED", "plan-2")
}

func TestAdminPlanArchiveMapsConflict(t *testing.T) {
	console := &fakeLifecycleConsole{
		product:        &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Status: domain.CatalogStatusActive},
		plan:           &domain.Plan{ID: "plan-2", ProductID: "product-2", Status: domain.CatalogStatusActive},
		archivePlanErr: domain.ErrCatalogRecordInUse,
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/products/product-2/plans/plan-2/archive", ""))

	var body struct{ Code string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusConflict || body.Code != "CATALOG_RECORD_IN_USE" {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
}

func TestAdminCatalogLifecycleRequiresCatalogWrite(t *testing.T) {
	account := testOwnerAccount()
	account.Permissions = []string{domain.PermApplicationsWrite}
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: account}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/products/product-2/archive", ""))

	if recorder.Code != http.StatusForbidden || console.archiveProductCalls != 0 {
		t.Fatalf("status = %d, archive calls = %d, body = %s", recorder.Code, console.archiveProductCalls, recorder.Body.String())
	}
}

func TestAdminProductArchiveUsesExplicitSelectedApplication(t *testing.T) {
	const applicationID = "019c2222-2222-7222-8222-222222222222"
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: applicationID, Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequestForApplication(t, router, http.MethodPost, "/v1/admin/products/product-2/archive", "", applicationID, true))

	if recorder.Code != http.StatusOK || console.archiveProductAppID != applicationID {
		t.Fatalf("status = %d, application scope = %q, body = %s", recorder.Code, console.archiveProductAppID, recorder.Body.String())
	}
}

func TestAdminProductLifecycleRejectsCrossApplicationProduct(t *testing.T) {
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: "019c2222-2222-7222-8222-222222222222", Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/products/product-2", `{"name":"Other"}`))

	var body struct{ Code string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusNotFound || body.Code != "PRODUCT_NOT_FOUND" || console.updateProductCalls != 0 {
		t.Fatalf("status = %d, code = %q, update calls = %d, body = %s, err = %v", recorder.Code, body.Code, console.updateProductCalls, recorder.Body.String(), err)
	}
}

func TestAdminPlanLifecycleRejectsCrossApplicationPlan(t *testing.T) {
	console := &fakeLifecycleConsole{
		product: &domain.Product{ID: "product-2", ApplicationID: "019c2222-2222-7222-8222-222222222222", Status: domain.CatalogStatusActive},
		plan:    &domain.Plan{ID: "plan-2", ProductID: "product-2", Status: domain.CatalogStatusActive},
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/products/product-2/plans/plan-2", `{"max_devices":5}`))

	var body struct{ Code string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusNotFound || body.Code != "PRODUCT_NOT_FOUND" || console.updatePlanCalls != 0 {
		t.Fatalf("status = %d, code = %q, update calls = %d, body = %s, err = %v", recorder.Code, body.Code, console.updatePlanCalls, recorder.Body.String(), err)
	}
}

func TestAdminLifecycleMutationRejectsMissingCSRFBeforeService(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequestForApplication(t, router, http.MethodPatch, "/v1/admin/applications/application-2", `{"name":"Other"}`, "", false))

	var body struct{ Code string }
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusForbidden || body.Code != "CSRF_REJECTED" || console.updateApplicationCalls != 0 {
		t.Fatalf("status = %d, code = %q, update calls = %d, body = %s, err = %v", recorder.Code, body.Code, console.updateApplicationCalls, recorder.Body.String(), err)
	}
}

func TestAdminApplicationTransitionAuditsSuccess(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)
	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications/application-2/transition", `{"status":"maintenance"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertLifecycleAudit(t, console, "APPLICATION_TRANSITIONED", "application-2")
}

func TestAdminApplicationListRemainsReachableWhenDefaultApplicationDisabled(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "019c1111-1111-7111-8111-111111111111", Status: domain.ApplicationStatusDisabled}}
	router := newAdminLifecycleTestRouterWithResolver(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console, lifecycleStatusResolver{status: domain.ApplicationStatusDisabled})
	recorder := httptest.NewRecorder()

	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodGet, "/v1/admin/applications", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminRecoveryReadsBypassDisabledDefaultApplicationResolution(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "019c1111-1111-7111-8111-111111111111", Status: domain.ApplicationStatusDisabled}}
	router := newAdminLifecycleTestRouterWithResolver(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console, lifecycleStatusResolver{status: domain.ApplicationStatusDisabled})
	for _, path := range []string{"/v1/admin/me", "/v1/admin/applications", "/v1/admin/applications/organizations"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodGet, path, ""))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminApplicationTransitionCanRestoreDisabledDefaultApplication(t *testing.T) {
	const defaultApplicationID = "019c1111-1111-7111-8111-111111111111"
	console := &fakeLifecycleConsole{application: &domain.Application{ID: defaultApplicationID, Status: domain.ApplicationStatusDisabled}}
	router := newAdminLifecycleTestRouterWithResolver(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console, lifecycleStatusResolver{status: domain.ApplicationStatusDisabled})
	recorder := httptest.NewRecorder()

	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/applications/"+defaultApplicationID+"/transition", `{"status":"active"}`))

	if recorder.Code != http.StatusOK || console.transitionApplicationID != defaultApplicationID {
		t.Fatalf("status = %d, transition application = %q, body = %s", recorder.Code, console.transitionApplicationID, recorder.Body.String())
	}
}

func TestAdminDisabledDefaultStillBlocksCatalogMutation(t *testing.T) {
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouterWithResolver(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console, lifecycleStatusResolver{status: domain.ApplicationStatusDisabled})
	recorder := httptest.NewRecorder()

	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/products/product-2/archive", ""))

	if recorder.Code != http.StatusForbidden || console.archiveProductCalls != 0 {
		t.Fatalf("status = %d, archive calls = %d, body = %s", recorder.Code, console.archiveProductCalls, recorder.Body.String())
	}
}

func TestAuditLogResponsesSanitizeMetadataValuesAndKeepSafeScalars(t *testing.T) {
	items := mapAuditEntries([]domain.AuditLog{{
		Metadata: json.RawMessage(`{"email":"owner@example.test","role":"owner","devices":2,"revoked":1,"user_email":"user@example.test","before":{"name":"Portal","slug":"portal","status":"active","api_key":"secret","code":{"leak":"secret"}},"after":{"status":"disabled","max_devices":3,"token":"secret","name":"Bearer secret"},"password":"secret"}`),
	}})
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	var metadata map[string]any
	if err := json.Unmarshal(items[0].Metadata, &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	before := metadata["before"].(map[string]any)
	after := metadata["after"].(map[string]any)
	if before["name"] != "Portal" || before["slug"] != "portal" || after["status"] != "disabled" || after["max_devices"] != float64(3) {
		t.Fatalf("safe lifecycle metadata = %#v", metadata)
	}
	if metadata["email"] != "owner@example.test" || metadata["role"] != "owner" || metadata["devices"] != float64(2) || metadata["revoked"] != float64(1) || metadata["user_email"] != "user@example.test" {
		t.Fatalf("safe scalar metadata = %#v", metadata)
	}
	if _, exists := metadata["password"]; exists {
		t.Fatalf("metadata leaked top-level secret: %#v", metadata)
	}
	if _, exists := before["api_key"]; exists {
		t.Fatalf("metadata leaked before secret: %#v", metadata)
	}
	if _, exists := before["code"]; exists {
		t.Fatalf("metadata leaked structured code value: %#v", metadata)
	}
	if _, exists := after["token"]; exists {
		t.Fatalf("metadata leaked after secret: %#v", metadata)
	}
	if _, exists := after["name"]; exists {
		t.Fatalf("metadata leaked unsafe formatted name: %#v", metadata)
	}
}

func TestAdminProductPatchAuditsSuccess(t *testing.T) {
	console := &fakeLifecycleConsole{product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Name: "Pro", Status: domain.CatalogStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)
	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/products/product-2", `{"name":"Other"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertLifecycleAudit(t, console, "PRODUCT_UPDATED", "product-2")
}

func TestAdminPlanArchiveAuditsSuccess(t *testing.T) {
	console := &fakeLifecycleConsole{
		product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Status: domain.CatalogStatusActive},
		plan:    &domain.Plan{ID: "plan-2", ProductID: "product-2", Status: domain.CatalogStatusActive},
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)
	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPost, "/v1/admin/products/product-2/plans/plan-2/archive", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertLifecycleAudit(t, console, "PLAN_ARCHIVED", "plan-2")
}
