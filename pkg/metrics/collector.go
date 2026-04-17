package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	labelMethod    = "method"
	labelPath      = "path"
	labelStatus    = "status"
	labelProvider  = "provider"
	labelModel     = "model"
	labelTokenType = "type"
	labelAgentType = "agent_type"
	labelAgentID   = "agent_id"
	labelFromState = "from_state"
	labelToState   = "to_state"
	labelCacheType = "cache_type"
	labelToolName  = "tool_name"
	labelDatabase  = "database"
	labelOperation = "operation"
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
	// K3 FIX: agent_id 改为 agent_type，避免动态 ID 导致时间序列基数爆炸
	agentExecutionsTotal   *prometheus.CounterVec
	agentExecutionDuration *prometheus.HistogramVec
	agentStateTransitions  *prometheus.CounterVec
	agentInfo              *prometheus.GaugeVec // agent_id → agent_type 映射，仅用于调试

	// 缓存指标
	cacheHits      *prometheus.CounterVec
	cacheMisses    *prometheus.CounterVec
	cacheEvictions *prometheus.CounterVec
	cacheSize      *prometheus.GaugeVec

	// 工具调用指标
	toolCallsTotal   *prometheus.CounterVec
	toolCallDuration *prometheus.HistogramVec

	// 数据库指标
	dbConnectionsOpen *prometheus.GaugeVec
	dbConnectionsIdle *prometheus.GaugeVec
	dbQueryDuration   *prometheus.HistogramVec

	logger *zap.Logger
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
		[]string{labelMethod, labelPath, labelStatus},
	)

	c.httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{labelMethod, labelPath},
	)

	c.httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_size_bytes",
			Help:      "HTTP request size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{labelMethod, labelPath},
	)

	c.httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_response_size_bytes",
			Help:      "HTTP response size in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{labelMethod, labelPath},
	)

	// LLM 指标
	c.llmRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_requests_total",
			Help:      "Total number of LLM requests",
		},
		[]string{labelProvider, labelModel, labelStatus},
	)

	c.llmRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "llm_request_duration_seconds",
			Help:      "LLM request duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{labelProvider, labelModel},
	)

	c.llmTokensUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_tokens_used_total",
			Help:      "Total number of tokens used",
		},
		[]string{labelProvider, labelModel, labelTokenType}, // token_type: prompt, completion
	)

	c.llmCost = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "llm_cost_total",
			Help:      "Total LLM cost in USD",
		},
		[]string{labelProvider, labelModel},
	)

	// Agent 指标
	// K3 FIX: 使用 agent_type（有限枚举）替代 agent_id（动态 UUID），防止时间序列基数爆炸
	c.agentExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_executions_total",
			Help:      "Total number of agent executions",
		},
		[]string{labelAgentType, labelStatus},
	)

	c.agentExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "agent_execution_duration_seconds",
			Help:      "Agent execution duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
		},
		[]string{labelAgentType},
	)

	c.agentStateTransitions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "agent_state_transitions_total",
			Help:      "Total number of agent state transitions",
		},
		[]string{labelAgentType, labelFromState, labelToState},
	)

	// agent_info gauge 保留 agent_id 的可观测性，但不参与高频指标
	c.agentInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "agent_info",
			Help:      "Agent metadata mapping (agent_id to agent_type)",
		},
		[]string{labelAgentID, labelAgentType},
	)

	// 缓存指标
	c.cacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{labelCacheType},
	)

	c.cacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{labelCacheType},
	)

	c.cacheEvictions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_evictions_total",
			Help:      "Total number of cache evictions",
		},
		[]string{labelCacheType},
	)

	c.cacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cache_size",
			Help:      "Current number of items in cache",
		},
		[]string{labelCacheType},
	)

	// 工具调用指标
	c.toolCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tool_calls_total",
			Help:      "Total number of tool calls",
		},
		[]string{labelToolName, labelStatus},
	)

	c.toolCallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "tool_call_duration_seconds",
			Help:      "Tool call duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{labelToolName},
	)

	// 数据库指标
	c.dbConnectionsOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_open",
			Help:      "Number of open database connections",
		},
		[]string{labelDatabase},
	)

	c.dbConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections_idle",
			Help:      "Number of idle database connections",
		},
		[]string{labelDatabase},
	)

	c.dbQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{labelDatabase, labelOperation},
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
// K3 FIX: 移除 agentID 参数，仅使用 agentType（有限枚举值）
func (c *Collector) RecordAgentExecution(agentType, status string, duration time.Duration) {
	c.agentExecutionsTotal.WithLabelValues(agentType, status).Inc()
	c.agentExecutionDuration.WithLabelValues(agentType).Observe(duration.Seconds())
}

// RecordAgentStateTransition 记录 Agent 状态转换
// K3 FIX: 使用 agentType 替代 agentID，避免 agent_id x from_state x to_state 笛卡尔积爆炸
func (c *Collector) RecordAgentStateTransition(agentType, fromState, toState string) {
	c.agentStateTransitions.WithLabelValues(agentType, fromState, toState).Inc()
}

// RecordAgentInfo 记录 agent_id 到 agent_type 的映射关系（低频调用，仅用于调试）
func (c *Collector) RecordAgentInfo(agentID, agentType string) {
	c.agentInfo.WithLabelValues(agentID, agentType).Set(1)
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

// RecordCacheEviction 记录缓存驱逐
func (c *Collector) RecordCacheEviction(cacheType string) {
	c.cacheEvictions.WithLabelValues(cacheType).Inc()
}

// RecordCacheSize 记录缓存当前大小
func (c *Collector) RecordCacheSize(cacheType string, size int) {
	c.cacheSize.WithLabelValues(cacheType).Set(float64(size))
}

// =============================================================================
// 🔧 工具调用指标记录
// =============================================================================

// RecordToolCall 记录一次工具调用
func (c *Collector) RecordToolCall(toolName, status string, duration time.Duration) {
	c.toolCallsTotal.WithLabelValues(toolName, status).Inc()
	c.toolCallDuration.WithLabelValues(toolName).Observe(duration.Seconds())
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
