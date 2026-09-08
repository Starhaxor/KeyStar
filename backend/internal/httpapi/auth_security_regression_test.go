package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
)

func TestSessionRejectsInactiveApplication(t *testing.T) {
	for _, profile := range []domain.ApplicationAuthProfile{domain.ApplicationAuthLegacy, domain.ApplicationAuthProofBound} {
		for _, status := range []domain.ApplicationStatus{domain.ApplicationStatusDisabled, domain.ApplicationStatusSuspended, domain.ApplicationStatusMaintenance} {
			t.Run(string(profile)+"/"+string(status), func(t *testing.T) {
				now := time.Now().UTC().Truncate(time.Second)
				key := newDPoPMiddlewareKey(t)
				claims := proofBoundMiddlewareClaims(now, key.thumb)
				application := proofBoundApplication()
				application.AuthProfile, application.Status = profile, status
				config := sessionAuthConfig(nil)
				config.Applications = &middlewareTestApplicationResolver{application: application}
				config.Now = func() time.Time { return now }
				config.Replays = &fakeReplayStore{consumed: true}
				request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
				token := unsignedAccessToken(application.ID)
				if profile == domain.ApplicationAuthLegacy {
					claims.ProofBound = nil
					config.LegacyVerifier = &fakeBearerVerifier{claims: claims}
					request.Header.Set("Authorization", "Bearer "+token)
				} else {
					config.ProofBoundVerifier = &fakeProofBoundVerifier{claims: claims}
					request.Header.Set("Authorization", "DPoP "+token)
					request.Header.Set("DPoP", key.mintProof(t, token, "GET", "https://api.example.com/v1/me", randomCanonicalJTI(t), now.Unix()))
				}
				called := false
				handler := RequireSession(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusUnauthorized || called {
					t.Fatalf("inactive application admitted: status=%d handler=%t", response.Code, called)
				}
			})
		}
	}
}

func TestInvalidTokensDoNotExhaustValidSessionQuota(t *testing.T) {
	verifier := &fakeBearerVerifier{err: errors.New("invalid signature")}
	router := NewRouter(RouterConfig{SessionVerifier: verifier, RateLimitMaxKeys: 4, Profile: &fakeProfileRepository{profile: validMeProfile()}})
	for _, token := range []string{"fake-one", "fake-two", "fake-three", "fake-four", "fake-five"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("invalid token poisoned shared quota: status=%d", response.Code)
		}
	}
}
