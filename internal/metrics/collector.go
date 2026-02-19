// Package metrics provides internal metrics collection.
// This package is internal and should not be imported by external projects.
package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// =============================================================================
// 📊 指标收集器
// =============================================================================

// Collector 指标收集器
type Collector struct {
	// HTTP 指标
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	httpRequestSize     *prometheus.HistogramVec
	httpResponseSize    *prometheus.HistogramVec

	// LLM 指标
	llmRequestsTotal   *prometheus.CounterVec
	llmRequestDuration *prometheus.HistogramVec
	llmTokensUsed      *prometheus.CounterVec
	llmCost            *prometheus.CounterVec

	// Agent 指标
	agentExecutionsTotal   *prometheus.CounterVec
	agentExecutionDuration *prometheus.HistogramVec
	agentStateTransitions  *prometheus.CounterVec

	// 缓存指标
	cacheHits   *prometheus.CounterVec
	cacheMisses *prometheus.CounterVec

	// 数据库指标
	dbConnectionsOpen *prometheus.GaugeVec
	dbConnectionsIdle *prometheus.GaugeVec
	dbQueryDuration   *prometheus.HistogramVec

	logger *zap.Logger
	mu     sync.RWMutex
}

// NewCollector 创建指标收集器
func NewCollector(namespace string, logger *zap.Logger) *Collector {
	c := &Collector{
		logger: logger.With(zap.String("component", "metrics")),
	}

	// HTTP 指标
	c.httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	c.httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	c.httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_size_bytes",
			Help:      "HTTP request size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	c.httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_response_size_bytes",
			Help:      "HTTP response size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	// LLM 指标
	c.llmRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_requests_total",
			Help:      "Total number of LLM requests",
		},
		[]string{"provider", "model", "status"},
	)

	c.llmRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "llm_request_duration_seconds",
			Help:      "LLM request duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"provider", "model"},
	)

	c.llmTokensUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_tokens_used_total",
			Help:      "Total number of tokens used",
		},
		[]string{"provider", "model", "type"}, // type: prompt, completion
	)

	c.llmCost = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_cost_total",
			Help:      "Total LLM cost in USD",
		},
		[]string{"provider", "model"},
	)

	// Agent 指标
	c.agentExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_executions_total",
			Help:      "Total number of agent executions",
		},
		[]string{"agent_id", "agent_type", "status"},
	)

	c.agentExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "agent_execution_duration_seconds",
			Help:      "Agent execution duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
		},
		[]string{"agent_id", "agent_type"},
	)

	c.agentStateTransitions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_state_transitions_total",
			Help:      "Total number of agent state transitions",
		},
		[]string{"agent_id", "from_state", "to_state"},
	)

	// 缓存指标
	c.cacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	c.cacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	// 数据库指标
	c.dbConnectionsOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_open",
			Help:      "Number of open database connections",
		},
		[]string{"database"},
	)

	c.dbConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_idle",
			Help:      "Number of idle database connections",
		},
		[]string{"database"},
	)

	c.dbQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"database", "operation"},
	)

	logger.Info("metrics collector initialized", zap.String("namespace", namespace))

	return c
}

// =============================================================================
// 🎯 HTTP 指标记录
// =============================================================================

// RecordHTTPRequest 记录 HTTP 请求
func (c *Collector) RecordHTTPRequest(method, path string, status int, duration time.Duration, requestSize, responseSize int64) {
	c.httpRequestsTotal.WithLabelValues(method, path, statusCode(status)).Inc()
	c.httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	c.httpRequestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	c.httpResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
}

// =============================================================================
// 🤖 LLM 指标记录
// =============================================================================

// RecordLLMRequest 记录 LLM 请求
func (c *Collector) RecordLLMRequest(provider, model, status string, duration time.Duration, promptTokens, completionTokens int, cost float64) {
	c.llmRequestsTotal.WithLabelValues(provider, model, status).Inc()
	c.llmRequestDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
	c.llmTokensUsed.WithLabelValues(provider, model, "prompt").Add(float64(promptTokens))
	c.llmTokensUsed.WithLabelValues(provider, model, "completion").Add(float64(completionTokens))
	c.llmCost.WithLabelValues(provider, model).Add(cost)
}

// =============================================================================
// 🎭 Agent 指标记录
// =============================================================================

// RecordAgentExecution 记录 Agent 执行
func (c *Collector) RecordAgentExecution(agentID, agentType, status string, duration time.Duration) {
	c.agentExecutionsTotal.WithLabelValues(agentID, agentType, status).Inc()
	c.agentExecutionDuration.WithLabelValues(agentID, agentType).Observe(duration.Seconds())
}

// RecordAgentStateTransition 记录 Agent 状态转换
func (c *Collector) RecordAgentStateTransition(agentID, fromState, toState string) {
	c.agentStateTransitions.WithLabelValues(agentID, fromState, toState).Inc()
}

// =============================================================================
// 💾 缓存指标记录
// =============================================================================

// RecordCacheHit 记录缓存命中
func (c *Collector) RecordCacheHit(cacheType string) {
	c.cacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss 记录缓存未命中
func (c *Collector) RecordCacheMiss(cacheType string) {
	c.cacheMisses.WithLabelValues(cacheType).Inc()
}

// =============================================================================
// 🗄️ 数据库指标记录
// =============================================================================

// RecordDBConnections 记录数据库连接数
func (c *Collector) RecordDBConnections(database string, open, idle int) {
	c.dbConnectionsOpen.WithLabelValues(database).Set(float64(open))
	c.dbConnectionsIdle.WithLabelValues(database).Set(float64(idle))
}

// RecordDBQuery 记录数据库查询
func (c *Collector) RecordDBQuery(database, operation string, duration time.Duration) {
	c.dbQueryDuration.WithLabelValues(database, operation).Observe(duration.Seconds())
}

// =============================================================================
// 🔧 辅助函数
// =============================================================================

// statusCode 将 HTTP 状态码转换为字符串
func statusCode(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}
