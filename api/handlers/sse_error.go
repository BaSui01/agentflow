package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
)

type sseErrorEnvelope struct {
	Error     *api.ErrorInfo `json:"error"`
	RequestID string         `json:"request_id"`
}

func errorInfoFromTypesError(err *types.Error) *api.ErrorInfo {
	if err == nil {
		return nil
	}
	status := err.HTTPStatus
	if status == 0 {
		status = api.HTTPStatusFromErrorCode(err.Code)
	}
	return api.ErrorInfoFromTypesError(err, status)
}

func writeSSEErrorEvent(w http.ResponseWriter, errInfo *api.ErrorInfo, requestID string) error {
	if errInfo == nil {
		errInfo = api.ErrorInfoFromTypesError(types.NewInternalError("internal error"), http.StatusInternalServerError)
	}
	return writeSSEEventJSON(w, "error", sseErrorEnvelope{Error: errInfo, RequestID: requestID})
}

func writeSSETypesErrorEvent(w http.ResponseWriter, err *types.Error, requestID string) error {
	return writeSSEErrorEvent(w, errorInfoFromTypesError(err), requestID)
}
