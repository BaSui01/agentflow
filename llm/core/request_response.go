package core

import "github.com/BaSui01/agentflow/types"

// UnifiedRequest 是统一入口请求结构。
type UnifiedRequest struct {
	Capability   Capability        `json:"capability"`
	ProviderHint string            `json:"provider_hint,omitempty"`
	ModelHint    string            `json:"model_hint,omitempty"`
	RoutePolicy  RoutePolicy       `json:"route_policy,omitempty"`
	TraceID      string            `json:"trace_id,omitempty"`
	Payload      any               `json:"payload,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

// ProviderDecision 记录路由最终决策。
type ProviderDecision struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Strategy string `json:"strategy,omitempty"`
}

// UnifiedResponse 是统一入口同步响应结构。
type UnifiedResponse struct {
	Output           any              `json:"output,omitempty"`
	Usage            Usage            `json:"usage"`
	Cost             Cost             `json:"cost"`
	TraceID          string           `json:"trace_id,omitempty"`
	ProviderDecision ProviderDecision `json:"provider_decision,omitempty"`
}

// UnifiedChunk 是统一入口流式响应块。
type UnifiedChunk struct {
	Output           any              `json:"output,omitempty"`
	Usage            *Usage           `json:"usage,omitempty"`
	Cost             *Cost            `json:"cost,omitempty"`
	TraceID          string           `json:"trace_id,omitempty"`
	ProviderDecision ProviderDecision `json:"provider_decision,omitempty"`
	Done             bool             `json:"done,omitempty"`
	Err              *types.Error     `json:"error,omitempty"`
}
