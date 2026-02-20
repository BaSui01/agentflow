package llm

import (
	"context"
	"sync"
	"time"
)

// Handler处理一个请求并返回一个响应.
type Handler func(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

// Middleware 将处理器包裹在外加功能.
type Middleware func(next Handler) Handler

// 链条代表了中件链.
type Chain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewChain创建了新的中件链.
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

// 使用将中间软件添加到链中 。
func (c *Chain) Use(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, m)
	return c
}

// 然后用所有中间软件包住一个处理器.
func (c *Chain) Then(h Handler) Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Len 返回中间软件的数量 。
func (c *Chain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.middlewares)
}

// 日志Middleware日志请求/回复细节 。
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

// 超时Middleware 对请求添加超时.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// 恢复Middleware从恐慌中恢复过来.
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

// 恐慌代表了恢复的恐慌。
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return "panic recovered"
}

// MetricsMiddleware 收集请求的度量衡.
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

// Metrics Collector定义了度量衡收集界面.
type MetricsCollector interface {
	RecordRequest(model string, duration time.Duration, success bool)
	RecordTokens(model string, tokens int)
}
