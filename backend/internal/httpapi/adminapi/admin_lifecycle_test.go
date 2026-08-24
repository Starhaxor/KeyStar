package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type fakeLifecycleConsole struct {
	httpapi.AdminConsoleStore
	auditEntries []domain.NewAuditLog

	application *domain.Application
	product     *domain.Product
	plan        *domain.Plan

	transitionApplicationErr error
	archivePlanErr           error
	createApplicationErr     error

	transitionApplicationID string
	archiveProductAppID     string
	updatePlanAppID         string
	updateApplicationCalls  int
	updateProductCalls      int
	archiveProductCalls     int
	updatePlanCalls         int
	archivePlanCalls        int
}

func (f *fakeLifecycleConsole) AppendAuditLog(_ context.Context, input domain.NewAuditLog) error {
	f.auditEntries = append(f.auditEntries, input)
	return nil
}

func (*fakeLifecycleConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (f *fakeLifecycleConsole) ListApplications(context.Context) ([]domain.Application, error) {
	if f.application == nil {
		return nil, nil
	}
	return []domain.Application{*f.application}, nil
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
	if f.createApplicationErr != nil {
		return nil, f.createApplicationErr
	}
	return f.application, nil
}

func newAdminLifecycleTestRouter(auth *fakeAdminAuth, console *fakeLifecycleConsole) *Router {
	core := httpapi.NewRouter(httpapi.RouterConfig{
		DefaultApplicationID: "019c1111-1111-7111-8111-111111111111",
		Applications:         fakeAdminApplicationResolver{},
		Admin: httpapi.AdminConfig{
			Auth: auth, Console: console, AllowedOrigins: []string{"http://localhost:3000"},
			CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour,
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
