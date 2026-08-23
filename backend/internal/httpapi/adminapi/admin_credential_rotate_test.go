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

type fakeRotateConsole struct {
	httpapi.AdminConsoleStore
	found      *domain.ApplicationCredential
	findID     string
	created    domain.NewApplicationCredential
	expiredID  string
	expiryAt   time.Time
	revokedID  string
	auditTrail []string
}

func (fake *fakeRotateConsole) AppendAuditLog(_ context.Context, input domain.NewAuditLog) error {
	fake.auditTrail = append(fake.auditTrail, input.Action)
	return nil
}

func (fake *fakeRotateConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (fake *fakeRotateConsole) FindCredentialByID(_ context.Context, applicationID, credentialID string) (*domain.ApplicationCredential, error) {
	fake.findID = credentialID
	if fake.found == nil {
		return nil, domain.ErrCredentialNotFound
	}
	return fake.found, nil
}

func (fake *fakeRotateConsole) CreateCredential(_ context.Context, input domain.NewApplicationCredential) (*domain.ApplicationCredential, error) {
	fake.created = input
	return &domain.ApplicationCredential{
		ID: "replacement-1", ApplicationID: input.ApplicationID, Environment: input.Environment,
		CredentialType: input.CredentialType, Name: input.Name, KeyPrefix: input.KeyPrefix,
		KeyHash: input.KeyHash, Scopes: append([]string(nil), input.Scopes...),
		Status: domain.CredentialStatusActive,
	}, nil
}

func (fake *fakeRotateConsole) RevokeCredential(_ context.Context, applicationID, credentialID string) error {
	fake.revokedID = credentialID
	return nil
}

func (fake *fakeRotateConsole) ExpireCredentialAt(_ context.Context, applicationID, credentialID string, expiresAt time.Time) error {
	fake.expiredID = credentialID
	fake.expiryAt = expiresAt
	return nil
}

func TestAdminCredentialRotateWithGracePeriod(t *testing.T) {
	console := &fakeRotateConsole{found: &domain.ApplicationCredential{
		ID: "old-cred", ApplicationID: "app-1", Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialSecret, Name: "CI Backend",
		KeyPrefix: "ks_sk_live_0123456789", KeyHash: []byte("old-hash"),
		Scopes: []string{"users.read"}, Status: domain.CredentialStatusActive,
	}}
	router := newListTestRouter(t, console)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, credentialAdminRequest(t, router,
		http.MethodPost, "/v1/admin/credentials/old-cred/rotate", `{"grace_hours":72}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.findID != "old-cred" || console.expiredID != "old-cred" || console.revokedID != "" {
		t.Fatalf("rotate flow wrong: find=%q expired=%q revoked=%q", console.findID, console.expiredID, console.revokedID)
	}
	if console.created.Name != "CI Backend" || len(console.created.Scopes) != 1 || console.created.Scopes[0] != "users.read" {
		t.Fatalf("replacement config = %#v", console.created)
	}
	if !stringsContains(console.auditTrail, "CREDENTIAL_ROTATED") {
		t.Fatalf("audit trail = %v", console.auditTrail)
	}
	var response struct {
		Key          string `json:"key"`
		OldExpiresAt string `json:"old_expires_at"`
		Credential   struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Credential.ID == "old-cred" || response.Key == "" {
		t.Fatalf("replacement response = %#v", response)
	}
	if response.OldExpiresAt == "" {
		t.Fatal("old_expires_at missing")
	}
}

func TestAdminCredentialRotateImmediateRevokes(t *testing.T) {
	console := &fakeRotateConsole{found: &domain.ApplicationCredential{
		ID: "old-cred", ApplicationID: "app-1", Environment: domain.CredentialEnvironmentTest,
		CredentialType: domain.CredentialPublishable, Name: "Desktop",
		KeyPrefix: "ks_pk_test_0123456789", KeyHash: []byte("old-hash"),
		Scopes: []string{"auth.login"}, Status: domain.CredentialStatusActive,
	}}
	router := newListTestRouter(t, console)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, credentialAdminRequest(t, router,
		http.MethodPost, "/v1/admin/credentials/old-cred/rotate", `{"grace_hours":0}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if console.revokedID != "old-cred" || !console.expiryAt.IsZero() {
		t.Fatalf("immediate rotation did not revoke: revoked=%q expiry=%v", console.revokedID, console.expiryAt)
	}
}

func TestAdminCredentialRotateRejectsBadInput(t *testing.T) {
	console := &fakeRotateConsole{}
	router := newListTestRouter(t, console)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router,
		http.MethodPost, "/v1/admin/credentials/nope/rotate", `{"grace_hours":1}`))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown credential status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router,
		http.MethodPost, "/v1/admin/credentials/old-cred/rotate", `{"grace_hours":9999}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversized grace status = %d", recorder.Code)
	}
}

func stringsContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
