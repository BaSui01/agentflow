package llm

import (
	"context"
	"sync"
	"time"
)

// Handler processes a request and returns a response.
type Handler func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

// Middleware wraps a handler with additional functionality.
type Middleware func(next Handler) Handler

// Chain represents a middleware chain.
type Chain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewChain creates a new middleware chain.
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Use adds middleware to the chain.
func (c *Chain) Use(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, m)
	return c
}

// Then wraps a handler with all middleware.
func (c *Chain) Then(h Handler) Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Len returns the number of middleware.
func (c *Chain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.middlewares)
}

// LoggingMiddleware logs request/response details.
func LoggingMiddleware(logger func(format string, args ...any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
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
		return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// RecoveryMiddleware recovers from panics.
func RecoveryMiddleware(onPanic func(any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *ChatRequest) (resp *ChatResponse, err error) {
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

// MetricsMiddleware collects request metrics.
func MetricsMiddleware(collector MetricsCollector) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
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
