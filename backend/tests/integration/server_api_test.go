package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/httpapi/serverapi"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
)

func serverAPIRequest(t *testing.T, router *httpapi.Router, method, path, authorization, applicationID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if applicationID != "" {
		request.Header.Set("X-KeyStar-App", applicationID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestServerAPIEndToEnd(t *testing.T) {
	// Full server-to-server lifecycle through the HTTP router: secret-key
	// authentication, scope enforcement, user/license/variable management,
	// and cross-tenant isolation.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)

	secret, err := credential.Generate("secret", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: applicationID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialSecret, Name: "Backend Ops",
		KeyPrefix: secret.Prefix, KeyHash: secret.Hash,
		Scopes: []string{"users.read", "users.write", "licenses.read", "licenses.write", "devices.write", "variables.read", "variables.write"},
	}); err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewRouter(httpapi.RouterConfig{
		Login:                service.NewLoginService(repository, "StarLoader"),
		DeviceVerification:   newIntegrationDeviceService(t, repository, time.Now().UTC().Truncate(time.Second)),
		DefaultApplicationID: applicationID,
		Applications:         repository,
		Credentials:          credential.NewVerifier(repository),
		Server:               httpapi.ServerConfig{LicenseHMACKey: []byte("integration-license-hmac"), Product: "StarLoader"},
		ServerStore:          repository,
	})
	router.MountServer(serverapi.New(router))
	authorization := "Bearer " + secret.Key

	// The server API never falls back to the legacy default application: a
	// request without a credential is rejected even in legacy mode.
	assertIntegrationError(t, serverAPIRequest(t, router, http.MethodGet, "/v1/server/users", "", "", ""), http.StatusUnauthorized, "INVALID_CREDENTIAL")

	// Publishable keys cannot reach the server API.
	publishable, err := credential.Generate("publishable", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: applicationID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialPublishable, Name: "Desktop SDK",
		KeyPrefix: publishable.Prefix, KeyHash: publishable.Hash, Scopes: []string{"auth.login"},
	}); err != nil {
		t.Fatal(err)
	}
	assertIntegrationError(t, serverAPIRequest(t, router, http.MethodGet, "/v1/server/users", "Bearer "+publishable.Key, "", ""), http.StatusUnauthorized, "INVALID_CREDENTIAL")

	// A secret key without the required scope is denied.
	limited, err := credential.Generate("secret", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: applicationID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialSecret, Name: "Read-only",
		KeyPrefix: limited.Prefix, KeyHash: limited.Hash, Scopes: []string{"users.read"},
	}); err != nil {
		t.Fatal(err)
	}
	assertIntegrationError(t, serverAPIRequest(t, router, http.MethodGet, "/v1/server/licenses", "Bearer "+limited.Key, "", ""), http.StatusForbidden, "INSUFFICIENT_SCOPE")

	// Create a user and a license through the server API.
	created := serverAPIRequest(t, router, http.MethodPost, "/v1/server/users", authorization, "", `{"email":"server-user@example.com","password":"server-pass-123","notes":"created by ops"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdUser struct {
		OK   bool `json:"ok"`
		Data struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Notes string `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdUser); err != nil {
		t.Fatal(err)
	}
	if !createdUser.OK || createdUser.Data.ID == "" || createdUser.Data.Email != "server-user@example.com" || createdUser.Data.Notes != "created by ops" {
		t.Fatalf("create user response = %s", created.Body.String())
	}

	licenseCreated := serverAPIRequest(t, router, http.MethodPost, "/v1/server/licenses", authorization, "", `{"user_id":"`+createdUser.Data.ID+`","product":"StarLoader","duration_days":30,"max_devices":2,"level":3}`)
	if licenseCreated.Code != http.StatusCreated {
		t.Fatalf("create license status = %d, body = %s", licenseCreated.Code, licenseCreated.Body.String())
	}
	var licenseResponse struct {
		OK      bool   `json:"ok"`
		ID      string `json:"id"`
		License string `json:"license"`
	}
	if err := json.Unmarshal(licenseCreated.Body.Bytes(), &licenseResponse); err != nil {
		t.Fatal(err)
	}
	if !licenseResponse.OK || licenseResponse.ID == "" || licenseResponse.License == "" {
		t.Fatalf("create license response = %s", licenseCreated.Body.String())
	}
	// The database holds the HMAC of the normalized key, never the plaintext.
	var storedHMAC string
	if err := pool.QueryRow(ctx, `select license_hmac from licenses where id = $1::uuid`, licenseResponse.ID).Scan(&storedHMAC); err != nil {
		t.Fatal(err)
	}
	wantHMAC := security.HMACHex([]byte("integration-license-hmac"), security.NormalizeLicense(licenseResponse.License))
	if storedHMAC != wantHMAC || storedHMAC == licenseResponse.License {
		t.Fatalf("stored license HMAC = %q, want hash of the shown key", storedHMAC)
	}

	// List and detail endpoints return the created records.
	listed := serverAPIRequest(t, router, http.MethodGet, "/v1/server/users?limit=10", authorization, "", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), createdUser.Data.ID) {
		t.Fatalf("list users status = %d, body = %s", listed.Code, listed.Body.String())
	}
	licenseListed := serverAPIRequest(t, router, http.MethodGet, "/v1/server/licenses?limit=10", authorization, "", "")
	if licenseListed.Code != http.StatusOK || !strings.Contains(licenseListed.Body.String(), licenseResponse.ID) {
		t.Fatalf("list licenses status = %d, body = %s", licenseListed.Code, licenseListed.Body.String())
	}
	detail := serverAPIRequest(t, router, http.MethodGet, "/v1/server/users/"+createdUser.Data.ID, authorization, "", "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "server-user@example.com") {
		t.Fatalf("user detail status = %d, body = %s", detail.Code, detail.Body.String())
	}

	// Ban, unban and reset-devices.
	banned := serverAPIRequest(t, router, http.MethodPost, "/v1/server/users/"+createdUser.Data.ID+"/ban", authorization, "", `{"reason":"abuse","expires_in":"720h"}`)
	assertIntegrationOK(t, banned, http.StatusOK)
	var banState struct {
		Status string `json:"status"`
	}
	if err := pool.QueryRow(ctx, `select status::text from users where id = $1::uuid`, createdUser.Data.ID).Scan(&banState.Status); err != nil {
		t.Fatal(err)
	}
	if banState.Status != "banned" {
		t.Fatalf("user status after ban = %q", banState.Status)
	}
	assertIntegrationOK(t, serverAPIRequest(t, router, http.MethodPost, "/v1/server/users/"+createdUser.Data.ID+"/unban", authorization, "", ""), http.StatusOK)

	if _, err := pool.Exec(ctx, `
		insert into devices (application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, last_seen_at)
		values ($1::uuid, $2::uuid, $3::uuid, 'tpm', decode(repeat('ab', 32), 'hex'), repeat('f', 64), now())`,
		applicationID, createdUser.Data.ID, licenseResponse.ID); err != nil {
		t.Fatal(err)
	}
	reset := serverAPIRequest(t, router, http.MethodPost, "/v1/server/users/"+createdUser.Data.ID+"/reset-devices", authorization, "", "")
	if reset.Code != http.StatusOK || !strings.Contains(reset.Body.String(), `"revoked":1`) {
		t.Fatalf("reset-devices status = %d, body = %s", reset.Code, reset.Body.String())
	}

	// License extend and revoke.
	extended := serverAPIRequest(t, router, http.MethodPost, "/v1/server/licenses/"+licenseResponse.ID+"/extend", authorization, "", `{"duration_days":7}`)
	if extended.Code != http.StatusOK {
		t.Fatalf("extend status = %d, body = %s", extended.Code, extended.Body.String())
	}
	revoked := serverAPIRequest(t, router, http.MethodPost, "/v1/server/licenses/"+licenseResponse.ID+"/revoke", authorization, "", "")
	assertIntegrationOK(t, revoked, http.StatusOK)
	var licenseStatus string
	if err := pool.QueryRow(ctx, `select status::text from licenses where id = $1::uuid`, licenseResponse.ID).Scan(&licenseStatus); err != nil {
		t.Fatal(err)
	}
	if licenseStatus != "revoked" {
		t.Fatalf("license status after revoke = %q", licenseStatus)
	}

	// Variable CRUD.
	varCreated := serverAPIRequest(t, router, http.MethodPost, "/v1/server/variables", authorization, "", `{"key":"minimum_version","value":"1.4.0"}`)
	if varCreated.Code != http.StatusCreated {
		t.Fatalf("create variable status = %d, body = %s", varCreated.Code, varCreated.Body.String())
	}
	var variableResponse struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(varCreated.Body.Bytes(), &variableResponse); err != nil {
		t.Fatal(err)
	}
	varsListed := serverAPIRequest(t, router, http.MethodGet, "/v1/server/variables", authorization, "", "")
	if varsListed.Code != http.StatusOK || !strings.Contains(varsListed.Body.String(), "minimum_version") {
		t.Fatalf("list variables status = %d, body = %s", varsListed.Code, varsListed.Body.String())
	}
	assertIntegrationOK(t, serverAPIRequest(t, router, http.MethodPatch, "/v1/server/variables/"+variableResponse.Data.ID, authorization, "", `{"value":"1.5.0"}`), http.StatusOK)
	assertIntegrationOK(t, serverAPIRequest(t, router, http.MethodDelete, "/v1/server/variables/"+variableResponse.Data.ID, authorization, "", ""), http.StatusOK)

	// Cross-tenant isolation: another application's credential and context
	// cannot see this application's users (404, never 403 or 200).
	organization, err := repository.CreateOrganization(ctx, "server-tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	appB, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Server Tenant B", Slug: "server-tenant-b-app",
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantBKey, err := credential.Generate("secret", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: appB.ID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialSecret, Name: "Tenant B Ops",
		KeyPrefix: tenantBKey.Prefix, KeyHash: tenantBKey.Hash, Scopes: []string{"users.read"},
	}); err != nil {
		t.Fatal(err)
	}
	tenantBList := serverAPIRequest(t, router, http.MethodGet, "/v1/server/users", "Bearer "+tenantBKey.Key, appB.ID, "")
	if tenantBList.Code != http.StatusOK || strings.Contains(tenantBList.Body.String(), createdUser.Data.ID) {
		t.Fatalf("tenant B sees tenant A data: status = %d, body = %s", tenantBList.Code, tenantBList.Body.String())
	}
	tenantBDetail := serverAPIRequest(t, router, http.MethodGet, "/v1/server/users/"+createdUser.Data.ID, "Bearer "+tenantBKey.Key, appB.ID, "")
	assertIntegrationError(t, tenantBDetail, http.StatusNotFound, "USER_NOT_FOUND")
}
