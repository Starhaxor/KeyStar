package adminapi

import (
	"net/http"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

func (router *Router) routeAdminProducts(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	if !router.RequirePermission(writer, request, account, domain.PermCatalogRead) {
		return
	}
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		router.handleAdminProductList(writer, request)
	case len(segments) == 3 && segments[2] == "plans" && request.Method == http.MethodGet:
		router.handleAdminPlanList(writer, request, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminProductList(writer http.ResponseWriter, request *http.Request) {
	items, err := router.Admin.Console.ListProducts(request.Context(), router.AdminApplicationID(request))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool             `json:"ok"`
		Items []domain.Product `json:"items"`
	}{OK: true, Items: items})
}

func (router *Router) handleAdminPlanList(writer http.ResponseWriter, request *http.Request, productID string) {
	product, err := router.Admin.Console.FindProductByID(request.Context(), router.AdminApplicationID(request), productID)
	if err != nil || product == nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items, err := router.Admin.Console.ListPlans(request.Context(), product.ID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool          `json:"ok"`
		Items []domain.Plan `json:"items"`
	}{OK: true, Items: items})
}
