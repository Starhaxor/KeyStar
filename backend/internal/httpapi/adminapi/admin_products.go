package adminapi

import (
	"net/http"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

func (router *Router) routeAdminProducts(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogRead) {
			return
		}
		router.handleAdminProductList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminProductCreate(writer, request, account)
	case len(segments) == 3 && segments[2] == "plans" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogRead) {
			return
		}
		router.handleAdminPlanList(writer, request, segments[1])
	case len(segments) == 3 && segments[2] == "plans" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminPlanCreate(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminProductCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	product, err := router.Admin.Console.CreateProduct(request.Context(), router.AdminApplicationID(request), domain.NewProduct{Name: body.Name, Slug: body.Slug})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PRODUCT_CREATED", "product", product.ID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool            `json:"ok"`
		Product *domain.Product `json:"product"`
	}{true, product})
}

func (router *Router) handleAdminPlanCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, productID string) {
	var body struct {
		Name       string `json:"name"`
		Code       string `json:"code"`
		Level      int    `json:"level"`
		MaxDevices int    `json:"max_devices"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil || body.Name == "" || body.Code == "" || body.MaxDevices < 1 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	product, err := router.Admin.Console.FindProductByID(request.Context(), router.AdminApplicationID(request), productID)
	if err != nil || product == nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	plan, err := router.Admin.Console.CreatePlan(request.Context(), domain.NewPlan{ProductID: product.ID, Name: body.Name, Code: body.Code, Level: body.Level, MaxDevices: body.MaxDevices})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PLAN_CREATED", "plan", plan.ID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool         `json:"ok"`
		Plan *domain.Plan `json:"plan"`
	}{true, plan})
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
