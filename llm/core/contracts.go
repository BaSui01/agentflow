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
)

// Gateway 定义 LLM 统一入口。
type Gateway interface {
	Invoke(ctx context.Context, req *UnifiedRequest) (*UnifiedResponse, error)
	Stream(ctx context.Context, req *UnifiedRequest) (<-chan UnifiedChunk, error)
}
