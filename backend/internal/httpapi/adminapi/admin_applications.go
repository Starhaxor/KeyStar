package adminapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type applicationLifecycleConsole interface {
	ListApplications(ctx context.Context) ([]domain.Application, error)
	UpdateApplication(ctx context.Context, applicationID string, input domain.UpdateApplication) (*domain.Application, error)
	TransitionApplication(ctx context.Context, applicationID string, status domain.ApplicationStatus) (*domain.Application, error)
}

type applicationJSON struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Status          string `json:"status"`
	EnvironmentMode string `json:"environment_mode"`
}

type applicationSigningKeyJSON struct {
	KID         string                             `json:"kid"`
	Algorithm   string                             `json:"algorithm"`
	Status      domain.ApplicationSigningKeyStatus `json:"status"`
	PublicKey   []byte                             `json:"public_key"`
	CreatedAt   string                             `json:"created_at"`
	ActivatedAt *string                            `json:"activated_at"`
	RetireAt    *string                            `json:"retire_at"`
	RevokedAt   *string                            `json:"revoked_at"`
}

func (router *Router) routeAdminApplications(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsRead) {
			return
		}
		router.handleAdminApplicationList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsWrite) {
			return
		}
		router.handleAdminApplicationCreate(writer, request, account)
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsWrite) {
			return
		}
		router.handleAdminApplicationUpdate(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "transition" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsWrite) {
			return
		}
		router.handleAdminApplicationTransition(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "signing-keys" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsRead) {
			return
		}
		router.handleAdminApplicationSigningKeys(writer, request, segments[1])
	case len(segments) == 2 && segments[1] == "organizations" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsRead) {
			return
		}
		router.handleAdminOrganizationList(writer, request)
	case len(segments) == 2 && segments[1] == "organizations" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermApplicationsWrite) {
			return
		}
		router.handleAdminOrganizationCreate(writer, request, account)
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) applicationLifecycleConsole(writer http.ResponseWriter, request *http.Request) (applicationLifecycleConsole, bool) {
	console, ok := router.Admin.Console.(applicationLifecycleConsole)
	if !ok {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
	return console, ok
}

func (router *Router) writeLifecycleError(writer http.ResponseWriter, request *http.Request, err error) {
	var conflict *domain.ConflictError
	switch {
	case errors.As(err, &conflict):
		httpapi.WriteError(writer, request, http.StatusConflict, conflict.Code(), conflict.Error())
	case errors.Is(err, domain.ErrApplicationNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "APPLICATION_NOT_FOUND", "application not found")
	case errors.Is(err, domain.ErrProductNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "PRODUCT_NOT_FOUND", "product not found")
	case errors.Is(err, domain.ErrPlanNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "PLAN_NOT_FOUND", "plan not found")
	case errors.Is(err, domain.ErrApplicationExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "APPLICATION_ALREADY_EXISTS", "an application with this slug already exists")
	case errors.Is(err, domain.ErrInvalidApplicationUpdate), errors.Is(err, domain.ErrInvalidApplicationTransition):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_APPLICATION", "invalid application lifecycle request")
	case errors.Is(err, domain.ErrInvalidCatalogStatus):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_CATALOG", "invalid catalog lifecycle request")
	default:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}

func (router *Router) findAdminApplication(writer http.ResponseWriter, request *http.Request, console applicationLifecycleConsole, applicationID string) (*domain.Application, bool) {
	applications, err := console.ListApplications(request.Context())
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return nil, false
	}
	for index := range applications {
		if applications[index].ID == applicationID {
			return &applications[index], true
		}
	}
	router.writeLifecycleError(writer, request, domain.ErrApplicationNotFound)
	return nil, false
}

func (router *Router) handleAdminApplicationUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, applicationID string) {
	var body struct {
		Name *string `json:"name"`
		Slug *string `json:"slug"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	console, ok := router.applicationLifecycleConsole(writer, request)
	if !ok {
		return
	}
	before, ok := router.findAdminApplication(writer, request, console, applicationID)
	if !ok {
		return
	}
	application, err := console.UpdateApplication(request.Context(), applicationID, domain.UpdateApplication{Name: body.Name, Slug: body.Slug})
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "APPLICATION_UPDATED", "application", application.ID, map[string]any{"before": applicationAuditState(before), "after": applicationAuditState(application)})
	writeAdminApplication(writer, http.StatusOK, application)
}

func (router *Router) handleAdminApplicationTransition(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, applicationID string) {
	var body struct {
		Status domain.ApplicationStatus `json:"status"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	console, ok := router.applicationLifecycleConsole(writer, request)
	if !ok {
		return
	}
	before, ok := router.findAdminApplication(writer, request, console, applicationID)
	if !ok {
		return
	}
	application, err := console.TransitionApplication(request.Context(), applicationID, body.Status)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "APPLICATION_TRANSITIONED", "application", application.ID, map[string]any{"before": applicationAuditState(before), "after": applicationAuditState(application)})
	writeAdminApplication(writer, http.StatusOK, application)
}

