package adminapi

import (
	"net/http"

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
