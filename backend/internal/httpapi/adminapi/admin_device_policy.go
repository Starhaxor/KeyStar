package adminapi

import (
	"net/http"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

// devicePolicyJSON is the safe response shape of a device policy.
type devicePolicyJSON struct {
	ID                     string `json:"id"`
	ApplicationID          string `json:"application_id"`
	TPMPolicy              string `json:"tpm_policy"`
	MinMatchScore          int    `json:"min_match_score"`
	StepUpScore            int    `json:"step_up_score"`
	AllowAutoRebind        bool   `json:"allow_auto_rebind"`
	RebindCooldownSeconds  int64  `json:"rebind_cooldown_seconds"`
	MaxDeviceChangesPer30d int    `json:"max_device_changes_per_30d"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

func mapDevicePolicy(policy *domain.DevicePolicy) devicePolicyJSON {
	return devicePolicyJSON{
		ID:                     policy.ID,
		ApplicationID:          policy.ApplicationID,
		TPMPolicy:              string(policy.TPMPolicy),
		MinMatchScore:          policy.MinMatchScore,
		StepUpScore:            policy.StepUpScore,
		AllowAutoRebind:        policy.AllowAutoRebind,
		RebindCooldownSeconds:  policy.RebindCooldownSeconds,
		MaxDeviceChangesPer30d: policy.MaxDeviceChangesPer30d,
		CreatedAt:              httpapi.FormatTime(policy.CreatedAt),
		UpdatedAt:              httpapi.FormatTime(policy.UpdatedAt),
	}
}

// handleAdminDevicePolicyGet returns the device policy for the default
// application. When no row exists, the defaults are returned.
func (router *Router) handleAdminDevicePolicyGet(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	if !router.RequirePermission(writer, request, account, domain.PermDevicesRead) {
		return
	}
	policy, err := router.Admin.Console.GetDevicePolicy(request.Context(), router.AdminApplicationID(request))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK     bool             `json:"ok"`
		Policy devicePolicyJSON `json:"policy"`
	}{OK: true, Policy: mapDevicePolicy(policy)})
}

// handleAdminDevicePolicyUpdate creates or replaces the device policy for
// the default application.
func (router *Router) handleAdminDevicePolicyUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
		return
	}
	var body struct {
		TPMPolicy              string `json:"tpm_policy"`
		MinMatchScore          int    `json:"min_match_score"`
		StepUpScore            int    `json:"step_up_score"`
		AllowAutoRebind        bool   `json:"allow_auto_rebind"`
		RebindCooldownSeconds  int64  `json:"rebind_cooldown_seconds"`
		MaxDeviceChangesPer30d int    `json:"max_device_changes_per_30d"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.TPMPolicy == "" {
		body.TPMPolicy = string(domain.TPMOptional)
	}
	policy, err := router.Admin.Console.UpsertDevicePolicy(request.Context(), router.AdminApplicationID(request), domain.NewDevicePolicy{
		TPMPolicy:              domain.TPMPolicy(body.TPMPolicy),
		MinMatchScore:          body.MinMatchScore,
		StepUpScore:            body.StepUpScore,
		AllowAutoRebind:        body.AllowAutoRebind,
		RebindCooldownSeconds:  body.RebindCooldownSeconds,
		MaxDeviceChangesPer30d: body.MaxDeviceChangesPer30d,
	})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_POLICY_UPDATED", "device_policy", policy.ID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK     bool             `json:"ok"`
		Policy devicePolicyJSON `json:"policy"`
	}{OK: true, Policy: mapDevicePolicy(policy)})
}

// handleAdminDevicePolicyDelete removes the device policy for the default
// application, reverting to hard-coded defaults on next read.
func (router *Router) handleAdminDevicePolicyDelete(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
		return
	}
	if err := router.Admin.Console.DeleteDevicePolicy(request.Context(), router.AdminApplicationID(request)); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_POLICY_DELETED", "device_policy", router.AdminApplicationID(request), nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
