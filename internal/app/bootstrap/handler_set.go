package bootstrap

import (
	"github.com/BaSui01/agentflow/api/handlers"
)

// HTTPHandlerSet aggregates all HTTP handlers built at startup.
// This struct has a single responsibility: hold handler references.
type HTTPHandlerSet struct {
	HealthHandler       *handlers.HealthHandler
	ChatHandler         *handlers.ChatHandler
	AgentHandler        *handlers.AgentHandler
	APIKeyHandler       *handlers.APIKeyHandler
	ToolRegistryHandler *handlers.ToolRegistryHandler
	ToolProviderHandler *handlers.ToolProviderHandler
	ToolApprovalHandler *handlers.ToolApprovalHandler
	AuthAuditHandler    *handlers.AuthorizationAuditHandler
	RAGHandler          *handlers.RAGHandler
	WorkflowHandler     *handlers.WorkflowHandler
	ProtocolHandler     *handlers.ProtocolHandler
	MultimodalHandler   *handlers.MultimodalHandler
	CostHandler         *handlers.CostHandler
}

// Count returns the number of non-nil handlers in the set.
func (s *HTTPHandlerSet) Count() int {
	count := 0
	if s.HealthHandler != nil {
		count++
	}
	if s.ChatHandler != nil {
		count++
	}
	if s.AgentHandler != nil {
		count++
	}
	if s.APIKeyHandler != nil {
		count++
	}
	if s.ToolRegistryHandler != nil {
		count++
	}
	if s.ToolProviderHandler != nil {
		count++
	}
	if s.ToolApprovalHandler != nil {
		count++
	}
	if s.AuthAuditHandler != nil {
		count++
	}
	if s.RAGHandler != nil {
		count++
	}
	if s.WorkflowHandler != nil {
		count++
	}
	if s.ProtocolHandler != nil {
		count++
	}
	if s.MultimodalHandler != nil {
		count++
	}
	if s.CostHandler != nil {
		count++
	}
	return count
}
