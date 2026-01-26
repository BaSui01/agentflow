package llm

import (
	"context"
	"time"
)

// Extensions provides optional framework extension interfaces.
// All interfaces have NoOp implementations, allowing users to inject real implementations as needed.

// ====== Security & Authentication ======

// Identity represents an agent or user identity
type Identity struct {
	ID          string
	Type        string // "agent", "user", "service"
	Permissions []string
	Roles       []string
	Metadata    map[string]interface{}
}

// SecurityProvider provides authentication and authorization
type SecurityProvider interface {
	// Authenticate verifies credentials and returns identity
	Authenticate(ctx context.Context, credentials interface{}) (*Identity, error)
	
	// Authorize checks if identity has permission for resource/action
	Authorize(ctx context.Context, identity *Identity, resource string, action string) error
}

// NoOpSecurityProvider is a no-op implementation
type NoOpSecurityProvider struct{}

func (n *NoOpSecurityProvider) Authenticate(ctx context.Context, credentials interface{}) (*Identity, error) {
	return &Identity{ID: "anonymous", Type: "user"}, nil
}

func (n *NoOpSecurityProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) error {
	return nil
}

// ====== Audit Logging ======

// AuditEvent represents an auditable event
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

// AuditLogger logs audit events
type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
}

// AuditFilter filters audit log queries
type AuditFilter struct {
	StartTime  time.Time
	EndTime    time.Time
	EventTypes []string
	ActorID    string
	Resource   string
}

// NoOpAuditLogger is a no-op implementation
type NoOpAuditLogger struct{}

func (n *NoOpAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	return nil
}

func (n *NoOpAuditLogger) Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	return []AuditEvent{}, nil
}

// ====== Rate Limiting ======

// RateLimiter controls request rates
type RateLimiter interface {
	// Allow checks if request is allowed
	Allow(ctx context.Context, key string) (bool, error)
	
	// AllowN checks if N requests are allowed
	AllowN(ctx context.Context, key string, n int) (bool, error)
	
	// Reset resets rate limit for key
	Reset(ctx context.Context, key string) error
}

// NoOpRateLimiter is a no-op implementation
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

// ====== Distributed Tracing ======

// Span represents a trace span
type Span interface {
	SetAttribute(key string, value interface{})
	AddEvent(name string, attributes map[string]interface{})
	End()
}

// Tracer provides distributed tracing
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// NoOpSpan is a no-op span
type NoOpSpan struct{}

func (n *NoOpSpan) SetAttribute(key string, value interface{}) {}
func (n *NoOpSpan) AddEvent(name string, attributes map[string]interface{}) {}
func (n *NoOpSpan) End() {}

// NoOpTracer is a no-op tracer
type NoOpTracer struct{}

func (n *NoOpTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	return ctx, &NoOpSpan{}
}

// ====== Middleware System ======

// ProviderMiddleware wraps a Provider
type ProviderMiddleware interface {
	Wrap(next Provider) Provider
}

// ProviderMiddlewareFunc is a function adapter for ProviderMiddleware
type ProviderMiddlewareFunc func(Provider) Provider

func (f ProviderMiddlewareFunc) Wrap(next Provider) Provider {
	return f(next)
}

// ChainProviderMiddleware chains multiple middlewares
func ChainProviderMiddleware(provider Provider, middlewares ...ProviderMiddleware) Provider {
	for i := len(middlewares) - 1; i >= 0; i-- {
		provider = middlewares[i].Wrap(provider)
	}
	return provider
}
