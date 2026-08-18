package adminapi

import (
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

// fakeCredentialConsole embeds the interface so the credential handlers can be
// exercised without reimplementing the whole admin store.
type fakeCredentialConsole struct {
	httpapi.AdminConsoleStore
	credentials  []domain.ApplicationCredential
	created      domain.NewApplicationCredential
	createdAt    time.Time
	revokedApp   string
	revokedID    string
	listApp      string
	auditEntries []domain.NewAuditLog
}

func (fake *fakeCredentialConsole) AppendAuditLog(_ context.Context, input domain.NewAuditLog) error {
	fake.auditEntries = append(fake.auditEntries, input)
	return nil
}

func (fake *fakeCredentialConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (fake *fakeCredentialConsole) ListCredentials(_ context.Context, applicationID string) ([]domain.ApplicationCredential, error) {
	fake.listApp = applicationID
	return fake.credentials, nil
}

func (fake *fakeCredentialConsole) CreateCredential(_ context.Context, input domain.NewApplicationCredential) (*domain.ApplicationCredential, error) {
	fake.created = input
	fake.createdAt = time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	return &domain.ApplicationCredential{
		ID: "cred-1", ApplicationID: input.ApplicationID, Environment: input.Environment,
		CredentialType: input.CredentialType, Name: input.Name, KeyPrefix: input.KeyPrefix,
		KeyHash: input.KeyHash, Scopes: input.Scopes, Status: domain.CredentialStatusActive,
		CreatedAt: fake.createdAt,
	}, nil
}

func (fake *fakeCredentialConsole) RevokeCredential(_ context.Context, applicationID, credentialID string) error {
	fake.revokedApp = applicationID
	fake.revokedID = credentialID
	return nil
}

func newCredentialTestRouter(t *testing.T, console *fakeCredentialConsole) *Router {
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

func credentialAdminRequest(t *testing.T, router *Router, method, path, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: httpapi.AdminSessionCookieName, Value: "session-token"})
	request.Header.Set(httpapi.AdminCSRFHeader, router.AdminCSRFToken("session-token"))
	request.Header.Set("Origin", "http://localhost:3000")
	return request
}

func TestAdminCredentialListReturnsOnlySafeFields(t *testing.T) {
	console := &fakeCredentialConsole{credentials: []domain.ApplicationCredential{
		{
			ID: "cred-1", ApplicationID: "app-1", Environment: domain.CredentialEnvironmentLive,
			CredentialType: domain.CredentialPublishable, Name: "Desktop SDK",
			KeyPrefix: "ks_pk_live_0123456789", KeyHash: []byte("must-not-leak"),
			Scopes: []string{"auth.login"}, Status: domain.CredentialStatusActive,
			CreatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC),
		},
	}}
	router := newCredentialTestRouter(t, console)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodGet, "/v1/admin/credentials", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.listApp != "019c1111-1111-7111-8111-111111111111" {
		t.Fatalf("list application = %q", console.listApp)
	}
	if strings.Contains(recorder.Body.String(), "must-not-leak") {
		t.Fatal("credential list leaked the key hash")
	}
	var response struct {
		OK          bool `json:"ok"`
		Credentials []struct {
			KeyPrefix string   `json:"key_prefix"`
			Type      string   `json:"type"`
			Scopes    []string `json:"scopes"`
			Key       string   `json:"key"`
			KeyHash   string   `json:"key_hash"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || len(response.Credentials) != 1 || response.Credentials[0].KeyPrefix != "ks_pk_live_0123456789" {
		t.Fatalf("response = %#v", response)
	}
	if response.Credentials[0].Key != "" || response.Credentials[0].KeyHash != "" {
		t.Fatalf("list exposed a secret or hash field: %#v", response.Credentials[0])
	}
}

func TestAdminCredentialCreateShowsKeyOnceAndValidates(t *testing.T) {
	console := &fakeCredentialConsole{}
	router := newCredentialTestRouter(t, console)

	body := `{"name":"CI Backend","environment":"live","type":"secret","scopes":["users.read","licenses.write"]}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/credentials", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		OK         bool   `json:"ok"`
		Key        string `json:"key"`
		Credential struct {
			KeyPrefix string `json:"key_prefix"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || !strings.HasPrefix(response.Key, "ks_sk_live_") || !strings.HasPrefix(response.Key, response.Credential.KeyPrefix+"_") {
		t.Fatalf("response = %#v", response)
	}
	if len(response.Key) != len(response.Credential.KeyPrefix)+1+43 {
		t.Fatalf("key %q does not match prefix %q + separator + 43-char secret", response.Key, response.Credential.KeyPrefix)
	}
	if console.created.ApplicationID != "019c1111-1111-7111-8111-111111111111" || console.created.CredentialType != domain.CredentialSecret {
		t.Fatalf("created input = %#v", console.created)
	}

	for _, invalid := range []string{
		`{"name":"","environment":"live","type":"secret","scopes":["users.read"]}`,
		`{"name":"x","environment":"prod","type":"secret","scopes":["users.read"]}`,
		`{"name":"x","environment":"live","type":"admin","scopes":["users.read"]}`,
		`{"name":"x","environment":"live","type":"secret","scopes":["auth.login"]}`,
		`{"name":"x","environment":"live","type":"publishable","scopes":["users.write"]}`,
		`{"name":"x","environment":"live","type":"secret","scopes":[]}`,
		`{"name":"x","environment":"live","type":"secret","scopes":["users.read"],"expires_in":"not-a-duration"}`,
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/credentials", invalid))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid body %q status = %d, body = %s", invalid, recorder.Code, recorder.Body.String())
		}
	}
}

func TestAdminCredentialRevokeRequiresPermissionAndAudits(t *testing.T) {
	console := &fakeCredentialConsole{}
	router := newCredentialTestRouter(t, console)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/credentials/cred-1/revoke", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.revokedID != "cred-1" || console.revokedApp != "019c1111-1111-7111-8111-111111111111" {
		t.Fatalf("revoke args = (%q, %q)", console.revokedApp, console.revokedID)
	}
	if len(console.auditEntries) == 0 || console.auditEntries[len(console.auditEntries)-1].Action != "CREDENTIAL_REVOKED" {
		t.Fatalf("audit entries = %#v", console.auditEntries)
	}

	// A viewer without credentials permissions is denied before the store is hit.
	viewer := &fakeCredentialConsole{}
	auth := &fakeAdminAuth{token: "session-token", account: &domain.AdminAccount{
		ID: "viewer-id", Status: domain.AdminStatusActive, MFAEnrolled: true, Permissions: []string{domain.PermUsersRead},
	}}
	router = &Router{Router: httpapi.NewRouter(httpapi.RouterConfig{
		Admin: httpapi.AdminConfig{Auth: auth, Console: viewer, CSRFSecret: []byte("test-csrf-secret"), SessionTTL: time.Hour},
	})}
	router.MountAdmin(New(router.Router))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodGet, "/v1/admin/credentials", ""))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want 403", recorder.Code)
	}
}
