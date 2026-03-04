package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func dummyHandler(resp *llmpkg.ChatResponse, err error) Handler {
	return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
		return resp, err
	}
}

func successHandler() Handler {
	return dummyHandler(&llmpkg.ChatResponse{
		Model: "test",
		Usage: llmpkg.ChatUsage{TotalTokens: 42},
		Choices: []llmpkg.ChatChoice{
			{Index: 0, Message: llmpkg.Message{Content: "ok"}},
		},
	}, nil)
}

func simpleReq() *llmpkg.ChatRequest {
	return &llmpkg.ChatRequest{
		Model:    "test-model",
		Messages: []llmpkg.Message{{Role: llmpkg.RoleUser, Content: "hi"}},
	}
}

// --- Chain tests ---

func TestNewChain(t *testing.T) {
	t.Run("empty chain", func(t *testing.T) {
		c := NewChain()
		assert.Equal(t, 0, c.Len())
		h := c.Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Choices[0].Message.Content)
	})

	t.Run("chain with middlewares", func(t *testing.T) {
		var order []string
		m1 := func(next Handler) Handler {
			return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
				order = append(order, "m1-before")
				resp, err := next(ctx, req)
				order = append(order, "m1-after")
				return resp, err
			}
		}
		m2 := func(next Handler) Handler {
			return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
				order = append(order, "m2-before")
				resp, err := next(ctx, req)
				order = append(order, "m2-after")
				return resp, err
			}
		}
		c := NewChain(m1, m2)
		assert.Equal(t, 2, c.Len())
		h := c.Then(successHandler())
		_, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, []string{"m1-before", "m2-before", "m2-after", "m1-after"}, order)
	})
}

func TestChain_Use(t *testing.T) {
	c := NewChain()
	assert.Equal(t, 0, c.Len())
	c.Use(func(next Handler) Handler { return next })
	assert.Equal(t, 1, c.Len())
	c.Use(func(next Handler) Handler { return next })
	assert.Equal(t, 2, c.Len())
}

func TestChain_UseFront(t *testing.T) {
	var order []string
	mA := func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			order = append(order, "A")
			return next(ctx, req)
		}
	}
	mB := func(next Handler) Handler {
		return func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			order = append(order, "B")
			return next(ctx, req)
		}
	}
	c := NewChain(mA)
	c.UseFront(mB)
	h := c.Then(successHandler())
	_, err := h(context.Background(), simpleReq())
	require.NoError(t, err)
	// B was added to front, so B runs first
	assert.Equal(t, []string{"B", "A"}, order)
}

// --- LoggingMiddleware ---

func TestLoggingMiddleware(t *testing.T) {
	var logs []string
	logger := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	t.Run("success", func(t *testing.T) {
		logs = nil
		h := NewChain(LoggingMiddleware(logger)).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, logs, 2)
		assert.Contains(t, logs[0], "Request")
		assert.Contains(t, logs[1], "Response")
	})

	t.Run("error", func(t *testing.T) {
		logs = nil
		h := NewChain(LoggingMiddleware(logger)).Then(dummyHandler(nil, errors.New("boom")))
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Len(t, logs, 2)
		assert.Contains(t, logs[1], "Error")
	})
}

// --- TimeoutMiddleware ---

func TestTimeoutMiddleware(t *testing.T) {
	t.Run("request completes before timeout", func(t *testing.T) {
		h := NewChain(TimeoutMiddleware(5 * time.Second)).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("context cancelled by timeout", func(t *testing.T) {
		slow := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &llmpkg.ChatResponse{}, nil
			}
		}
		h := NewChain(TimeoutMiddleware(10 * time.Millisecond)).Then(slow)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// --- RetryMiddleware ---

func TestRetryMiddleware(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return &llmpkg.ChatResponse{Model: "ok"}, nil
		}
		h := NewChain(RetryMiddleware(3, time.Millisecond)).Then(inner)
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Model)
		assert.Equal(t, 1, calls)
	})

	t.Run("retries retryable error then succeeds", func(t *testing.T) {
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			if calls < 3 {
				return nil, &types.Error{Code: "TRANSIENT", Retryable: true}
			}
			return &llmpkg.ChatResponse{Model: "ok"}, nil
		}
		h := NewChain(RetryMiddleware(3, time.Millisecond)).Then(inner)
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Model)
		assert.Equal(t, 3, calls)
	})

	t.Run("non-retryable error returns immediately", func(t *testing.T) {
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return nil, &types.Error{Code: "PERMANENT", Retryable: false}
		}
		h := NewChain(RetryMiddleware(3, time.Millisecond)).Then(inner)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("exhausts retries", func(t *testing.T) {
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return nil, &types.Error{Code: "TRANSIENT", Retryable: true}
		}
		h := NewChain(RetryMiddleware(2, time.Millisecond)).Then(inner)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Equal(t, 3, calls) // initial + 2 retries
	})
}

