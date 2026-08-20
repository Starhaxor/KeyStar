package serverapi

import (
	"net/http"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

// serverDevicePolicyJSON is the safe response shape of a device policy for
// the machine-to-machine API.
type serverDevicePolicyJSON struct {
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

func mapServerDevicePolicy(policy *domain.DevicePolicy) serverDevicePolicyJSON {
	return serverDevicePolicyJSON{
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

// handleServerDevicePolicyGet returns the device policy for the calling
// application. When no row exists, defaults are returned.
func (router *Router) handleServerDevicePolicyGet(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	policy, err := router.ServerStore.GetDevicePolicy(request.Context(), applicationID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK     bool                   `json:"ok"`
		Policy serverDevicePolicyJSON `json:"policy"`
	}{OK: true, Policy: mapServerDevicePolicy(policy)})
}

// handleServerDevicePolicyUpdate creates or replaces the device policy for
// the calling application.
func (router *Router) handleServerDevicePolicyUpdate(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
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
	policy, err := router.ServerStore.UpsertDevicePolicy(request.Context(), applicationID, domain.NewDevicePolicy{
		TPMPolicy:              domain.TPMPolicy(body.TPMPolicy),
		MinMatchScore:          body.MinMatchScore,
		StepUpScore:            body.StepUpScore,
		AllowAutoRebind:        body.AllowAutoRebind,
		RebindCooldownSeconds:  body.RebindCooldownSeconds,
		MaxDeviceChangesPer30d: body.MaxDeviceChangesPer30d,
	})
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK     bool                   `json:"ok"`
		Policy serverDevicePolicyJSON `json:"policy"`
	}{OK: true, Policy: mapServerDevicePolicy(policy)})
}
