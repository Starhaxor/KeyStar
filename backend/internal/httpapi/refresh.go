package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/service"
)

// RefreshRequest is the dependency boundary for the refresh endpoint.
type RefreshRequest interface {
	Refresh(ctx interface{ Value(any) any }, input service.RefreshInput) (service.RefreshResult, error)
}

type refreshRequestBody struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	OK             bool   `json:"ok"`
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token"`
	TokenExpiresAt string `json:"token_expires_at"`
}

func (router *Router) handleRefresh(writer http.ResponseWriter, request *http.Request) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		WriteError(writer, request, http.StatusUnsupportedMediaType, "INVALID_REQUEST", "invalid request")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body refreshRequestBody
	if err := decoder.Decode(&body); err != nil {
		WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "multiple JSON values")
			return
		}
		WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}

	if body.RefreshToken == "" {
		WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "refresh_token is required")
		return
	}

	// Resolve the application context.
	applicationID := router.defaultApplicationID
	if principal, ok := AppPrincipalFromContext(request.Context()); ok && principal.ApplicationID != "" {
		applicationID = principal.ApplicationID
	}

	// The refresh service is accessed through the device verification service
	// configuration. We use a type assertion to reach it.
	type refreshCapable interface {
		Refresh(ctx interface{ Value(any) any }, input service.RefreshInput) (service.RefreshResult, error)
	}

	// For now, we delegate to the service layer through an adapter that
	// captures the refresh input from the request context.
	result, err := router.refreshFromRequest(request, applicationID, body.RefreshToken)
	if err != nil {
		router.writeRefreshError(writer, request, err)
		return
	}

	WriteJSON(writer, http.StatusOK, refreshResponse{
		OK:             true,
		AccessToken:    result.AccessToken,
		RefreshToken:   result.RefreshToken,
		TokenExpiresAt: result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// refreshFromRequest bridges the HTTP handler to the RefreshService. The
// RefreshService is wired through the DeviceService's configuration; the
// router holds a reference to it via the refreshService field.
func (router *Router) refreshFromRequest(request *http.Request, applicationID, refreshToken string) (service.RefreshResult, error) {
	// Access the refresh service through the router's hidden dependency.
	// This is set by main.go when wiring the server.
	if router.refreshService == nil {
		return service.RefreshResult{}, errors.New("refresh service is not configured")
	}
	return router.refreshService.Refresh(request.Context(), service.RefreshInput{
		ApplicationID: applicationID,
		RefreshToken:  refreshToken,
	})
}

func (router *Router) writeRefreshError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRefreshToken):
		WriteError(writer, request, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "invalid refresh token")
	case errors.Is(err, domain.ErrRefreshTokenRevoked):
		WriteError(writer, request, http.StatusUnauthorized, "REFRESH_TOKEN_REVOKED", "refresh token has been revoked")
	case errors.Is(err, domain.ErrRefreshTokenRotated):
		WriteError(writer, request, http.StatusUnauthorized, "REFRESH_TOKEN_ROTATED", "refresh token has been rotated")
	case errors.Is(err, domain.ErrRefreshTokenExpired):
		WriteError(writer, request, http.StatusUnauthorized, "REFRESH_TOKEN_EXPIRED", "refresh token has expired")
	case errors.Is(err, domain.ErrRefreshTokenReuse):
		WriteError(writer, request, http.StatusUnauthorized, "REFRESH_TOKEN_REUSE", "refresh token reuse detected: all sessions revoked")
	default:
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