func writeAdminApplication(writer http.ResponseWriter, status int, application *domain.Application) {
	httpapi.WriteJSON(writer, status, struct {
		OK          bool            `json:"ok"`
		Application applicationJSON `json:"application"`
	}{OK: true, Application: applicationJSON{ID: application.ID, OrganizationID: application.OrganizationID, Name: application.Name, Slug: application.Slug, Status: string(application.Status), EnvironmentMode: application.EnvironmentMode}})
}

func applicationAuditState(application *domain.Application) map[string]string {
	return map[string]string{"name": application.Name, "slug": application.Slug, "status": string(application.Status), "environment_mode": application.EnvironmentMode}
}

func (router *Router) handleAdminApplicationList(writer http.ResponseWriter, request *http.Request) {
	applications, err := router.Admin.Console.ListApplications(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]applicationJSON, 0, len(applications))
	for _, application := range applications {
		items = append(items, applicationJSON{ID: application.ID, OrganizationID: application.OrganizationID, Name: application.Name, Slug: application.Slug, Status: string(application.Status), EnvironmentMode: application.EnvironmentMode})
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool              `json:"ok"`
		Items []applicationJSON `json:"items"`
	}{OK: true, Items: items})
}

func (router *Router) handleAdminApplicationCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		OrganizationID string `json:"organization_id"`
		Name           string `json:"name"`
		Slug           string `json:"slug"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil || strings.TrimSpace(body.OrganizationID) == "" || strings.TrimSpace(body.Name) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "organization_id and name are required")
		return
	}
	if router.Admin.ApplicationProvisioner == nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	application, err := router.Admin.ApplicationProvisioner.Create(request.Context(), domain.NewApplication{OrganizationID: body.OrganizationID, Name: body.Name, Slug: body.Slug})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "APPLICATION_CREATED", "application", application.ID, nil)
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK          bool            `json:"ok"`
		Application applicationJSON `json:"application"`
	}{OK: true, Application: applicationJSON{ID: application.ID, OrganizationID: application.OrganizationID, Name: application.Name, Slug: application.Slug, Status: string(application.Status), EnvironmentMode: application.EnvironmentMode}})
}

func (router *Router) handleAdminApplicationSigningKeys(writer http.ResponseWriter, request *http.Request, applicationID string) {
	console, ok := router.applicationLifecycleConsole(writer, request)
	if !ok {
		return
	}
	if _, ok := router.findAdminApplication(writer, request, console, applicationID); !ok {
		return
	}
	if router.Admin.ApplicationSigningKeys == nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	keys, err := router.Admin.ApplicationSigningKeys.ListApplicationSigningKeys(request.Context(), applicationID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	items := make([]applicationSigningKeyJSON, 0, len(keys))
	for _, key := range keys {
		items = append(items, applicationSigningKeyJSON{
			KID: key.KID, Algorithm: key.Algorithm, Status: key.Status,
			PublicKey: key.PublicKey, CreatedAt: httpapi.FormatTime(key.CreatedAt),
			ActivatedAt: httpapi.FormatOptionalTime(key.ActivatedAt), RetireAt: httpapi.FormatOptionalTime(key.RetireAt),
			RevokedAt: httpapi.FormatOptionalTime(key.RevokedAt),
		})
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool                        `json:"ok"`
		Items []applicationSigningKeyJSON `json:"items"`
	}{OK: true, Items: items})
}

func (router *Router) handleAdminOrganizationList(writer http.ResponseWriter, request *http.Request) {
	items, err := router.Admin.Console.ListOrganizations(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool                  `json:"ok"`
		Items []domain.Organization `json:"items"`
	}{OK: true, Items: items})
}

func (router *Router) handleAdminOrganizationCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "name is required")
		return
	}
	organization, err := router.Admin.Console.CreateOrganization(request.Context(), body.Name)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "ORGANIZATION_CREATED", "organization", organization.ID, nil)
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK           bool                 `json:"ok"`
		Organization *domain.Organization `json:"organization"`
	}{OK: true, Organization: organization})
}
