package handlers

import (
	"errors"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

func asTypesAPIError(err error, fallbackMessage string) *types.Error {
	if err == nil {
		return nil
	}
	var typed *types.Error
	if errors.As(err, &typed) && typed != nil {
		return typed
	}
	message := fallbackMessage
	if message == "" {
		message = "internal error"
	}
	return types.NewError(types.ErrInternalError, message).WithCause(err)
}

func serviceUnavailableError(component string) *types.Error {
	return types.NewServiceUnavailableError(fmt.Sprintf("%s service is not configured", component)).
		WithHTTPStatus(503)
}
