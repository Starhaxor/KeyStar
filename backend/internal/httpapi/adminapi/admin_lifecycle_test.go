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

	transitionApplicationID string
	archiveProductAppID     string
	updatePlanAppID         string
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
	f.updatePlanAppID = applicationID
	updated := *f.plan
	if input.MaxDevices != nil {
		updated.MaxDevices = *input.MaxDevices
	}
	return &updated, nil
}

func (f *fakeLifecycleConsole) ArchivePlan(_ context.Context, _ string, _ string, _ string) (*domain.Plan, error) {
	if f.archivePlanErr != nil {
		return nil, f.archivePlanErr
	}
	updated := *f.plan
	updated.Status = domain.CatalogStatusArchived
	return &updated, nil
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
	if console.archiveProductAppID != "019c1111-1111-7111-8111-111111111111" || len(console.auditEntries) != 1 || console.auditEntries[0].Action != "PRODUCT_ARCHIVED" || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"before"`)) {
		t.Fatalf("application scope = %q, audit entries = %#v", console.archiveProductAppID, console.auditEntries)
	}
}

func TestAdminPlanPatchUsesSelectedApplicationAndAudits(t *testing.T) {
	console := &fakeLifecycleConsole{
		product: &domain.Product{ID: "product-2", ApplicationID: "019c1111-1111-7111-8111-111111111111", Status: domain.CatalogStatusActive},
		plan:    &domain.Plan{ID: "plan-2", ProductID: "product-2", Name: "Pro", Code: "pro", MaxDevices: 2, Status: domain.CatalogStatusActive},
	}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/products/product-2/plans/plan-2", `{"max_devices":5}`))

	var body struct {
		OK   bool        `json:"ok"`
		Plan domain.Plan `json:"plan"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || !body.OK || body.Plan.MaxDevices != 5 {
		t.Fatalf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
	if console.updatePlanAppID != "019c1111-1111-7111-8111-111111111111" || len(console.auditEntries) != 1 || console.auditEntries[0].Action != "PLAN_UPDATED" || !bytes.Contains(console.auditEntries[0].Metadata, []byte(`"after"`)) {
		t.Fatalf("application scope = %q, audit entries = %#v", console.updatePlanAppID, console.auditEntries)
	}
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
