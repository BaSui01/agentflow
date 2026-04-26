package usecase

import "github.com/BaSui01/agentflow/types"

// ToTypesAgentError converts an error to *types.Error when needed.
func ToTypesAgentError(err error) *types.Error {
	if err == nil {
		return nil
	}
	if typedErr, ok := err.(*types.Error); ok {
		return typedErr
	}
	return types.NewInternalError("agent operation failed").WithCause(err)
}
