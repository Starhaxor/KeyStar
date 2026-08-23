package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// TestCredentialRateLimitRejectsWithRetryAfter verifies the per-credential
// throttle: the first requests pass, then 429 + Retry-After until the window
// rolls over.
func TestCredentialRateLimitRejectsWithRetryAfter(t *testing.T) {
	resolver := &middlewareTestApplicationResolver{}
	verifier := &middlewareTestCredentialVerifier{credential: activePublishableCredential("auth.login")}
	router := NewRouter(RouterConfig{
		Login:                &fakeLoginService{},
		DeviceVerification:   &fakeDeviceVerificationService{},
		DefaultApplicationID: middlewareTestApplicationID,
		Applications:         resolver,
		Credentials:          verifier,
		CredentialRateLimit:  2,
		RateLimitMaxKeys:     100,
	})

	newRequest := func() *http.Request {
		request := loginRequest(validLoginJSON)
		request.Header.Set("X-KeyStar-App", middlewareTestApplicationID)
		request.Header.Set("Authorization", "Bearer ks_pk_live_0123456789_secretvaluewithcorrectlengthplus1")
		return request
	}

	codes := make([]int, 3)
	var retryAfter string
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, newRequest())
		codes[i] = recorder.Code
		if recorder.Code == http.StatusTooManyRequests {
			retryAfter = recorder.Header().Get("Retry-After")
			if retryAfter == "" {
				t.Fatal("429 response missing Retry-After header")
			}
			if _, err := strconv.Atoi(retryAfter); err != nil {
				t.Fatalf("Retry-After %q is not numeric", retryAfter)
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != "RATE_LIMITED" {
				t.Fatalf("error code = %q", body.Code)
			}
		}
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("first attempts should pass, got %v", codes)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("third attempt = %d, want 429", codes[2])
	}
}