// --- MetricsMiddleware ---

type testMetricsCollector struct {
	mu       sync.Mutex
	requests []struct {
		model    string
		duration time.Duration
		success  bool
	}
	tokens []struct {
		model  string
		tokens int
	}
}

func (c *testMetricsCollector) RecordRequest(model string, duration time.Duration, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, struct {
		model    string
		duration time.Duration
		success  bool
	}{model, duration, success})
}

func (c *testMetricsCollector) RecordTokens(model string, tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = append(c.tokens, struct {
		model  string
		tokens int
	}{model, tokens})
}

func TestMetricsMiddleware(t *testing.T) {
	t.Run("records success", func(t *testing.T) {
		collector := &testMetricsCollector{}
		h := NewChain(MetricsMiddleware(collector)).Then(successHandler())
		_, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Len(t, collector.requests, 1)
		assert.True(t, collector.requests[0].success)
		assert.Len(t, collector.tokens, 1)
		assert.Equal(t, 42, collector.tokens[0].tokens)
	})

	t.Run("records failure", func(t *testing.T) {
		collector := &testMetricsCollector{}
		h := NewChain(MetricsMiddleware(collector)).Then(dummyHandler(nil, errors.New("fail")))
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Len(t, collector.requests, 1)
		assert.False(t, collector.requests[0].success)
		assert.Len(t, collector.tokens, 0) // no tokens on error
	})
}

// --- HeadersMiddleware ---

func TestHeadersMiddleware(t *testing.T) {
	t.Run("adds headers to nil metadata", func(t *testing.T) {
		req := &llmpkg.ChatRequest{Model: "m"}
		h := NewChain(HeadersMiddleware(map[string]string{"X-Key": "val"})).Then(
			func(ctx context.Context, r *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
				assert.Equal(t, "val", r.Metadata["X-Key"])
				return &llmpkg.ChatResponse{}, nil
			},
		)
		_, err := h(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("merges with existing metadata", func(t *testing.T) {
		req := &llmpkg.ChatRequest{
			Model:    "m",
			Metadata: map[string]string{"existing": "yes"},
		}
		h := NewChain(HeadersMiddleware(map[string]string{"new": "val"})).Then(
			func(ctx context.Context, r *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
				assert.Equal(t, "yes", r.Metadata["existing"])
				assert.Equal(t, "val", r.Metadata["new"])
				return &llmpkg.ChatResponse{}, nil
			},
		)
		_, err := h(context.Background(), req)
		require.NoError(t, err)
	})
}

// --- CacheMiddleware ---

type testCache struct {
	store map[string]*llmpkg.ChatResponse
}

func newTestCache() *testCache {
	return &testCache{store: make(map[string]*llmpkg.ChatResponse)}
}

func (c *testCache) Key(req *llmpkg.ChatRequest) string { return req.Model }
func (c *testCache) Get(key string) (*llmpkg.ChatResponse, bool) {
	r, ok := c.store[key]
	return r, ok
}
func (c *testCache) Set(key string, resp *llmpkg.ChatResponse) { c.store[key] = resp }

func TestCacheMiddleware(t *testing.T) {
	t.Run("cache miss then hit", func(t *testing.T) {
		cache := newTestCache()
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return &llmpkg.ChatResponse{Model: req.Model}, nil
		}
		h := NewChain(CacheMiddleware(cache)).Then(inner)

		// First call: cache miss
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, 1, calls)
		assert.Equal(t, "test-model", resp.Model)

		// Second call: cache hit
		resp2, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, 1, calls) // not called again
		assert.Equal(t, "test-model", resp2.Model)
	})

	t.Run("error not cached", func(t *testing.T) {
		cache := newTestCache()
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return nil, errors.New("fail")
		}
		h := NewChain(CacheMiddleware(cache)).Then(inner)
		_, _ = h(context.Background(), simpleReq())
		_, _ = h(context.Background(), simpleReq())
		assert.Equal(t, 2, calls) // called twice because error not cached
	})
}

// --- RateLimitMiddleware ---

type testLimiter struct {
	err error
}

func (l *testLimiter) Wait(ctx context.Context) error { return l.err }

func TestRateLimitMiddleware(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		h := NewChain(RateLimitMiddleware(&testLimiter{})).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("rate limited", func(t *testing.T) {
		h := NewChain(RateLimitMiddleware(&testLimiter{err: errors.New("rate limited")})).Then(successHandler())
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate limited")
	})
}

