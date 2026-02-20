package llm

import (
	"context"
	"sync"
	"time"
)

// Handler 处理请求并返回响应.
type Handler func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

// Middleware 用附加功能包装处理器.
type Middleware func(next Handler) Handler

// Chain 表示中间件链.
type Chain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewChain 创建新的中间件链.
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// Use 将中间件添加到链中。
func (c *Chain) Use(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, m)
	return c
}

// Then 用所有中间件包装处理器.
func (c *Chain) Then(h Handler) Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Len 返回中间件的数量。
func (c *Chain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.middlewares)
}

// LoggingMiddleware 记录请求/响应详情。
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

// TimeoutMiddleware 为请求添加超时控制.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// RecoveryMiddleware 从 panic 中恢复.
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

// PanicError 表示已恢复的 panic。
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return "panic recovered"
}

// MetricsMiddleware 收集请求指标.
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

// MetricsCollector 定义指标收集接口.
type MetricsCollector interface {
	RecordRequest(model string, duration time.Duration, success bool)
	RecordTokens(model string, tokens int)
}
