package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starloader/backend/internal/domain"
)

const middlewareTestApplicationID = "019c1111-1111-7111-8111-111111111111"

type middlewareTestApplicationResolver struct {
	application *domain.Application
	err         error
}

func (resolver *middlewareTestApplicationResolver) FindApplicationByID(_ context.Context, applicationID string) (*domain.Application, error) {
	if resolver.err != nil {
		return nil, resolver.err
	}
	if resolver.application != nil {
		return resolver.application, nil
	}
	return &domain.Application{
		ID: applicationID, OrganizationID: "org-1", Name: "Test App", Slug: "test-app",
		Status: domain.ApplicationStatusActive, EnvironmentMode: "separate",
	}, nil
}

type middlewareTestCredentialVerifier struct {
	credential *domain.ApplicationCredential
	err        error
	key        string
	appID      string
}

func (verifier *middlewareTestCredentialVerifier) Verify(_ context.Context, applicationID, key string) (*domain.ApplicationCredential, error) {
	verifier.appID = applicationID
	verifier.key = key
	if verifier.err != nil {
		return nil, verifier.err
	}
	return verifier.credential, nil
}

func newMiddlewareTestRouter(credentials CredentialVerifier, resolver ApplicationResolver, legacy bool) *Router {
	return NewRouter(RouterConfig{
		Login:                    &fakeLoginService{},
		DeviceVerification:       &fakeDeviceVerificationService{},
		DefaultApplicationID:     middlewareTestApplicationID,
		Applications:             resolver,
		Credentials:              credentials,
		DisableLegacyApplication: legacy,
	})
}

func activePublishableCredential(scopes ...string) *domain.ApplicationCredential {
	return &domain.ApplicationCredential{
		ID: "cred-1", ApplicationID: middlewareTestApplicationID,
		Environment: domain.CredentialEnvironmentLive, CredentialType: domain.CredentialPublishable,
		Scopes: scopes, Status: domain.CredentialStatusActive,
	}
}

func TestApplicationMiddlewareLegacyFallsBackToDefaultApplication(t *testing.T) {
	router := newMiddlewareTestRouter(nil, &middlewareTestApplicationResolver{}, false)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, loginRequest(validLoginJSON))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestApplicationMiddlewareHeaderSelectsApplication(t *testing.T) {
	router := newMiddlewareTestRouter(nil, &middlewareTestApplicationResolver{}, false)
	request := loginRequest(validLoginJSON)
	request.Header.Set(applicationHeader, "019c2222-2222-7222-8222-222222222222")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestApplicationMiddlewareRejectsInvalidOrUnknownApplication(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		resolver ApplicationResolver
		status   int
		code     string
	}{
		{name: "malformed header", header: "not-a-uuid", resolver: &middlewareTestApplicationResolver{}, status: http.StatusBadRequest, code: "INVALID_APPLICATION"},
		{name: "unknown application", header: "019c2222-2222-7222-8222-222222222222", resolver: &middlewareTestApplicationResolver{err: domain.ErrApplicationNotFound}, status: http.StatusNotFound, code: "INVALID_APPLICATION"},
		{name: "disabled application", header: middlewareTestApplicationID, resolver: &middlewareTestApplicationResolver{application: &domain.Application{
			ID: middlewareTestApplicationID, OrganizationID: "org-1", Status: domain.ApplicationStatusDisabled,
		}}, status: http.StatusForbidden, code: "APPLICATION_DISABLED"},
		{name: "suspended application", header: middlewareTestApplicationID, resolver: &middlewareTestApplicationResolver{application: &domain.Application{
			ID: middlewareTestApplicationID, OrganizationID: "org-1", Status: domain.ApplicationStatusSuspended,
		}}, status: http.StatusForbidden, code: "APPLICATION_DISABLED"},
		{name: "maintenance application", header: middlewareTestApplicationID, resolver: &middlewareTestApplicationResolver{application: &domain.Application{
			ID: middlewareTestApplicationID, OrganizationID: "org-1", Status: domain.ApplicationStatusMaintenance,
		}}, status: http.StatusServiceUnavailable, code: "APPLICATION_MAINTENANCE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := newMiddlewareTestRouter(nil, test.resolver, false)
			request := loginRequest(validLoginJSON)
			request.Header.Set(applicationHeader, test.header)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assertErrorResponse(t, recorder, test.status, test.code)
		})
	}
}

func TestApplicationMiddlewareAcceptsValidPublishableCredential(t *testing.T) {
	credentials := &middlewareTestCredentialVerifier{credential: activePublishableCredential("auth.login", "device.verify")}
	router := newMiddlewareTestRouter(credentials, &middlewareTestApplicationResolver{}, false)
	request := loginRequest(validLoginJSON)
	request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if credentials.appID != middlewareTestApplicationID {
		t.Fatalf("Verify() application = %q", credentials.appID)
	}
	if credentials.key != "ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1" {
		t.Fatalf("Verify() key = %q", credentials.key)
	}
}

