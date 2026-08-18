package httpapi

import (
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
)

func mapServerVariables(variables []domain.Variable) []variableJSON {
	result := make([]variableJSON, 0, len(variables))
	for _, variable := range variables {
		result = append(result, variableJSON{
			ID: variable.ID, Key: variable.Key, Value: variable.Value,
			Description: variable.Description,
			CreatedAt:   formatTime(variable.CreatedAt), UpdatedAt: formatTime(variable.UpdatedAt),
		})
	}
	return result
}

func (router *Router) handleServerVariableList(writer http.ResponseWriter, request *http.Request) {
	variables, err := router.serverStore.ListVariables(request.Context(), principalApplicationID(request))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK   bool           `json:"ok"`
		Data []variableJSON `json:"data"`
	}{OK: true, Data: mapServerVariables(variables)})
}

type createServerVariableRequestBody struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

func (router *Router) handleServerVariableCreate(writer http.ResponseWriter, request *http.Request) {
	var body createServerVariableRequestBody
	if err := decodeAdminJSONBody(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	key := strings.TrimSpace(body.Key)
	if key == "" || len(key) > 128 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	variable, err := router.serverStore.CreateVariable(request.Context(), principalApplicationID(request), key, body.Value, strings.TrimSpace(body.Description))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, struct {
		OK   bool          `json:"ok"`
		Data variableJSON  `json:"data"`
	}{OK: true, Data: variableJSON{
		ID: variable.ID, Key: variable.Key, Value: variable.Value,
		Description: variable.Description,
		CreatedAt:   formatTime(variable.CreatedAt), UpdatedAt: formatTime(variable.UpdatedAt),
	}})
}

type updateServerVariableRequestBody struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

func (router *Router) handleServerVariableUpdate(writer http.ResponseWriter, request *http.Request) {
	var body updateServerVariableRequestBody
	if err := decodeAdminJSONBody(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if err := router.serverStore.UpdateVariable(request.Context(), principalApplicationID(request), serverPathID(request), body.Value, strings.TrimSpace(body.Description)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleServerVariableDelete(writer http.ResponseWriter, request *http.Request) {
	if err := router.serverStore.DeleteVariable(request.Context(), principalApplicationID(request), serverPathID(request)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
