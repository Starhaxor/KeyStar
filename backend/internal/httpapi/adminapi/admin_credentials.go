package adminapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type credentialJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Environment string   `json:"environment"`
	Type        string   `json:"type"`
	Scopes      []string `json:"scopes"`
	KeyPrefix   string   `json:"key_prefix"`
	Status      string   `json:"status"`
	LastUsedAt  *string  `json:"last_used_at"`
	ExpiresAt   *string  `json:"expires_at"`
	CreatedAt   string   `json:"created_at"`
}

func mapCredential(entry domain.ApplicationCredential) credentialJSON {
	return credentialJSON{
		ID: entry.ID, Name: entry.Name,
		Environment: string(entry.Environment), Type: string(entry.CredentialType),
		Scopes: append([]string(nil), entry.Scopes...), KeyPrefix: entry.KeyPrefix,
		Status: string(entry.Status), LastUsedAt: httpapi.FormatOptionalTime(entry.LastUsedAt),
		ExpiresAt: httpapi.FormatOptionalTime(entry.ExpiresAt), CreatedAt: httpapi.FormatTime(entry.CreatedAt),
	}
}

func mapCredentials(entries []domain.ApplicationCredential) []credentialJSON {
	result := make([]credentialJSON, 0, len(entries))
	for _, entry := range entries {
		result = append(result, mapCredential(entry))
	}
	return result
}

func (router *Router) handleAdminCredentialList(writer http.ResponseWriter, request *http.Request) {
	credentials, err := router.Admin.Console.ListCredentials(request.Context(), router.AdminApplicationID(request))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK          bool             `json:"ok"`
		Credentials []credentialJSON `json:"credentials"`
	}{
		OK:          true,
		Credentials: mapCredentials(credentials),
	})
}

type createCredentialRequestBody struct {
	Name        string   `json:"name"`
	Environment string   `json:"environment"`
	Type        string   `json:"type"`
	Scopes      []string `json:"scopes"`
	ExpiresIn   string   `json:"expires_in"` // Go duration, e.g. "30d" is not valid Go syntax; use "720h"
}

func (router *Router) handleAdminCredentialCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body createCredentialRequestBody
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 64 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	environment := strings.ToLower(strings.TrimSpace(body.Environment))
	credentialType := strings.ToLower(strings.TrimSpace(body.Type))
	if environment != string(domain.CredentialEnvironmentTest) && environment != string(domain.CredentialEnvironmentLive) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if credentialType != string(domain.CredentialPublishable) && credentialType != string(domain.CredentialSecret) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	scopes := make([]string, 0, len(body.Scopes))
	for _, scope := range body.Scopes {
		scopes = append(scopes, strings.ToLower(strings.TrimSpace(scope)))
	}
	if len(scopes) == 0 {
		// A publishable key with no scope is useless; a secret key must
		// declare its exact permissions explicitly.
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if !credential.ValidScopes(credentialType, scopes) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	var expiresAt *time.Time
	if duration := strings.TrimSpace(body.ExpiresIn); duration != "" {
		parsed, err := time.ParseDuration(duration)
		if err != nil || parsed <= 0 {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
			return
		}
		value := time.Now().UTC().Add(parsed)
		expiresAt = &value
	}

	generated, err := credential.Generate(credentialType, environment, nil)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	created, err := router.Admin.Console.CreateCredential(request.Context(), domain.NewApplicationCredential{
		ApplicationID:  router.AdminApplicationID(request),
		Environment:    domain.CredentialEnvironment(environment),
		CredentialType: domain.CredentialType(credentialType),
		Name:           name,
		KeyPrefix:      generated.Prefix,
		KeyHash:        generated.Hash,
		Scopes:         scopes,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		router.writeCredentialError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "CREDENTIAL_CREATED", "credential", created.ID, map[string]string{
		"name":        created.Name,
		"environment": string(created.Environment),
		"type":        string(created.CredentialType),
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK         bool           `json:"ok"`
		Credential credentialJSON `json:"credential"`
		Key        string         `json:"key"` // shown exactly once
	}{
		OK:         true,
		Credential: mapCredential(*created),
		Key:        generated.Key,
	})
}

func (router *Router) handleAdminCredentialRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, credentialID string) {
	if err := router.Admin.Console.RevokeCredential(request.Context(), router.AdminApplicationID(request), credentialID); err != nil {
		router.writeCredentialError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "CREDENTIAL_REVOKED", "credential", credentialID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) writeCredentialError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrCredentialNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "CREDENTIAL_NOT_FOUND", "credential not found")
	case errors.Is(err, domain.ErrCredentialExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "CREDENTIAL_ALREADY_EXISTS", "a credential with this prefix already exists")
	default:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
