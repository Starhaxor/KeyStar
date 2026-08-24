package adminapi

import (
	"context"
	"net/http"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type catalogLifecycleConsole interface {
	FindProductByID(ctx context.Context, applicationID, productID string) (*domain.Product, error)
	ListPlans(ctx context.Context, productID string) ([]domain.Plan, error)
	UpdateProduct(ctx context.Context, applicationID, productID string, input domain.UpdateProduct) (*domain.Product, error)
	ArchiveProduct(ctx context.Context, applicationID, productID string) (*domain.Product, error)
	UpdatePlan(ctx context.Context, applicationID, productID, planID string, input domain.UpdatePlan) (*domain.Plan, error)
	ArchivePlan(ctx context.Context, applicationID, productID, planID string) (*domain.Plan, error)
}

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
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminProductUpdate(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "archive" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminProductArchive(writer, request, account, segments[1])
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
	case len(segments) == 4 && segments[2] == "plans" && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminPlanUpdate(writer, request, account, segments[1], segments[3])
	case len(segments) == 5 && segments[2] == "plans" && segments[4] == "archive" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCatalogWrite) {
			return
		}
		router.handleAdminPlanArchive(writer, request, account, segments[1], segments[3])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) catalogLifecycleConsole(writer http.ResponseWriter, request *http.Request) (catalogLifecycleConsole, bool) {
	console, ok := router.Admin.Console.(catalogLifecycleConsole)
	if !ok {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
	return console, ok
}

func (router *Router) findAdminPlan(writer http.ResponseWriter, request *http.Request, console catalogLifecycleConsole, productID, planID string) (*domain.Plan, bool) {
	plans, err := console.ListPlans(request.Context(), productID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return nil, false
	}
	for index := range plans {
		if plans[index].ID == planID {
			return &plans[index], true
		}
	}
	router.writeLifecycleError(writer, request, domain.ErrPlanNotFound)
	return nil, false
}

func (router *Router) handleAdminProductUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, productID string) {
	var body struct {
		Name   *string               `json:"name"`
		Slug   *string               `json:"slug"`
		Status *domain.CatalogStatus `json:"status"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	console, ok := router.catalogLifecycleConsole(writer, request)
	if !ok {
		return
	}
	applicationID := router.AdminApplicationID(request)
	before, err := console.FindProductByID(request.Context(), applicationID, productID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	product, err := console.UpdateProduct(request.Context(), applicationID, productID, domain.UpdateProduct{Name: body.Name, Slug: body.Slug, Status: body.Status})
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PRODUCT_UPDATED", "product", product.ID, map[string]any{"before": productAuditState(before), "after": productAuditState(product)})
	writeAdminProduct(writer, product)
}

func (router *Router) handleAdminProductArchive(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, productID string) {
	console, ok := router.catalogLifecycleConsole(writer, request)
	if !ok {
		return
	}
	applicationID := router.AdminApplicationID(request)
	before, err := console.FindProductByID(request.Context(), applicationID, productID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	product, err := console.ArchiveProduct(request.Context(), applicationID, productID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PRODUCT_ARCHIVED", "product", product.ID, map[string]any{"before": productAuditState(before), "after": productAuditState(product)})
	writeAdminProduct(writer, product)
}

func (router *Router) handleAdminPlanUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, productID, planID string) {
	var body struct {
		Name       *string               `json:"name"`
		Code       *string               `json:"code"`
		Level      *int                  `json:"level"`
		MaxDevices *int                  `json:"max_devices"`
		Status     *domain.CatalogStatus `json:"status"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	console, ok := router.catalogLifecycleConsole(writer, request)
	if !ok {
		return
	}
	applicationID := router.AdminApplicationID(request)
	if _, err := console.FindProductByID(request.Context(), applicationID, productID); err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	before, ok := router.findAdminPlan(writer, request, console, productID, planID)
	if !ok {
		return
	}
	plan, err := console.UpdatePlan(request.Context(), applicationID, productID, planID, domain.UpdatePlan{Name: body.Name, Code: body.Code, Level: body.Level, MaxDevices: body.MaxDevices, Status: body.Status})
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PLAN_UPDATED", "plan", plan.ID, map[string]any{"before": planAuditState(before), "after": planAuditState(plan)})
	writeAdminPlan(writer, plan)
}

func (router *Router) handleAdminPlanArchive(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, productID, planID string) {
	console, ok := router.catalogLifecycleConsole(writer, request)
	if !ok {
		return
	}
	applicationID := router.AdminApplicationID(request)
	if _, err := console.FindProductByID(request.Context(), applicationID, productID); err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	before, ok := router.findAdminPlan(writer, request, console, productID, planID)
	if !ok {
		return
	}
	plan, err := console.ArchivePlan(request.Context(), applicationID, productID, planID)
	if err != nil {
		router.writeLifecycleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "PLAN_ARCHIVED", "plan", plan.ID, map[string]any{"before": planAuditState(before), "after": planAuditState(plan)})
	writeAdminPlan(writer, plan)
}

func writeAdminProduct(writer http.ResponseWriter, product *domain.Product) {
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool            `json:"ok"`
		Product *domain.Product `json:"product"`
	}{OK: true, Product: product})
}

func writeAdminPlan(writer http.ResponseWriter, plan *domain.Plan) {
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool         `json:"ok"`
		Plan *domain.Plan `json:"plan"`
	}{OK: true, Plan: plan})
}

func productAuditState(product *domain.Product) map[string]string {
	return map[string]string{"name": product.Name, "slug": product.Slug, "status": string(product.Status)}
}

func planAuditState(plan *domain.Plan) map[string]any {
	return map[string]any{"name": plan.Name, "code": plan.Code, "level": plan.Level, "max_devices": plan.MaxDevices, "status": string(plan.Status)}
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
