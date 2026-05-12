package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
)

func compatErrorStatus(err *types.Error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	if err.HTTPStatus != 0 {
		return err.HTTPStatus
	}
	if status := api.HTTPStatusFromErrorCode(err.Code); status != 0 {
		return status
	}
	return http.StatusInternalServerError
}

func normalizeCompatError(err *types.Error) *types.Error {
	if err != nil {
		return err
	}
	return types.NewInternalError("internal error")
}

func openAICompatErrorEnvelopeFromTypes(err *types.Error) (int, openAICompatErrorEnvelope) {
	err = normalizeCompatError(err)
	return compatErrorStatus(err), openAICompatErrorEnvelope{
		Error: openAICompatError{
			Message: err.Message,
			Type:    openAICompatErrorType(err),
			Code:    string(err.Code),
		},
	}
}

func anthropicCompatErrorEnvelopeFromTypes(err *types.Error) (int, anthropicCompatErrorEnvelope) {
	err = normalizeCompatError(err)
	return compatErrorStatus(err), anthropicCompatErrorEnvelope{
		Type: "error",
		Error: anthropicCompatError{
			Type:    anthropicCompatErrorType(err),
			Message: err.Message,
		},
	}
}
