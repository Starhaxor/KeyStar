package adminapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starloader/backend/internal/domain"
)

func TestAdminApplicationPatchTransitionsAuthProfileAndReturnsIt(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{
		ID: "application-2", OrganizationID: "org-1", Name: "Portal", Slug: "portal",
		Status: domain.ApplicationStatusActive, AuthProfile: domain.ApplicationAuthLegacy,
	}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/applications/application-2", `{"auth_profile":"proof_bound"}`))

	var body struct {
		Application struct {
			AuthProfile string `json:"auth_profile"`
		} `json:"application"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Application.AuthProfile != "proof_bound" {
		t.Fatalf("status = %d, auth_profile = %q, body = %s", recorder.Code, body.Application.AuthProfile, recorder.Body.String())
	}
	if console.updateApplicationCalls != 1 {
		t.Fatalf("update calls = %d, want 1", console.updateApplicationCalls)
	}
	assertLifecycleAudit(t, console, "APPLICATION_UPDATED", "application-2")
}

func TestAdminApplicationPatchRejectsUnknownAuthProfile(t *testing.T) {
	console := &fakeLifecycleConsole{application: &domain.Application{ID: "application-2", Status: domain.ApplicationStatusActive}}
	router := newAdminLifecycleTestRouter(&fakeAdminAuth{token: "session-token", account: testOwnerAccount()}, console)

	recorder := httptest.NewRecorder()
	router.serveAdmin(recorder, lifecycleRequest(t, router, http.MethodPatch, "/v1/admin/applications/application-2", `{"auth_profile":"unknown"}`))

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusBadRequest || body.Code != "INVALID_REQUEST" {
		t.Fatalf("status = %d, code = %q, body = %s", recorder.Code, body.Code, recorder.Body.String())
	}
	if console.updateApplicationCalls != 0 {
		t.Fatalf("update calls = %d, want 0", console.updateApplicationCalls)
	}
}