func TestApplicationMiddlewareRejectsWrongCredentialTypeAndScope(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		credential    *domain.ApplicationCredential
		status        int
		code          string
	}{
		{
			name: "secret key on client endpoint", authorization: "Bearer ks_sk_live_0123456789_secretvaluewithcorrectlengthplus1",
			credential: &domain.ApplicationCredential{ID: "cred-secret", CredentialType: domain.CredentialSecret, Scopes: []string{"users.read"}, Status: domain.CredentialStatusActive},
			status:     http.StatusUnauthorized, code: "INVALID_CREDENTIAL",
		},
		{
			name: "missing scope", authorization: "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1",
			credential: activePublishableCredential("device.verify"),
			status:     http.StatusForbidden, code: "INSUFFICIENT_SCOPE",
		},
		{
			name: "malformed bearer", authorization: "Bearer",
			credential: activePublishableCredential("auth.login"),
			status:     http.StatusUnauthorized, code: "INVALID_CREDENTIAL",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := &middlewareTestCredentialVerifier{credential: test.credential}
			router := newMiddlewareTestRouter(credentials, &middlewareTestApplicationResolver{}, false)
			request := loginRequest(validLoginJSON)
			request.Header.Set("Authorization", test.authorization)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assertErrorResponse(t, recorder, test.status, test.code)
		})
	}
}

func TestApplicationMiddlewareMapsVerificationFailures(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid", err: domain.ErrInvalidCredential, status: http.StatusUnauthorized, code: "INVALID_CREDENTIAL"},
		{name: "revoked", err: domain.ErrCredentialRevoked, status: http.StatusUnauthorized, code: "CREDENTIAL_REVOKED"},
		{name: "expired", err: domain.ErrCredentialExpired, status: http.StatusUnauthorized, code: "CREDENTIAL_EXPIRED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := &middlewareTestCredentialVerifier{err: test.err}
			router := newMiddlewareTestRouter(credentials, &middlewareTestApplicationResolver{}, false)
			request := loginRequest(validLoginJSON)
			request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assertErrorResponse(t, recorder, test.status, test.code)
		})
	}
}

func TestApplicationMiddlewareLegacyDisabledRequiresCredential(t *testing.T) {
	router := newMiddlewareTestRouter(nil, &middlewareTestApplicationResolver{}, true)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, loginRequest(validLoginJSON))

	assertErrorResponse(t, recorder, http.StatusUnauthorized, "INVALID_CREDENTIAL")
}

func TestAppPrincipalFromContextCarriesCredentialDetails(t *testing.T) {
	credentials := &middlewareTestCredentialVerifier{credential: activePublishableCredential("auth.login", "variables.read_public")}
	router := newMiddlewareTestRouter(credentials, &middlewareTestApplicationResolver{}, false)
	var captured AppPrincipal
	router.loginHandler = router.RequireCredential(domain.CredentialPublishable, "auth.login")(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := AppPrincipalFromContext(request.Context())
		if !ok {
			t.Error("principal missing from context")
			return
		}
		captured = principal
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := loginRequest(validLoginJSON)
	request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if captured.ApplicationID != middlewareTestApplicationID || captured.CredentialID != "cred-1" ||
		captured.CredentialType != string(domain.CredentialPublishable) || captured.Environment != string(domain.CredentialEnvironmentLive) {
		t.Fatalf("principal = %#v", captured)
	}
	if _, ok := captured.Scopes["auth.login"]; !ok {
		t.Fatalf("principal scopes = %#v", captured.Scopes)
	}
}

func TestDeviceVerifyReceivesPrincipalApplicationID(t *testing.T) {
	verification := &fakeDeviceVerificationService{}
	credentials := &middlewareTestCredentialVerifier{credential: activePublishableCredential("device.verify")}
	router := NewRouter(RouterConfig{
		Login:                &fakeLoginService{},
		DeviceVerification:   verification,
		DefaultApplicationID: middlewareTestApplicationID,
		Applications:         &middlewareTestApplicationResolver{},
		Credentials:          credentials,
	})
	request := deviceVerifyRequest(validDeviceVerifyJSON)
	request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if verification.input.ApplicationID != middlewareTestApplicationID {
		t.Fatalf("Verify() application = %q", verification.input.ApplicationID)
	}
}

func TestLoginServiceReceivesResolvedApplicationID(t *testing.T) {
	login := &fakeLoginService{}
	router := NewRouter(RouterConfig{
		Login:                login,
		DefaultApplicationID: middlewareTestApplicationID,
		Applications:         &middlewareTestApplicationResolver{},
	})
	request := loginRequest(validLoginJSON)
	request.Header.Set(applicationHeader, "019c2222-2222-7222-8222-222222222222")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if login.input.ApplicationID != "019c2222-2222-7222-8222-222222222222" {
		t.Fatalf("Login() application = %q", login.input.ApplicationID)
	}
	if login.input.Email != "a@b.c" || login.input.Password != "x" {
		t.Fatalf("Login() input = %#v", login.input)
	}
}
