package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/service"
)

type captureRefreshService struct{ applicationID string }

func (fake *captureRefreshService) Refresh(_ context.Context, input service.RefreshInput) (service.RefreshResult, error) {
	fake.applicationID = input.ApplicationID
	return service.RefreshResult{AccessToken: "access", RefreshToken: "next", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (fake *captureRefreshService) Revoke(_ context.Context, input service.RefreshInput) error {
	fake.applicationID = input.ApplicationID
	return nil
}

func TestRefreshRequiresPublishableCredentialAndUsesSelectedApplication(t *testing.T) {
	refresh := &captureRefreshService{}
	verifier := &middlewareTestCredentialVerifier{credential: activePublishableCredential("auth.refresh")}
	router := NewRouter(RouterConfig{DefaultApplicationID: middlewareTestApplicationID, Applications: &middlewareTestApplicationResolver{},
		Credentials: verifier, DisableLegacyApplication: true, RefreshService: refresh})

	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refresh_token":"token"}`))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", unauthorizedRecorder.Code, unauthorizedRecorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", strings.NewReader(`{"refresh_token":"token"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
	request.Header.Set(applicationHeader, middlewareTestApplicationID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || refresh.applicationID != middlewareTestApplicationID {
		t.Fatalf("status=%d app=%q body=%s", recorder.Code, refresh.applicationID, recorder.Body.String())
	}
}
