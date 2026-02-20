package llm

import (
	"context"
	"time"
)

// 扩展提供了可选的框架扩展接口.
// 所有接口都有"NoOp"执行,允许用户根据需要注入真正的执行.

// 安全认证

// 身份代表代理人或用户身份
type Identity struct {
	ID          string
	Type        string // "agent", "user", "service"
	Permissions []string
	Roles       []string
	Metadata    map[string]interface{}
}

// 提供认证和授权
type SecurityProvider interface {
	// 认证证书和返回身份
	Authenticate(ctx context.Context, credentials interface{}) (*Identity, error)
	
	// 授权检查身份是否有资源/行动许可
	Authorize(ctx context.Context, identity *Identity, resource string, action string) error
}

// NoOp Security Provider 是一个不执行
type NoOpSecurityProvider struct{}

func (n *NoOpSecurityProvider) Authenticate(ctx context.Context, credentials interface{}) (*Identity, error) {
	return &Identity{ID: "anonymous", Type: "user"}, nil
}

func (n *NoOpSecurityProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) error {
	return nil
}

// 审计记录

// 审计工作代表可审计事件
type AuditEvent struct {
	Timestamp  time.Time
	EventType  string // "agent.execute", "tool.call", "provider.request"
	ActorID    string
	ActorType  string
	Resource   string
	Action     string
	Result     string // "success", "failure"
	Error      string
	Metadata   map[string]interface{}
}

// 审计
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
}

// 审计过滤器过滤审计日志查询
type AuditFilter struct {
	StartTime  time.Time
	EndTime    time.Time
	EventTypes []string
	ActorID    string
	Resource   string
}

// NoOpAudit Logger 是无执行
type NoOpAuditLogger struct{}

func (n *NoOpAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	return nil
}

func (n *NoOpAuditLogger) Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	return []AuditEvent{}, nil
}

// · 限制费率

// 百分位控制请求率
type RateLimiter interface {
	// 如果允许请求, 允许检查
	Allow(ctx context.Context, key string) (bool, error)
	
	// 允许检查 N 请求是否被允许
	AllowN(ctx context.Context, key string, n int) (bool, error)
	
	// 重置密钥的速率限制
	Reset(ctx context.Context, key string) error
}

// NoOpRateLimiter 是一个不执行
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

// 分配追踪

// Span 表示跟踪跨度
type Span interface {
	SetAttribute(key string, value interface{})
	AddEvent(name string, attributes map[string]interface{})
	End()
}

// 追踪器提供分布式追踪
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// 无 OpSpan 是无线
type NoOpSpan struct{}

func (n *NoOpSpan) SetAttribute(key string, value interface{}) {}
func (n *NoOpSpan) AddEvent(name string, attributes map[string]interface{}) {}
func (n *NoOpSpan) End() {}

// 无Op追踪器是无Op追踪器
type NoOpTracer struct{}

func (n *NoOpTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	return ctx, &NoOpSpan{}
}

// 中间软件系统

// 提供者中间软件包装提供者
type ProviderMiddleware interface {
	Wrap(next Provider) Provider
}

// ProverMiddlewareFunc 是 ProverMiddleware 的函数适配器
type ProviderMiddlewareFunc func(Provider) Provider

func (f ProviderMiddlewareFunc) Wrap(next Provider) Provider {
	return f(next)
}

// 链式 ProviderMiddleware 链式 多个中件
func ChainProviderMiddleware(provider Provider, middlewares ...ProviderMiddleware) Provider {
	for i := len(middlewares) - 1; i >= 0; i-- {
		provider = middlewares[i].Wrap(provider)
	}
	return provider
}
