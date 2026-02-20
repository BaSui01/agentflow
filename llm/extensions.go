package llm

import (
	"context"
	"time"
)

// 扩展提供了可选的框架扩展接口。
// 所有接口都有 NoOp 实现，允许用户根据需要注入真正的实现。

// == 安全认证 ==

// Identity 表示代理或用户身份。
type Identity struct {
	ID          string
	Type        string // "agent", "user", "service"
	Permissions []string
	Roles       []string
	Metadata    map[string]any
}

// SecurityProvider 提供认证和授权功能。
type SecurityProvider interface {
	// Authenticate 验证凭证并返回身份。
	Authenticate(ctx context.Context, credentials any) (*Identity, error)
	
	// Authorize 检查身份是否有对资源执行操作的权限。
	Authorize(ctx context.Context, identity *Identity, resource string, action string) error
}

// NoOpSecurityProvider 是空操作实现。
type NoOpSecurityProvider struct{}

func (n *NoOpSecurityProvider) Authenticate(ctx context.Context, credentials any) (*Identity, error) {
	return &Identity{ID: "anonymous", Type: "user"}, nil
}

func (n *NoOpSecurityProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) error {
	return nil
}

// == 审计日志 ==

// AuditEvent 表示可审计的事件。
type AuditEvent struct {
	Timestamp  time.Time
	EventType  string // "agent.execute", "tool.call", "provider.request"
	ActorID    string
	ActorType  string
	Resource   string
	Action     string
	Result     string // "success", "failure"
	Error      string
	Metadata   map[string]any
}

// AuditLogger 提供审计日志记录功能（框架级扩展点）。
//
// 注意：项目中存在三个 AuditLogger 接口，各自服务不同领域，无法统一：
//   - llm.AuditLogger（本接口）         — 框架级，记录 AuditEvent（通用事件）
//   - llm/tools.AuditLogger             — 工具层，记录 *AuditEntry（工具调用/权限/成本），含 LogAsync/Close
//   - agent/guardrails.AuditLogger      — 护栏层，记录 *AuditLogEntry（验证失败/PII/注入），含 Count
//
// 三者的事件类型、过滤器结构和方法签名均不同，统一会导致接口膨胀。
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
}

// AuditFilter 用于过滤审计日志查询。
type AuditFilter struct {
	StartTime  time.Time
	EndTime    time.Time
	EventTypes []string
	ActorID    string
	Resource   string
}

// NoOpAuditLogger 是空操作实现。
type NoOpAuditLogger struct{}

func (n *NoOpAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	return nil
}

func (n *NoOpAuditLogger) Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	return []AuditEvent{}, nil
}

// == 速率限制 ==

// RateLimiter 控制请求速率。
type RateLimiter interface {
	// Allow 检查是否允许该请求。
	Allow(ctx context.Context, key string) (bool, error)
	
	// AllowN 检查是否允许 N 个请求。
	AllowN(ctx context.Context, key string, n int) (bool, error)
	
	// Reset 重置指定 key 的速率限制。
	Reset(ctx context.Context, key string) error
}

// NoOpRateLimiter 是空操作实现。
type NoOpRateLimiter struct{}

func (n *NoOpRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return true, nil
}

func (r *NoOpRateLimiter) AllowN(ctx context.Context, key string, count int) (bool, error) {
	return true, nil
}

func (n *NoOpRateLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

// == 分布式追踪 ==

// Span 表示一个追踪 span。
type Span interface {
	SetAttribute(key string, value any)
	AddEvent(name string, attributes map[string]any)
	SetError(err error)
	End()
}

// Tracer 提供分布式追踪功能。
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// NoOpSpan 是空操作实现。
type NoOpSpan struct{}

func (n *NoOpSpan) SetAttribute(key string, value any) {}
func (n *NoOpSpan) AddEvent(name string, attributes map[string]any) {}
func (n *NoOpSpan) SetError(err error) {}
func (n *NoOpSpan) End() {}

// NoOpTracer 是空操作实现。
type NoOpTracer struct{}

func (n *NoOpTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	return ctx, &NoOpSpan{}
}

// == 中间件系统 ==

// ProviderMiddleware 包装提供者以添加额外功能。
type ProviderMiddleware interface {
	Wrap(next Provider) Provider
}

// ProviderMiddlewareFunc 是 ProviderMiddleware 的函数适配器。
type ProviderMiddlewareFunc func(Provider) Provider

func (f ProviderMiddlewareFunc) Wrap(next Provider) Provider {
	return f(next)
}

// ChainProviderMiddleware 将多个中间件串联起来。
func ChainProviderMiddleware(provider Provider, middlewares ...ProviderMiddleware) Provider {
	for i := len(middlewares) - 1; i >= 0; i-- {
		provider = middlewares[i].Wrap(provider)
	}
	return provider
}
