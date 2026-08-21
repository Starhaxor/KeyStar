package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type fakeWebhookConsole struct {
	httpapi.AdminConsoleStore
	created domain.NewWebhook
	appID   string
}

func (fake *fakeWebhookConsole) CreateWebhook(_ context.Context, applicationID string, input domain.NewWebhook, _ []byte) (*domain.Webhook, error) {
	fake.appID, fake.created = applicationID, input
	return &domain.Webhook{ID: "webhook-1", URL: input.URL, Events: input.Events, Status: domain.WebhookStatusActive, CreatedAt: time.Now().UTC()}, nil
}

func (fake *fakeWebhookConsole) AppendAuditLog(context.Context, domain.NewAuditLog) error { return nil }
func (fake *fakeWebhookConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func TestAdminWebhookCreateUsesApplicationAndReturnsOneTimeSecret(t *testing.T) {
	console := &fakeWebhookConsole{}
	auth := &fakeAdminAuth{token: "session-token", account: testOwnerAccount()}
	core := httpapi.NewRouter(httpapi.RouterConfig{DefaultApplicationID: "019c1111-1111-7111-8111-111111111111", Admin: httpapi.AdminConfig{Auth: auth, Console: console, CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour}})
	core.MountAdmin(New(core))
	router := &Router{Router: core}
	request := credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/webhooks", `{"url":"https://example.test/hooks","events":["license.created"]}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.appID != "019c1111-1111-7111-8111-111111111111" || console.created.URL != "https://example.test/hooks" {
		t.Fatalf("created = %#v app = %s", console.created, console.appID)
	}
	var response struct {
		Secret  string `json:"secret"`
		Webhook struct {
			URL string `json:"url"`
		} `json:"webhook"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Secret == "" || response.Webhook.URL != console.created.URL {
		t.Fatalf("response = %#v", response)
	}
}

func TestAdminWebhookCreateRejectsInvalidEvents(t *testing.T) {
	console := &fakeWebhookConsole{}
	auth := &fakeAdminAuth{token: "session-token", account: testOwnerAccount()}
	core := httpapi.NewRouter(httpapi.RouterConfig{Admin: httpapi.AdminConfig{Auth: auth, Console: console, CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour}})
	core.MountAdmin(New(core))
	router := &Router{Router: core}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/webhooks", `{"url":"https://example.test/hooks","events":["unknown.event"]}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
