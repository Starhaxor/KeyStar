package adminapi

import (
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type applicationJSON struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organization_id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Status          string `json:"status"`
	EnvironmentMode string `json:"environment_mode"`
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
	application, err := router.Admin.Console.CreateApplication(request.Context(), domain.NewApplication{OrganizationID: body.OrganizationID, Name: body.Name, Slug: body.Slug})
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
