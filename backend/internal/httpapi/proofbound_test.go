package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/service"
)

const proofBoundDeviceJWK = `{"crv":"P-256","kty":"EC","x":"axfR8uEsQkf4vOblY6RA8ncDfYEt6zOg9KE5RdiYwpY","y":"T-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU"}`

func deviceVerifyJSONWithJWK(t *testing.T, includeLegacyBlob bool) string {
	t.Helper()
	body := validDeviceVerifyJSON
	if !includeLegacyBlob {
		body = strings.Replace(body, `"tpm_public_key":"cHVibGljLWtleQ==",`, ``, 1)
	}
	body = strings.TrimSuffix(body, "}")
	return body + `,"device_jwk":` + proofBoundDeviceJWK + `}`
}

func TestDeviceVerifyForwardsDeviceJWKAndOmitsRefreshToken(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	verification := &fakeDeviceVerificationService{verified: service.VerifiedSession{
		Token: "proof-token", ExpiresAt: now, LicenseID: "license-1", DeviceID: "device-1",
	}}
	router := NewRouter(RouterConfig{Login: &fakeLoginService{}, DeviceVerification: verification})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, deviceVerifyRequest(deviceVerifyJSONWithJWK(t, true)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var rawJWK json.RawMessage
	if err := json.Unmarshal(verification.input.DeviceJWK, &rawJWK); err != nil {
		t.Fatalf("Verify() DeviceJWK = %q, decode error = %v", verification.input.DeviceJWK, err)
	}
	if strings.TrimSpace(string(verification.input.DeviceJWK)) != proofBoundDeviceJWK {
		t.Fatalf("Verify() DeviceJWK = %s", verification.input.DeviceJWK)
	}
	var response map[string]any
	decodeResponse(t, recorder, &response)
	if _, present := response["refresh_token"]; present {
		t.Fatalf("proof-bound response contains refresh_token: %#v", response)
	}
	if response["token"] != "proof-token" {
		t.Fatalf("response = %#v", response)
	}
}

func TestDeviceVerifyAcceptsJWKWithoutLegacyBlob(t *testing.T) {
	verification := &fakeDeviceVerificationService{}
	router := NewRouter(RouterConfig{Login: &fakeLoginService{}, DeviceVerification: verification})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, deviceVerifyRequest(deviceVerifyJSONWithJWK(t, false)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if verification.calls != 1 || len(verification.input.DeviceJWK) == 0 {
		t.Fatalf("Verify() calls = %d, input = %#v", verification.calls, verification.input)
	}
}

type countingRefreshService struct {
	calls         int
	applicationID string
}

func (fake *countingRefreshService) Refresh(_ context.Context, input service.RefreshInput) (service.RefreshResult, error) {
	fake.calls++
	fake.applicationID = input.ApplicationID
	return service.RefreshResult{AccessToken: "access", RefreshToken: "next", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (fake *countingRefreshService) Revoke(_ context.Context, input service.RefreshInput) error {
	fake.calls++
	fake.applicationID = input.ApplicationID
	return nil
}

func proofBoundHTTPRouter(refresh RefreshService, resolver ApplicationResolver) *Router {
	return NewRouter(RouterConfig{
		Login:                &fakeLoginService{},
		DeviceVerification:   &fakeDeviceVerificationService{},
		DefaultApplicationID: middlewareTestApplicationID,
		Applications:         resolver,
		RefreshService:       refresh,
	})
}

func proofBoundApplication() *domain.Application {
	return &domain.Application{
		ID: middlewareTestApplicationID, OrganizationID: "org-1", Name: "Proof App", Slug: "proof-app",
		Status: domain.ApplicationStatusActive, AuthProfile: domain.ApplicationAuthProofBound, EnvironmentMode: "separate",
	}
}

func TestRefreshRejectsProofBoundApplication(t *testing.T) {
	refresh := &countingRefreshService{}
	router := proofBoundHTTPRouter(refresh, &middlewareTestApplicationResolver{application: proofBoundApplication()})
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refresh_token":"legacy-token"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	response := assertErrorResponse(t, recorder, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN")
	if strings.Contains(response.Message, "legacy-token") {
		t.Fatalf("response reflects the presented token: %#v", response)
	}
	if refresh.calls != 0 {
		t.Fatal("proof-bound refresh reached the refresh service")
	}
}

func TestRefreshFailsClosedWhenApplicationPolicyUnavailable(t *testing.T) {
	refresh := &countingRefreshService{}
	router := proofBoundHTTPRouter(refresh, &middlewareTestApplicationResolver{err: domain.ErrApplicationNotFound})
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refresh_token":"legacy-token"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if refresh.calls != 0 {
		t.Fatal("unknown application policy reached the refresh service")
	}
}
