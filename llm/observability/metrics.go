package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/BaSui01/agentflow/llm"

// Metrics LLM 指标收集器
type Metrics struct {
	tracer trace.Tracer
	meter  metric.Meter
	// 柜台
	requestTotal   metric.Int64Counter
	tokenTotal     metric.Int64Counter
	errorTotal     metric.Int64Counter
	fallbackTotal  metric.Int64Counter
	cacheHitTotal  metric.Int64Counter
	cacheMissTotal metric.Int64Counter
	// 直方图
	requestDuration metric.Float64Histogram
	tokenCount      metric.Int64Histogram
	costPerRequest  metric.Float64Histogram
	// 高地语
	activeRequests metric.Int64UpDownCounter
	circuitState   metric.Int64ObservableGauge
}

// NewMetrics 创建指标收集器
func NewMetrics() (*Metrics, error) {
	tracer := otel.Tracer(instrumentationName)
	meter := otel.Meter(instrumentationName)

	m := &Metrics{
		tracer: tracer,
		meter:  meter,
	}

	var err error

	// 请求计数
	m.requestTotal, err = meter.Int64Counter("llm.request.total",
		metric.WithDescription("Total number of LLM requests"),
		metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}

	// Token 计数
	m.tokenTotal, err = meter.Int64Counter("llm.token.total",
		metric.WithDescription("Total tokens consumed"),
		metric.WithUnit("{token}"))
	if err != nil {
		return nil, err
	}

	// 错误计数
	m.errorTotal, err = meter.Int64Counter("llm.error.total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("{error}"))
	if err != nil {
		return nil, err
	}

	// 降级计数
	m.fallbackTotal, err = meter.Int64Counter("llm.fallback.total",
		metric.WithDescription("Total number of fallbacks triggered"),
		metric.WithUnit("{fallback}"))
	if err != nil {
		return nil, err
	}

	// 缓存命中
	m.cacheHitTotal, err = meter.Int64Counter("llm.cache.hit.total",
		metric.WithDescription("Total cache hits"),
		metric.WithUnit("{hit}"))
	if err != nil {
		return nil, err
	}

	// 缓存未命中
	m.cacheMissTotal, err = meter.Int64Counter("llm.cache.miss.total",
		metric.WithDescription("Total cache misses"),
		metric.WithUnit("{miss}"))
	if err != nil {
		return nil, err
	}

	// 请求延迟
	m.requestDuration, err = meter.Float64Histogram("llm.request.duration",
		metric.WithDescription("Request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30))
	if err != nil {
		return nil, err
	}

	// Token 分布
	m.tokenCount, err = meter.Int64Histogram("llm.token.count",
		metric.WithDescription("Token count per request"),
		metric.WithUnit("{token}"),
		metric.WithExplicitBucketBoundaries(100, 500, 1000, 2000, 4000, 8000, 16000, 32000))
	if err != nil {
		return nil, err
	}

	// 成本分布
	m.costPerRequest, err = meter.Float64Histogram("llm.cost.per_request",
		metric.WithDescription("Cost per request in USD"),
		metric.WithUnit("USD"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5))
	if err != nil {
		return nil, err
	}

	// 活跃请求数
	m.activeRequests, err = meter.Int64UpDownCounter("llm.request.active",
		metric.WithDescription("Number of active requests"),
		metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}

	return m, nil
}

// RequestAttrs 请求属性
type RequestAttrs struct {
	Provider string
	Model    string
	TenantID string
	UserID   string
	Feature  string
	TraceID  string
}

// ResponseAttrs 响应属性
type ResponseAttrs struct {
	Status           string
	ErrorCode        string
	TokensPrompt     int
	TokensCompletion int
	Cost             float64
	Duration         time.Duration
	Cached           bool
	Fallback         bool
	FallbackLevel    int
}

// StartRequest 开始请求追踪
func (m *Metrics) StartRequest(ctx context.Context, attrs RequestAttrs) (context.Context, trace.Span) {
	ctx, span := m.tracer.Start(ctx, "llm.completion",
		trace.WithAttributes(
			attribute.String("llm.provider", attrs.Provider),
			attribute.String("llm.model", attrs.Model),
			attribute.String("tenant.id", attrs.TenantID),
			attribute.String("user.id", attrs.UserID),
			attribute.String("llm.feature", attrs.Feature),
		))

	m.activeRequests.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("provider", attrs.Provider),
			attribute.String("model", attrs.Model)))

	return ctx, span
}

// EndRequest 结束请求追踪
func (m *Metrics) EndRequest(ctx context.Context, span trace.Span, req RequestAttrs, resp ResponseAttrs) {
	defer span.End()

	commonAttrs := []attribute.KeyValue{
		attribute.String("provider", req.Provider),
		attribute.String("model", req.Model),
		attribute.String("tenant_id", req.TenantID),
		attribute.String("feature", req.Feature),
		attribute.String("status", resp.Status),
	}

	// 减少活跃请求
	m.activeRequests.Add(ctx, -1,
		metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model)))

	// 记录请求
	m.requestTotal.Add(ctx, 1, metric.WithAttributes(commonAttrs...))

	// 记录延迟
	m.requestDuration.Record(ctx, resp.Duration.Seconds(), metric.WithAttributes(commonAttrs...))

	// 记录 Token
	totalTokens := int64(resp.TokensPrompt + resp.TokensCompletion)
	if totalTokens > 0 {
		m.tokenTotal.Add(ctx, totalTokens, metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model),
			attribute.String("type", "total")))

		m.tokenTotal.Add(ctx, int64(resp.TokensPrompt), metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model),
			attribute.String("type", "prompt")))

		m.tokenTotal.Add(ctx, int64(resp.TokensCompletion), metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model),
			attribute.String("type", "completion")))

		m.tokenCount.Record(ctx, totalTokens, metric.WithAttributes(commonAttrs...))
	}

	// 记录成本
	if resp.Cost > 0 {
		m.costPerRequest.Record(ctx, resp.Cost, metric.WithAttributes(commonAttrs...))
	}

	// 记录错误
	if resp.ErrorCode != "" {
		m.errorTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model),
			attribute.String("error_code", resp.ErrorCode)))

		span.SetAttributes(attribute.String("error.code", resp.ErrorCode))
	}

	// 记录降级
	if resp.Fallback {
		m.fallbackTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("provider", req.Provider),
			attribute.String("model", req.Model),
			attribute.Int("level", resp.FallbackLevel)))

		span.SetAttributes(
			attribute.Bool("llm.fallback", true),
			attribute.Int("llm.fallback_level", resp.FallbackLevel))
	}

	// 记录缓存
	if resp.Cached {
		m.cacheHitTotal.Add(ctx, 1, metric.WithAttributes(commonAttrs...))
		span.SetAttributes(attribute.Bool("llm.cache_hit", true))
	}

	// Span 属性
	span.SetAttributes(
		attribute.String("llm.status", resp.Status),
		attribute.Int("llm.tokens.prompt", resp.TokensPrompt),
		attribute.Int("llm.tokens.completion", resp.TokensCompletion),
		attribute.Float64("llm.cost", resp.Cost),
		attribute.Float64("llm.duration_ms", float64(resp.Duration.Milliseconds())))
}

// RecordCacheMiss 记录缓存未命中
func (m *Metrics) RecordCacheMiss(ctx context.Context, provider, model string) {
	m.cacheMissTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("model", model)))
}

// RecordToolCall 记录工具调用
func (m *Metrics) RecordToolCall(ctx context.Context, toolName string, duration time.Duration, success bool) {
	_, span := m.tracer.Start(ctx, "llm.tool_call",
		trace.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.Bool("tool.success", success),
			attribute.Float64("tool.duration_ms", float64(duration.Milliseconds()))))
	defer span.End()
}

// Tracer 获取 Tracer
func (m *Metrics) Tracer() trace.Tracer {
	return m.tracer
}
