package gateway

import (
	"strings"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func validateRequest(req *llmcore.UnifiedRequest) *types.Error {
	if req == nil {
		return types.NewInvalidRequestError("request is required")
	}
	if strings.TrimSpace(string(req.Capability)) == "" {
		return types.NewInvalidRequestError("capability is required")
	}
	if req.Payload == nil {
		return types.NewInvalidRequestError("payload is required")
	}
	return nil
}

func normalizeRequest(req *llmcore.UnifiedRequest) {
	req.ProviderHint = strings.TrimSpace(req.ProviderHint)
	req.ModelHint = strings.TrimSpace(req.ModelHint)
	req.TraceID = strings.TrimSpace(req.TraceID)
}
