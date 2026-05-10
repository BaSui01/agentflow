package handlers

import (
	"net/http"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func requireMethod(w http.ResponseWriter, r *http.Request, expected string, logger *zap.Logger) bool {
	if r != nil && r.Method == expected {
		return true
	}
	if expected != "" {
		w.Header().Set("Allow", expected)
	}
	WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", logger)
	return false
}