// --- RecoveryMiddleware ---

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		var recovered any
		onPanic := func(v any) { recovered = v }
		panicking := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			panic("test panic")
		}
		h := NewChain(RecoveryMiddleware(onPanic)).Then(panicking)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		var pe *PanicError
		assert.True(t, errors.As(err, &pe))
		assert.Equal(t, "middleware panic: test panic", pe.Error())
		assert.Equal(t, "test panic", recovered)
	})

	t.Run("no panic passes through", func(t *testing.T) {
		h := NewChain(RecoveryMiddleware(nil)).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("nil onPanic callback", func(t *testing.T) {
		panicking := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			panic("boom")
		}
		h := NewChain(RecoveryMiddleware(nil)).Then(panicking)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
	})

	t.Run("panic error preserves unwrap chain", func(t *testing.T) {
		root := errors.New("root panic")
		panicking := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			panic(root)
		}
		h := NewChain(RecoveryMiddleware(nil)).Then(panicking)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.ErrorIs(t, err, root)
	})
}

// --- TracingMiddleware ---

type testSpan struct {
	attrs  map[string]any
	events []string
	err    error
	ended  bool
}

func (s *testSpan) SetAttribute(key string, value any) {
	if s.attrs == nil {
		s.attrs = make(map[string]any)
	}
	s.attrs[key] = value
}
func (s *testSpan) AddEvent(name string, _ map[string]any) { s.events = append(s.events, name) }
func (s *testSpan) SetError(err error)                     { s.err = err }
func (s *testSpan) End()                                   { s.ended = true }

type testTracer struct {
	span *testSpan
}

func (t *testTracer) StartSpan(ctx context.Context, name string) (context.Context, llmpkg.Span) {
	t.span = &testSpan{}
	return ctx, t.span
}

func TestTracingMiddleware(t *testing.T) {
	t.Run("success sets attributes", func(t *testing.T) {
		tracer := &testTracer{}
		h := NewChain(TracingMiddleware(tracer)).Then(successHandler())
		_, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.True(t, tracer.span.ended)
		assert.Equal(t, "test-model", tracer.span.attrs["model"])
		assert.Equal(t, 42, tracer.span.attrs["tokens"])
		assert.Nil(t, tracer.span.err)
	})

	t.Run("error sets span error", func(t *testing.T) {
		tracer := &testTracer{}
		h := NewChain(TracingMiddleware(tracer)).Then(dummyHandler(nil, errors.New("fail")))
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.True(t, tracer.span.ended)
		assert.NotNil(t, tracer.span.err)
	})
}

// --- ValidatorMiddleware ---

type testValidator struct {
	err error
}

func (v *testValidator) Validate(req *llmpkg.ChatRequest) error { return v.err }

func TestValidatorMiddleware(t *testing.T) {
	t.Run("passes validation", func(t *testing.T) {
		h := NewChain(ValidatorMiddleware(&testValidator{})).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("fails validation", func(t *testing.T) {
		h := NewChain(ValidatorMiddleware(&testValidator{err: errors.New("invalid")})).Then(successHandler())
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("multiple validators first fails", func(t *testing.T) {
		calls := 0
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			calls++
			return &llmpkg.ChatResponse{}, nil
		}
		h := NewChain(ValidatorMiddleware(
			&testValidator{err: errors.New("v1 fail")},
			&testValidator{},
		)).Then(inner)
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.Equal(t, 0, calls) // handler never called
	})
}

// --- TransformMiddleware ---

func TestTransformMiddleware(t *testing.T) {
	t.Run("transforms request and response", func(t *testing.T) {
		reqTransform := func(req *llmpkg.ChatRequest) {
			req.Model = "transformed-model"
		}
		respTransform := func(resp *llmpkg.ChatResponse) {
			resp.Provider = "transformed-provider"
		}
		inner := func(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
			assert.Equal(t, "transformed-model", req.Model)
			return &llmpkg.ChatResponse{Model: req.Model}, nil
		}
		h := NewChain(TransformMiddleware(reqTransform, respTransform)).Then(inner)
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.Equal(t, "transformed-provider", resp.Provider)
	})

	t.Run("nil transforms are safe", func(t *testing.T) {
		h := NewChain(TransformMiddleware(nil, nil)).Then(successHandler())
		resp, err := h(context.Background(), simpleReq())
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("response transform skipped on error", func(t *testing.T) {
		called := false
		respTransform := func(resp *llmpkg.ChatResponse) { called = true }
		h := NewChain(TransformMiddleware(nil, respTransform)).Then(dummyHandler(nil, errors.New("fail")))
		_, err := h(context.Background(), simpleReq())
		require.Error(t, err)
		assert.False(t, called)
	})
}
