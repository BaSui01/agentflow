package providers

import "github.com/BaSui01/agentflow/llm"

// ChooseModel selects the model to use based on priority:
// 1. Request model (if specified in ChatRequest)
// 2. Config model (if specified in provider configuration)
// 3. Default model (provider-specific default)
//
// This function implements the model selection logic defined in Requirements 14.1, 14.2, 14.3.
func ChooseModel(req *llm.ChatRequest, configModel string, defaultModel string) string {
	// Priority 1: Request model
	if req != nil && req.Model != "" {
		return req.Model
	}
	
	// Priority 2: Config model
	if configModel != "" {
		return configModel
	}
	
	// Priority 3: Default model
	return defaultModel
}
