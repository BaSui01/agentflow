package middleware

import (
	"context"
	"sync"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
)

// Handler 处理一个请求并返回一个响应.
type Handler func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error)

// Middleware 将处理器包裹并添加额外功能.
type Middleware func(next Handler) Handler

// Chain 表示中间件链.
type Chain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewChain 创建新的中间件链.
func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

// Use 将中间件添加到链中.
func (c *Chain) Use(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, m)
	return c
}

// UseFront 在链的前部添加中间件.
func (c *Chain) UseFront(m Middleware) *Chain {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append([]Middleware{m}, c.middlewares...)
	return c
}

// Then 用链中的所有中间件包裹一个处理器.
func (c *Chain) Then(h Handler) Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 按倒序应用中间件
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

// Len 返回链中的中间件数量.
func (c *Chain) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.middlewares)
}

// 内置中间件

// LoggingMiddleware 记录请求/响应详情.
func LoggingMiddleware(logger func(format string, args ...any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
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

// TimeoutMiddleware 对请求添加超时.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// RetryMiddleware 重试失败的请求.
func RetryMiddleware(maxRetries int, backoff time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
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

// MetricsMiddleware 收集请求的指标.
func MetricsMiddleware(collector MetricsCollector) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
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

// HeadersMiddleware 添加自定义头到请求元数据.
func HeadersMiddleware(headers map[string]string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string)
			}
			for k, v := range headers {
				req.Metadata[k] = v
			}
			return next(ctx, req)
		}
	}
}

// CacheMiddleware 缓存响应.
func CacheMiddleware(cache Cache) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
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

// Cache 定义缓存接口.
type Cache interface {
	Key(req *llmpkg.ChatRequest) string
	Get(key string) (*llmpkg.ChatResponse, bool)
	Set(key string, resp *llmpkg.ChatResponse)
}

// RateLimitMiddleware 应用速率限制.
func RateLimitMiddleware(limiter BlockingRateLimiter) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			if err := limiter.Wait(ctx); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}
}

// BlockingRateLimiter 定义阻塞式速率限制接口.
// 与 llm.RateLimiter（非阻塞式 Allow/AllowN/Reset）不同，
// BlockingRateLimiter 在超出速率时阻塞等待，适用于中间件链场景.
type BlockingRateLimiter interface {
	Wait(ctx context.Context) error
}

// RecoveryMiddleware 从 panic 中恢复.
func RecoveryMiddleware(onPanic func(any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (resp *llmpkg.ChatResponse, err error) {
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

// RecoveredPanic 表示已恢复的 panic.
type PanicError struct {
	Value any
}

func (e *PanicError) Error() string {
	return "panic recovered"
}

// TracingMiddleware 添加分布式追踪.
func TracingMiddleware(tracer llmpkg.Tracer) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			ctx, span := tracer.StartSpan(ctx, "llm.request")
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

// ValidatorMiddleware 在处理前对请求进行验证.
func ValidatorMiddleware(validators ...Validator) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			for _, v := range validators {
				if err := v.Validate(req); err != nil {
					return nil, err
				}
			}
			return next(ctx, req)
		}
	}
}

// Validator 定义请求验证接口.
type Validator interface {
	Validate(req *llmpkg.ChatRequest) error
}

// TransformMiddleware 转换请求/响应.
func TransformMiddleware(reqTransform func(*llmpkg.ChatRequest), respTransform func(*llmpkg.ChatResponse)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
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
