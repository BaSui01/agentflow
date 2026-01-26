// Package middleware provides extensible middleware chain for LLM requests.
package middleware

import (
	"context"
	"sync"
	"time"
)

// Request represents an LLM request passing through middleware.
type Request struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Tools       []Tool            `json:"tools,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool represents a tool definition.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Response represents an LLM response passing through middleware.
type Response struct {
	Content      string         `json:"content"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
	Usage        Usage          `json:"usage"`
	Model        string         `json:"model"`
	FinishReason string         `json:"finish_reason"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// ToolCall represents a tool call in response.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage represents token usage.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Handler processes a request and returns a response.
type Handler func(ctx context.Context, req *Request) (*Response, error)

// Middleware wraps a handler with additional functionality.
type Middleware func(next Handler) Handler

// Chain represents a middleware chain.
type Chain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewChain creates a new middleware chain.
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Use adds middleware to the chain.
func (c *Chain) Use(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, m)
	return c
}

// UseFront adds middleware to the front of the chain.
func (c *Chain) UseFront(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append([]Middleware{m}, c.middlewares...)
	return c
}

// Then wraps a handler with all middleware in the chain.
func (c *Chain) Then(h Handler) Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Apply middleware in reverse order
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Len returns the number of middleware in the chain.
func (c *Chain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.middlewares)
}

// Built-in middleware

// LoggingMiddleware logs request/response details.
func LoggingMiddleware(logger func(format string, args ...any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			start := time.Now()
			logger("[LLM] Request: model=%s messages=%d", req.Model, len(req.Messages))

			resp, err := next(ctx, req)

			duration := time.Since(start)
			if err != nil {
				logger("[LLM] Error: %v duration=%v", err, duration)
			} else {
				logger("[LLM] Response: tokens=%d duration=%v", resp.Usage.TotalTokens, duration)
			}

			return resp, err
		}
	}
}

// TimeoutMiddleware adds timeout to requests.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// RetryMiddleware retries failed requests.
func RetryMiddleware(maxRetries int, backoff time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			var lastErr error
			for i := 0; i <= maxRetries; i++ {
				resp, err := next(ctx, req)
				if err == nil {
					return resp, nil
				}
				lastErr = err

				if i < maxRetries {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(backoff * time.Duration(i+1)):
					}
				}
			}
			return nil, lastErr
		}
	}
}

// MetricsMiddleware collects request metrics.
func MetricsMiddleware(collector MetricsCollector) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			collector.RecordRequest(req.Model, duration, err == nil)
			if resp != nil {
				collector.RecordTokens(req.Model, resp.Usage.TotalTokens)
			}

			return resp, err
		}
	}
}

// MetricsCollector defines metrics collection interface.
type MetricsCollector interface {
	RecordRequest(model string, duration time.Duration, success bool)
	RecordTokens(model string, tokens int)
}

// HeadersMiddleware adds custom headers to requests.
func HeadersMiddleware(headers map[string]string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			for k, v := range headers {
				req.Headers[k] = v
			}
			return next(ctx, req)
		}
	}
}

// CacheMiddleware caches responses.
func CacheMiddleware(cache Cache) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			// Skip cache for streaming requests
			if req.Stream {
				return next(ctx, req)
			}

			key := cache.Key(req)
			if cached, ok := cache.Get(key); ok {
				return cached, nil
			}

			resp, err := next(ctx, req)
			if err == nil {
				cache.Set(key, resp)
			}

			return resp, err
		}
	}
}

// Cache defines caching interface.
type Cache interface {
	Key(req *Request) string
	Get(key string) (*Response, bool)
	Set(key string, resp *Response)
}

// RateLimitMiddleware applies rate limiting.
func RateLimitMiddleware(limiter RateLimiter) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}
}

// RateLimiter defines rate limiting interface.
type RateLimiter interface {
	Wait(ctx context.Context) error
}

// RecoveryMiddleware recovers from panics.
func RecoveryMiddleware(onPanic func(any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (resp *Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(r)
					}
					err = &PanicError{Value: r}
				}
			}()
			return next(ctx, req)
		}
	}
}

// PanicError represents a recovered panic.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return "panic recovered"
}

// TracingMiddleware adds distributed tracing.
func TracingMiddleware(tracer Tracer) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			ctx, span := tracer.Start(ctx, "llm.request")
			defer span.End()

			span.SetAttribute("model", req.Model)
			span.SetAttribute("messages", len(req.Messages))

			resp, err := next(ctx, req)

			if err != nil {
				span.SetError(err)
			} else if resp != nil {
				span.SetAttribute("tokens", resp.Usage.TotalTokens)
			}

			return resp, err
		}
	}
}

// Tracer defines tracing interface.
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, Span)
}

// Span defines a trace span.
type Span interface {
	SetAttribute(key string, value any)
	SetError(err error)
	End()
}

// ValidatorMiddleware validates requests before processing.
func ValidatorMiddleware(validators ...Validator) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			for _, v := range validators {
				if err := v.Validate(req); err != nil {
					return nil, err
				}
			}
			return next(ctx, req)
		}
	}
}

// Validator defines request validation interface.
type Validator interface {
	Validate(req *Request) error
}

// TransformMiddleware transforms requests/responses.
func TransformMiddleware(reqTransform func(*Request), respTransform func(*Response)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			if reqTransform != nil {
				reqTransform(req)
			}

			resp, err := next(ctx, req)

			if err == nil && respTransform != nil {
				respTransform(resp)
			}

			return resp, err
		}
	}
}
