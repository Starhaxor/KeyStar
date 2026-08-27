package serverapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

func TestServerWebhookCreateReturnsRandomEncodedSecretAndRejectsPrivateTargets(t *testing.T) {
	store := &fakeServerStore{}
	router := newServerTestRouter(store, serverSecretKey())
	first := serverRequest(t, router, http.MethodPost, "/v1/server/webhooks", `{"url":"https://hooks.example.com/events","events":["license.created"]}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResponse struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResponse); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(firstResponse.Secret)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("secret is not 32-byte base64url: %q (%v)", firstResponse.Secret, err)
	}
	if !bytes.Equal(store.webhookSecretHash, domain.HashWebhookSecret([]byte(firstResponse.Secret))) {
		t.Fatal("stored hash does not match the returned secret")
	}
	second := serverRequest(t, router, http.MethodPost, "/v1/server/webhooks", `{"url":"https://hooks.example.com/events","events":["license.created"]}`)
	var secondResponse struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResponse); err != nil {
		t.Fatal(err)
	}
	if firstResponse.Secret == secondResponse.Secret {
		t.Fatal("webhook secrets were predictable/reused")
	}

	blocked := serverRequest(t, router, http.MethodPost, "/v1/server/webhooks", `{"url":"https://127.0.0.1/internal","events":["license.created"]}`)
	if blocked.Code != http.StatusBadRequest {
		t.Fatalf("private target status=%d body=%s", blocked.Code, blocked.Body.String())
	}
}

func TestServerSessionRevocationPassesTenantBoundary(t *testing.T) {
	store := &fakeServerStore{}
	router := newServerTestRouter(store, serverSecretKey())
	sessionID := "019c2222-2222-7222-8222-222222222222"
	response := serverRequest(t, router, http.MethodPost, "/v1/server/sessions/"+sessionID+"/revoke", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if store.revokeApplication != serverTestApplicationID || store.revokeSession != sessionID {
		t.Fatalf("revoke args=(%q,%q)", store.revokeApplication, store.revokeSession)
	}
}

const serverTestApplicationID = "019c1111-1111-7111-8111-111111111111"

// serverTestApplicationResolver accepts every well-formed application ID.
type serverTestApplicationResolver struct{}

func (resolver *serverTestApplicationResolver) FindApplicationByID(_ context.Context, applicationID string) (*domain.Application, error) {
	if applicationID != serverTestApplicationID {
		return nil, domain.ErrApplicationNotFound
	}
	return &domain.Application{ID: applicationID, OrganizationID: "org-1", Status: domain.ApplicationStatusActive}, nil
}

// serverTestCredentialVerifier accepts the configured credential for the test
// application.
type serverTestCredentialVerifier struct {
	credential *domain.ApplicationCredential
	err        error
}

func (verifier *serverTestCredentialVerifier) Verify(_ context.Context, applicationID, _ string) (*domain.ApplicationCredential, error) {
	if verifier.err != nil {
		return nil, verifier.err
	}
	if verifier.credential == nil || verifier.credential.ApplicationID != applicationID {
		return nil, domain.ErrInvalidCredential
	}
	return verifier.credential, nil
}

// fakeServerStore embeds the interface so only the exercised methods need to
// be implemented.
type fakeServerStore struct {
	httpapi.ServerStore
	users                []domain.ServerUser
	user                 *domain.ServerUser
	userErr              error
	createdUser          *domain.User
	licenses             []domain.ServerLicense
	license              *domain.ServerLicense
	licenseErr           error
	created              *domain.License
	variables            []domain.Variable
	variable             *domain.Variable
	domainPolicy         *domain.DevicePolicy
	notes                string
	status               domain.UserStatus
	banReason            string
	banExpires           *time.Time
	unbanned             string
	resetUser            string
	resetCount           int64
	revoked              string
	updated              string
	deleted              string
	createCalls          int
	webhookSecretHash    []byte
	webhookURL           string
	revokeApplication    string
	revokeSession        string
	revokeAllApplication string
	revokeAllUser        string
}

func (fake *fakeServerStore) ListServerUsers(_ context.Context, _, _ string, _ int) ([]domain.ServerUser, string, bool, error) {
	return fake.users, "", len(fake.users) > 1, nil
}

func (fake *fakeServerStore) FindServerUserByID(context.Context, string, string) (*domain.ServerUser, error) {
	return fake.user, fake.userErr
}

func (fake *fakeServerStore) FindUserByID(_ context.Context, _ string, userID string) (*domain.User, error) {
	if fake.user == nil {
		return nil, domain.ErrUserNotFound
	}
	return &domain.User{ID: userID, Email: fake.user.Email, Status: domain.UserStatusActive}, nil
}

func (fake *fakeServerStore) FindUserByEmail(_ context.Context, _ string, email string) (*domain.User, error) {
	if fake.user == nil {
		return nil, domain.ErrUserNotFound
	}
	return &domain.User{ID: fake.user.ID, Email: email, Status: domain.UserStatusActive}, nil
}

func (fake *fakeServerStore) CreateUser(_ context.Context, applicationID string, input domain.NewUser) (*domain.User, error) {
	if fake.createdUser != nil {
		return fake.createdUser, nil
	}
	fake.createCalls++
	return &domain.User{ID: "user-new", ApplicationID: applicationID, Email: input.Email, Status: domain.UserStatusActive, CreatedAt: time.Now()}, nil
}

func (fake *fakeServerStore) SetUserStatus(_ context.Context, _, _ string, status domain.UserStatus) error {
	fake.status = status
	return nil
}

func (fake *fakeServerStore) SetUserNotes(_ context.Context, _, _, notes string) error {
	fake.notes = notes
	return nil
}

func (fake *fakeServerStore) BanUser(_ context.Context, _, _, reason string, expiresAt *time.Time) error {
	fake.banReason = reason
	fake.banExpires = expiresAt
	return nil
}

func (fake *fakeServerStore) UnbanUser(_ context.Context, _, userID string) error {
	fake.unbanned = userID
	return nil
}

func (fake *fakeServerStore) ResetUserDevices(_ context.Context, _, userID string) (int64, error) {
	fake.resetUser = userID
	return fake.resetCount, nil
}

func (fake *fakeServerStore) ListServerLicenses(_ context.Context, _, _ string, _ int) ([]domain.ServerLicense, string, bool, error) {
	return fake.licenses, "", false, nil
}

func (fake *fakeServerStore) FindServerLicenseByID(context.Context, string, string) (*domain.ServerLicense, error) {
	return fake.license, fake.licenseErr
}

func (fake *fakeServerStore) ResolveProductPlan(_ context.Context, _, name string) (string, string, error) {
	return "product-1", "plan-1", nil
}

func (fake *fakeServerStore) CreateLicense(_ context.Context, applicationID string, input domain.NewLicense) (*domain.License, error) {
	if fake.created != nil {
		return fake.created, nil
	}
	return &domain.License{ID: "license-new", ApplicationID: applicationID, ProductID: input.ProductID, PlanID: input.PlanID, Product: "StarLoader", ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
}

func (fake *fakeServerStore) AdminUpdateLicense(_ context.Context, _, licenseID string, expiresAt time.Time, _, _ int, _ string) error {
	fake.updated = licenseID
	return nil
}

func (fake *fakeServerStore) AdminRevokeLicense(_ context.Context, _, licenseID string) error {
	fake.revoked = licenseID
	return nil
}

func (fake *fakeServerStore) ListVariables(_ context.Context, _ string) ([]domain.Variable, error) {
	return fake.variables, nil
}

func (fake *fakeServerStore) CreateVariable(_ context.Context, _, key, value, description string) (*domain.Variable, error) {
	if fake.variable != nil {
		return fake.variable, nil
	}
	return &domain.Variable{ID: "variable-new", Key: key, Value: value, Description: description, CreatedAt: time.Now()}, nil
}

func (fake *fakeServerStore) UpdateVariable(_ context.Context, _, variableID, _, _ string) error {
	fake.updated = variableID
	return nil
}

func (fake *fakeServerStore) DeleteVariable(_ context.Context, _, variableID string) error {
	fake.deleted = variableID
	return nil
}

func (fake *fakeServerStore) ListRefreshSessions(_ context.Context, _, _ string, _ string, _ int) ([]domain.RefreshSession, string, bool, error) {
	return nil, "", false, nil
}

func (fake *fakeServerStore) RevokeRefreshSession(_ context.Context, applicationID, sessionID string) error {
	fake.revokeApplication, fake.revokeSession = applicationID, sessionID
	return nil
}

func (fake *fakeServerStore) RevokeAllUserRefreshSessions(_ context.Context, applicationID, userID string) (int64, error) {
	fake.revokeAllApplication, fake.revokeAllUser = applicationID, userID
	return 0, nil
}

func (fake *fakeServerStore) CreateWebhook(_ context.Context, applicationID string, input domain.NewWebhook, secretHash []byte) (*domain.Webhook, error) {
	fake.webhookSecretHash = append([]byte(nil), secretHash...)
	fake.webhookURL = input.URL
	return &domain.Webhook{ID: "wh-1", ApplicationID: applicationID, URL: input.URL, Status: domain.WebhookStatusActive, Events: input.Events}, nil
}

func (fake *fakeServerStore) ListWebhooks(_ context.Context, _ string) ([]domain.Webhook, error) {
	return nil, nil
}

func (fake *fakeServerStore) FindWebhookByID(_ context.Context, _, _ string) (*domain.Webhook, error) {
	return &domain.Webhook{ID: "wh-1", Status: domain.WebhookStatusActive, Events: []string{"*"}}, nil
}

func (fake *fakeServerStore) UpdateWebhook(_ context.Context, _, _ string, _ *string, _ *domain.WebhookStatus, _ *[]string) error {
	return nil
}

func (fake *fakeServerStore) DeleteWebhook(_ context.Context, _, _ string) error {
	return nil
}

func (fake *fakeServerStore) GetDevicePolicy(_ context.Context, applicationID string) (*domain.DevicePolicy, error) {
	if fake.domainPolicy != nil {
		return fake.domainPolicy, nil
	}
	return domain.DefaultDevicePolicy(applicationID), nil
}

func (fake *fakeServerStore) UpsertDevicePolicy(_ context.Context, applicationID string, input domain.NewDevicePolicy) (*domain.DevicePolicy, error) {
	return &domain.DevicePolicy{
		ID:                     "policy-1",
		ApplicationID:          applicationID,
		TPMPolicy:              input.TPMPolicy,
		MinMatchScore:          input.MinMatchScore,
		StepUpScore:            input.StepUpScore,
		AllowAutoRebind:        input.AllowAutoRebind,
		RebindCooldownSeconds:  input.RebindCooldownSeconds,
		MaxDeviceChangesPer30d: input.MaxDeviceChangesPer30d,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}, nil
}

func newServerTestRouter(store *fakeServerStore, credential *domain.ApplicationCredential) *Router {
	verifier := &serverTestCredentialVerifier{credential: credential}
	core := httpapi.NewRouter(httpapi.RouterConfig{
		DefaultApplicationID: serverTestApplicationID,
		Applications:         &serverTestApplicationResolver{},
		Credentials:          verifier,
		Server:               httpapi.ServerConfig{LicenseHMACKey: []byte("license-hmac-key"), Product: "StarLoader"},
		ServerStore:          store,
	})
	core.MountServer(New(core))
	return &Router{Router: core}
}

func serverSecretKey() *domain.ApplicationCredential {
	return &domain.ApplicationCredential{
		ID: "cred-secret", ApplicationID: serverTestApplicationID,
		Environment: domain.CredentialEnvironmentLive, CredentialType: domain.CredentialSecret,
		Scopes: []string{"users.read", "users.write", "licenses.read", "licenses.write", "devices.read", "devices.write", "variables.read", "variables.write", "sessions.read", "sessions.revoke", "webhooks.read", "webhooks.write"},
		Status: domain.CredentialStatusActive,
	}
}

func serverRequest(t *testing.T, router *Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer ks_sk_live_0123456789_secretvaluewithcorrectlengthplus1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertServerError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	var response struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Code != code {
		t.Fatalf("error response = %#v", response)
	}
}

func TestServerAPIRequiresSecretKeyAndScopes(t *testing.T) {
	store := &fakeServerStore{}
	router := newServerTestRouter(store, serverSecretKey())

	// No credential.
	request := httptest.NewRequest(http.MethodGet, "/v1/server/users", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertServerError(t, recorder, http.StatusUnauthorized, "INVALID_CREDENTIAL")

	// Publishable key is rejected on server endpoints.
	request = httptest.NewRequest(http.MethodGet, "/v1/server/users", nil)
	request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertServerError(t, recorder, http.StatusUnauthorized, "INVALID_CREDENTIAL")

	// Missing required scope.
	verifier := &serverTestCredentialVerifier{credential: &domain.ApplicationCredential{
		ID: "cred-limited", ApplicationID: serverTestApplicationID, CredentialType: domain.CredentialSecret,
		Scopes: []string{"users.read"}, Status: domain.CredentialStatusActive,
	}}
	core := httpapi.NewRouter(httpapi.RouterConfig{
		DefaultApplicationID: serverTestApplicationID,
		Applications:         &serverTestApplicationResolver{},
		Credentials:          verifier,
		Server:               httpapi.ServerConfig{LicenseHMACKey: []byte("k"), Product: "StarLoader"},
		ServerStore:          store,
	})
	core.MountServer(New(core))
	limited := &Router{Router: core}
	recorder = serverRequest(t, limited, http.MethodGet, "/v1/server/licenses", "")
	assertServerError(t, recorder, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

func TestServerUsersCRUD(t *testing.T) {
	store := &fakeServerStore{
		user: &domain.ServerUser{ID: "user-1", Email: "a@b.c", Status: domain.UserStatusActive, CreatedAt: time.Now()},
	}
	router := newServerTestRouter(store, serverSecretKey())

	recorder := serverRequest(t, router, http.MethodGet, "/v1/server/users", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = serverRequest(t, router, http.MethodGet, "/v1/server/users/user-1", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/users", `{"email":"new@example.com","password":"longenoughpass","notes":"ops"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.notes != "ops" {
		t.Fatalf("create notes = %q", store.notes)
	}

	recorder = serverRequest(t, router, http.MethodPatch, "/v1/server/users/user-1", `{"status":"disabled","notes":"updated"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.status != domain.UserStatusDisabled || store.notes != "updated" {
		t.Fatalf("patch store = (%q, %q)", store.status, store.notes)
	}

	recorder = serverRequest(t, router, http.MethodDelete, "/v1/server/users/user-1", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.status != domain.UserStatusDisabled {
		t.Fatalf("delete did not soft-disable the user: status = %q", store.status)
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/users/user-1/ban", `{"reason":"abuse","expires_in":"720h"}`)
	if recorder.Code != http.StatusOK || store.banReason != "abuse" || store.banExpires == nil {
		t.Fatalf("ban status = %d, store = %#v", recorder.Code, store)
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/users/user-1/unban", "")
	if recorder.Code != http.StatusOK || store.unbanned != "user-1" {
		t.Fatalf("unban status = %d, store = %#v", recorder.Code, store)
	}

	store.resetCount = 2
	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/users/user-1/reset-devices", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset-devices status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var resetResponse struct {
		Revoked int64 `json:"revoked"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resetResponse); err != nil || resetResponse.Revoked != 2 {
		t.Fatalf("reset response = %s", recorder.Body.String())
	}
}

func TestServerLicensesCreateShowsKeyOnceAndExtend(t *testing.T) {
	store := &fakeServerStore{
		user:    &domain.ServerUser{ID: "user-1"},
		license: &domain.ServerLicense{ID: "license-1", UserID: "user-1", Product: "StarLoader", MaxDevices: 1, Level: 1, ExpiresAt: time.Now().Add(time.Hour)},
	}
	router := newServerTestRouter(store, serverSecretKey())

	recorder := serverRequest(t, router, http.MethodPost, "/v1/server/licenses", `{"user_id":"user-1","product":"StarLoader","duration_days":30,"max_devices":2,"level":3}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var createResponse struct {
		License string `json:"license"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &createResponse); err != nil || createResponse.License == "" {
		t.Fatalf("create response = %s", recorder.Body.String())
	}

	recorder = serverRequest(t, router, http.MethodGet, "/v1/server/licenses/license-1", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/licenses/license-1/extend", `{"duration_days":7}`)
	if recorder.Code != http.StatusOK || store.updated != "license-1" {
		t.Fatalf("extend status = %d, store = %#v", recorder.Code, store)
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/licenses/license-1/revoke", "")
	if recorder.Code != http.StatusOK || store.revoked != "license-1" {
		t.Fatalf("revoke status = %d, store = %#v", recorder.Code, store)
	}
}

func TestServerVariablesCRUD(t *testing.T) {
	store := &fakeServerStore{variables: []domain.Variable{
		{ID: "v1", Key: "minimum_version", Value: "1.4.0"},
	}}
	router := newServerTestRouter(store, serverSecretKey())

	recorder := serverRequest(t, router, http.MethodGet, "/v1/server/variables", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", recorder.Code)
	}

	recorder = serverRequest(t, router, http.MethodPost, "/v1/server/variables", `{"key":"maintenance","value":"false"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = serverRequest(t, router, http.MethodPatch, "/v1/server/variables/v1", `{"value":"1.5.0"}`)
	if recorder.Code != http.StatusOK || store.updated != "v1" {
		t.Fatalf("patch status = %d, store = %#v", recorder.Code, store)
	}

	recorder = serverRequest(t, router, http.MethodDelete, "/v1/server/variables/v1", "")
	if recorder.Code != http.StatusOK || store.deleted != "v1" {
		t.Fatalf("delete status = %d, store = %#v", recorder.Code, store)
	}
}

func TestServerDevicePolicy(t *testing.T) {
	store := &fakeServerStore{}
	router := newServerTestRouter(store, serverSecretKey())

	// GET returns defaults when no row exists.
	recorder := serverRequest(t, router, http.MethodGet, "/v1/server/device-policy", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var getResponse struct {
		OK     bool `json:"ok"`
		Policy struct {
			TPMPolicy     string `json:"tpm_policy"`
			MinMatchScore int    `json:"min_match_score"`
		} `json:"policy"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &getResponse); err != nil || !getResponse.OK {
		t.Fatalf("get response = %s", recorder.Body.String())
	}
	if getResponse.Policy.TPMPolicy != "optional" || getResponse.Policy.MinMatchScore != 70 {
		t.Fatalf("get defaults = %v", getResponse.Policy)
	}

	// PUT creates/updates the policy.
	recorder = serverRequest(t, router, http.MethodPut, "/v1/server/device-policy",
		`{"tpm_policy":"required","min_match_score":80,"step_up_score":50,"allow_auto_rebind":false,"rebind_cooldown_seconds":172800,"max_device_changes_per_30d":3}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var putResponse struct {
		OK     bool `json:"ok"`
		Policy struct {
			ID            string `json:"id"`
			TPMPolicy     string `json:"tpm_policy"`
			MinMatchScore int    `json:"min_match_score"`
		} `json:"policy"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &putResponse); err != nil || !putResponse.OK {
		t.Fatalf("put response = %s", recorder.Body.String())
	}
	if putResponse.Policy.ID != "policy-1" {
		t.Fatalf("put ID = %q", putResponse.Policy.ID)
	}
}

func TestServerAPIDisabledWithoutConfig(t *testing.T) {
	router := httpapi.NewRouter(httpapi.RouterConfig{})
	request := httptest.NewRequest(http.MethodGet, "/v1/server/users", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assertServerError(t, recorder, http.StatusServiceUnavailable, "SERVER_ERROR")
}
