package core

import "context"

// Capability 标识统一入口支持的能力类型。
type Capability string

const (
	CapabilityChat       Capability = "chat"
	CapabilityTools      Capability = "tools"
	CapabilityImage      Capability = "image"
	CapabilityVideo      Capability = "video"
	CapabilityAudio      Capability = "audio"
	CapabilityEmbedding  Capability = "embedding"
	CapabilityRerank     Capability = "rerank"
	CapabilityModeration Capability = "moderation"
	CapabilityMusic      Capability = "music"
	CapabilityThreeD     Capability = "threed"
	CapabilityAvatar     Capability = "avatar"
)

// RoutePolicy 定义统一路由策略标签。
type RoutePolicy string

const (
	RoutePolicyCostFirst    RoutePolicy = "cost_first"
	RoutePolicyHealthFirst  RoutePolicy = "health_first"
	RoutePolicyLatencyFirst RoutePolicy = "latency_first"
	RoutePolicyBalanced     RoutePolicy = "balanced"
	RoutePolicyQualityFirst RoutePolicy = "quality_first" // 质量优先（倾向低幻觉模型）
)

// Gateway 定义 LLM 统一入口。
type Gateway interface {
	Invoke(ctx context.Context, req *UnifiedRequest) (*UnifiedResponse, error)
	Stream(ctx context.Context, req *UnifiedRequest) (<-chan UnifiedChunk, error)
}

// ChatRerankBinding 定义 chat provider 到 rerank provider 的显式绑定关系。
type ChatRerankBinding struct {
	ChatProvider   string `json:"chat_provider"`
	RerankProvider string `json:"rerank_provider"`
}

// RerankProviderResolver 定义 rerank provider 解析契约，避免上层依赖 provider 私有实现细节。
type RerankProviderResolver interface {
	ResolveRerankProvider(chatProvider string) string
}
