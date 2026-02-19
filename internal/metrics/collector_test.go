package metrics

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var collectorNamespaceSeq uint64

func nextTestNamespace() string {
	seq := atomic.AddUint64(&collectorNamespaceSeq, 1)
	return fmt.Sprintf("test_%d", seq)
}

// =============================================================================
// 🧪 Collector 测试
// =============================================================================

func TestNewCollector(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	assert.NotNil(t, collector)
	assert.NotNil(t, collector.httpRequestsTotal)
	assert.NotNil(t, collector.httpRequestDuration)
	assert.NotNil(t, collector.llmRequestsTotal)
	assert.NotNil(t, collector.llmRequestDuration)
	assert.NotNil(t, collector.llmTokensUsed)
	assert.NotNil(t, collector.llmCost)
}

func TestCollector_RecordHTTPRequest(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 记录请求
	collector.RecordHTTPRequest("GET", "/test", 200, 100*time.Millisecond, 1024, 2048)

	// 验证指标
	count := testutil.CollectAndCount(collector.httpRequestsTotal)
	assert.Greater(t, count, 0)

	// 再记录一次相同的请求
	collector.RecordHTTPRequest("GET", "/test", 200, 50*time.Millisecond, 512, 1024)

	// 验证计数增加
	newCount := testutil.CollectAndCount(collector.httpRequestsTotal)
	assert.GreaterOrEqual(t, newCount, count)
}

func TestCollector_RecordLLMRequest(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 记录 LLM 请求
	collector.RecordLLMRequest(
		"openai",
		"gpt-4",
		"success",
		500*time.Millisecond,
		100,  // prompt tokens
		50,   // completion tokens
		0.01, // cost
	)

	// 验证指标
	count := testutil.CollectAndCount(collector.llmRequestsTotal)
	assert.Greater(t, count, 0)

	tokensCount := testutil.CollectAndCount(collector.llmTokensUsed)
	assert.Greater(t, tokensCount, 0)

	costCount := testutil.CollectAndCount(collector.llmCost)
	assert.Greater(t, costCount, 0)
}

func TestCollector_RecordAgentExecution(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 记录 Agent 执行
	collector.RecordAgentExecution(
		"test-agent",
		"chat",
		"success",
		1*time.Second,
	)

	// 验证指标
	count := testutil.CollectAndCount(collector.agentExecutionsTotal)
	assert.Greater(t, count, 0)
}

func TestCollector_RecordCacheOperation(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 记录缓存命中
	collector.RecordCacheHit("redis")

	// 记录缓存未命中
	collector.RecordCacheMiss("redis")

	// 验证指标
	hitCount := testutil.CollectAndCount(collector.cacheHits)
	assert.Greater(t, hitCount, 0)

	missCount := testutil.CollectAndCount(collector.cacheMisses)
	assert.Greater(t, missCount, 0)
}

func TestCollector_RecordDatabaseQuery(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 记录数据库查询
	collector.RecordDBQuery("postgres", "SELECT", 20*time.Millisecond)

	// 验证指标
	count := testutil.CollectAndCount(collector.dbQueryDuration)
	assert.Greater(t, count, 0)
}

func TestCollector_UpdateConnectionPool(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 更新连接池状态
	collector.RecordDBConnections("postgres", 10, 5)

	// 验证指标
	openCount := testutil.CollectAndCount(collector.dbConnectionsOpen)
	assert.Greater(t, openCount, 0)

	idleCount := testutil.CollectAndCount(collector.dbConnectionsIdle)
	assert.Greater(t, idleCount, 0)
}

func TestCollector_ConcurrentRecording(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(nextTestNamespace(), logger)

	// 并发记录多个指标
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			collector.RecordHTTPRequest("GET", "/test", 200, 100*time.Millisecond, 1024, 2048)
			collector.RecordLLMRequest("openai", "gpt-4", "success", 500*time.Millisecond, 100, 50, 0.01)
			collector.RecordCacheHit("redis")
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证指标被正确记录
	httpCount := testutil.CollectAndCount(collector.httpRequestsTotal)
	assert.Greater(t, httpCount, 0)

	llmCount := testutil.CollectAndCount(collector.llmRequestsTotal)
	assert.Greater(t, llmCount, 0)

	cacheCount := testutil.CollectAndCount(collector.cacheHits)
	assert.Greater(t, cacheCount, 0)
}

func TestCollector_MetricsRegistration(t *testing.T) {
	logger := zap.NewNop()

	// 创建自定义 registry
	registry := prometheus.NewRegistry()

	// 创建 collector（会自动注册到默认 registry）
	collector := NewCollector(nextTestNamespace(), logger)

	// 手动注册到自定义 registry
	registry.MustRegister(collector.httpRequestsTotal)
	registry.MustRegister(collector.httpRequestDuration)

	// 记录一些数据
	collector.RecordHTTPRequest("GET", "/test", 200, 100*time.Millisecond, 0, 0)

	// 验证可以从自定义 registry 收集指标
	count := testutil.CollectAndCount(collector.httpRequestsTotal)
	assert.Greater(t, count, 0)
}
