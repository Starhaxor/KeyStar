package serverapi

import (
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

func mapServerVariables(variables []domain.Variable) []httpapi.VariableJSON {
	result := make([]httpapi.VariableJSON, 0, len(variables))
	for _, variable := range variables {
		result = append(result, httpapi.MapVariable(variable))
	}
	return result
}

func (router *Router) handleServerVariableList(writer http.ResponseWriter, request *http.Request) {
	variables, err := router.ServerStore.ListVariables(request.Context(), principalApplicationID(request))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool                   `json:"ok"`
		Data []httpapi.VariableJSON `json:"data"`
	}{OK: true, Data: mapServerVariables(variables)})
}

type createServerVariableRequestBody struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

func (router *Router) handleServerVariableCreate(writer http.ResponseWriter, request *http.Request) {
	var body createServerVariableRequestBody
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	key := strings.TrimSpace(body.Key)
	if key == "" || len(key) > 128 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	variable, err := router.ServerStore.CreateVariable(request.Context(), principalApplicationID(request), key, body.Value, strings.TrimSpace(body.Description))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK   bool                 `json:"ok"`
		Data httpapi.VariableJSON `json:"data"`
	}{OK: true, Data: httpapi.MapVariable(*variable)})
}

type updateServerVariableRequestBody struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

func (router *Router) handleServerVariableUpdate(writer http.ResponseWriter, request *http.Request) {
	var body updateServerVariableRequestBody
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if err := router.ServerStore.UpdateVariable(request.Context(), principalApplicationID(request), serverPathID(request), body.Value, strings.TrimSpace(body.Description)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleServerVariableDelete(writer http.ResponseWriter, request *http.Request) {
	if err := router.ServerStore.DeleteVariable(request.Context(), principalApplicationID(request), serverPathID(request)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
