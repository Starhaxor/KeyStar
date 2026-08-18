package serverapi

import (
	cryptorand "crypto/rand"
	"net/http"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
)

func (router *Router) handleServerLicenseList(writer http.ResponseWriter, request *http.Request) {
	limit, after := parseServerPagination(request)
	licenses, nextCursor, hasMore, err := router.ServerStore.ListServerLicenses(request.Context(), principalApplicationID(request), after, limit)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool                `json:"ok"`
		Data []serverLicenseJSON `json:"data"`
		Page serverPage          `json:"page"`
	}{OK: true, Data: mapServerLicenses(licenses), Page: serverPage{NextCursor: nextCursor, HasMore: hasMore}})
}

type createServerLicenseRequestBody struct {
	UserID       string `json:"user_id"`
	UserEmail    string `json:"user_email"`
	Product      string `json:"product"`
	DurationDays int    `json:"duration_days"`
	MaxDevices   int    `json:"max_devices"`
	Level        int    `json:"level"`
	Notes        string `json:"notes"`
}

// handleServerLicenseCreate generates a fresh license key, persists only its
// HMAC and returns the plaintext key exactly once.
func (router *Router) handleServerLicenseCreate(writer http.ResponseWriter, request *http.Request) {
	var body createServerLicenseRequestBody
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	product := strings.TrimSpace(body.Product)
	if product == "" || body.DurationDays <= 0 || (strings.TrimSpace(body.UserID) == "" && strings.TrimSpace(body.UserEmail) == "") {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.MaxDevices <= 0 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	applicationID := principalApplicationID(request)
	var user *domain.User
	if strings.TrimSpace(body.UserID) != "" {
		found, err := router.ServerStore.FindUserByID(request.Context(), applicationID, strings.TrimSpace(body.UserID))
		if err != nil {
			router.writeServerError(writer, request, err)
			return
		}
		user = found
	} else {
		found, err := router.ServerStore.FindUserByEmail(request.Context(), applicationID, strings.TrimSpace(body.UserEmail))
		if err != nil {
			router.writeServerError(writer, request, err)
			return
		}
		user = found
	}

	plain, normalized, err := security.GenerateLicense(cryptorand.Reader)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	created, err := router.ServerStore.CreateLicense(request.Context(), applicationID, domain.NewLicense{
		LicenseHMAC: security.HMACHex(router.Server.LicenseHMACKey, normalized),
		UserID:      user.ID,
		Product:     product,
		MaxDevices:  body.MaxDevices,
		ExpiresAt:   time.Now().UTC().AddDate(0, 0, body.DurationDays),
	})
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK        bool   `json:"ok"`
		ID        string `json:"id"`
		License   string `json:"license"` // shown exactly once
		Product   string `json:"product"`
		ExpiresAt string `json:"expires_at"`
	}{
		OK: true, ID: created.ID, License: plain, Product: created.Product,
		ExpiresAt: created.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (router *Router) handleServerLicenseDetail(writer http.ResponseWriter, request *http.Request) {
	license, err := router.ServerStore.FindServerLicenseByID(request.Context(), principalApplicationID(request), serverPathID(request))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool              `json:"ok"`
		Data serverLicenseJSON `json:"data"`
	}{OK: true, Data: mapServerLicense(*license)})
}

func (router *Router) handleServerLicenseRevoke(writer http.ResponseWriter, request *http.Request) {
	if err := router.ServerStore.AdminRevokeLicense(request.Context(), principalApplicationID(request), serverPathID(request)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

type extendServerLicenseRequestBody struct {
	DurationDays int    `json:"duration_days"`
	MaxDevices   *int   `json:"max_devices"`
	Level        *int   `json:"level"`
	Notes        string `json:"notes"`
}

// handleServerLicenseExtend prolongs a license from its current expiry (or
// now, whichever is later) and optionally adjusts its limits.
func (router *Router) handleServerLicenseExtend(writer http.ResponseWriter, request *http.Request) {
	var body extendServerLicenseRequestBody
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.DurationDays <= 0 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	applicationID := principalApplicationID(request)
	licenseID := serverPathID(request)
	current, err := router.ServerStore.FindServerLicenseByID(request.Context(), applicationID, licenseID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	base := current.ExpiresAt
	if now := time.Now().UTC(); !base.After(now) {
		base = now
	}
	expiresAt := base.AddDate(0, 0, body.DurationDays)
	maxDevices, level := current.MaxDevices, current.Level
	if body.MaxDevices != nil {
		maxDevices = *body.MaxDevices
	}
	if body.Level != nil {
		level = *body.Level
	}
	notes := current.Notes
	if strings.TrimSpace(body.Notes) != "" {
		notes = strings.TrimSpace(body.Notes)
	}
	if err := router.ServerStore.AdminUpdateLicense(request.Context(), applicationID, licenseID, expiresAt, maxDevices, level, notes); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK        bool   `json:"ok"`
		ExpiresAt string `json:"expires_at"`
	}{OK: true, ExpiresAt: expiresAt.UTC().Format(time.RFC3339)})
}
