# P3 优化提示词

## P3-1: Provider A/B 测试路由

### 需求背景
llm/ 目录有 13+ provider（OpenAI、Anthropic、DeepSeek、Gemini、Qwen 等），现有路由机制（`llm/router/router.go` 的 `WeightedRouter` 和 `llm/router_multi_provider.go` 的 `MultiProviderRouter`）支持基于成本/健康/QPS 的策略选择，但缺少 A/B 测试路由能力。需要支持按比例将请求分发到不同 provider，用于质量对比和成本优化。

### 需要修改的文件

#### 新建文件：llm/router/ab_router.go

```go
package router

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ABVariant 表示 A/B 测试中的一个变体
type ABVariant struct {
	// Name 变体名称（如 "control", "experiment_a"）
	Name string
	// Provider 该变体使用的 Provider
	Provider llmpkg.Provider
	// Weight 流量权重（0-100），所有变体权重之和应为 100
	Weight int
	// Metadata 变体元数据
	Metadata map[string]string
}

// ABMetrics 单个变体的指标收集
type ABMetrics struct {
	VariantName    string
	TotalRequests  int64
	SuccessCount   int64
	FailureCount   int64
	TotalLatencyMs int64   // 累计延迟，用于计算平均值
	TotalCost      float64 // 累计成本
	QualityScores  []float64
	mu             sync.Mutex
}

// RecordRequest 记录一次请求
func (m *ABMetrics) RecordRequest(latencyMs int64, cost float64, success bool, qualityScore float64) {
	atomic.AddInt64(&m.TotalRequests, 1)
	atomic.AddInt64(&m.TotalLatencyMs, latencyMs)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalCost += cost
	if success {
		m.SuccessCount++
	} else {
		m.FailureCount++
	}
	if qualityScore > 0 {
		m.QualityScores = append(m.QualityScores, qualityScore)
	}
}

// GetAvgLatencyMs 获取平均延迟
func (m *ABMetrics) GetAvgLatencyMs() float64 {
	total := atomic.LoadInt64(&m.TotalRequests)
	if total == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.TotalLatencyMs)) / float64(total)
}

// GetSuccessRate 获取成功率
func (m *ABMetrics) GetSuccessRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.SuccessCount + m.FailureCount
	if total == 0 {
		return 0
	}
	return float64(m.SuccessCount) / float64(total)
}

// GetAvgQualityScore 获取平均质量评分
func (m *ABMetrics) GetAvgQualityScore() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.QualityScores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range m.QualityScores {
		sum += s
	}
	return sum / float64(len(m.QualityScores))
}

// ABTestConfig A/B 测试配置
type ABTestConfig struct {
	// Name 测试名称
	Name string
	// Variants 变体列表
	Variants []ABVariant
	// StickyRouting 是否启用粘性路由（同一用户/会话始终路由到同一变体）
	StickyRouting bool
	// StickyKey 粘性路由的 key 类型："user_id" 或 "session_id"
	StickyKey string
	// StartTime 测试开始时间
	StartTime time.Time
	// EndTime 测试结束时间（零值表示无限期）
	EndTime time.Time
}

// ABRouter A/B 测试路由器，实现 llmpkg.Provider 接口
type ABRouter struct {
	config  ABTestConfig
	metrics map[string]*ABMetrics // variantName -> metrics

	// 粘性路由缓存
	stickyCache map[string]string // stickyKey -> variantName
	stickyCacheMu sync.RWMutex

	// 动态权重调整
	dynamicWeights map[string]int // variantName -> weight
	weightsMu      sync.RWMutex

	logger *zap.Logger
	rng    *rand.Rand
	rngMu  sync.Mutex
}

// NewABRouter 创建 A/B 测试路由器
func NewABRouter(config ABTestConfig, logger *zap.Logger) (*ABRouter, error) {
	if len(config.Variants) < 2 {
		return nil, fmt.Errorf("A/B test requires at least 2 variants")
	}

	// 验证权重之和为 100
	totalWeight := 0
	for _, v := range config.Variants {
		totalWeight += v.Weight
	}
	if totalWeight != 100 {
		return nil, fmt.Errorf("variant weights must sum to 100, got %d", totalWeight)
	}

	metrics := make(map[string]*ABMetrics)
	dynamicWeights := make(map[string]int)
	for _, v := range config.Variants {
		metrics[v.Name] = &ABMetrics{VariantName: v.Name}
		dynamicWeights[v.Name] = v.Weight
	}

	return &ABRouter{
		config:         config,
		metrics:        metrics,
		stickyCache:    make(map[string]string),
		dynamicWeights: dynamicWeights,
		logger:         logger.With(zap.String("component", "ab_router")),
		rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// selectVariant 选择变体（核心路由逻辑）
func (r *ABRouter) selectVariant(ctx context.Context, req *llmpkg.ChatRequest) (*ABVariant, error) {
	// 1. 检查测试是否在有效期内
	now := time.Now()
	if !r.config.EndTime.IsZero() && now.After(r.config.EndTime) {
		// 测试已结束，返回第一个变体（control）
		return &r.config.Variants[0], nil
	}

	// 2. 粘性路由检查
	if r.config.StickyRouting {
		stickyKey := r.extractStickyKey(req)
		if stickyKey != "" {
			r.stickyCacheMu.RLock()
			variantName, exists := r.stickyCache[stickyKey]
			r.stickyCacheMu.RUnlock()

			if exists {
				for i := range r.config.Variants {
					if r.config.Variants[i].Name == variantName {
						return &r.config.Variants[i], nil
					}
				}
			}

			// 首次路由，使用确定性哈希分配
			variant := r.hashBasedSelect(stickyKey)
			r.stickyCacheMu.Lock()
			r.stickyCache[stickyKey] = variant.Name
			r.stickyCacheMu.Unlock()
			return variant, nil
		}
	}

	// 3. 加权随机选择
	return r.weightedRandomSelect(), nil
}

// extractStickyKey 从请求中提取粘性路由 key
func (r *ABRouter) extractStickyKey(req *llmpkg.ChatRequest) string {
	switch r.config.StickyKey {
	case "user_id":
		return req.UserID
	case "session_id":
		return req.TraceID // 用 TraceID 作为 session 标识
	case "tenant_id":
		return req.TenantID
	default:
		return req.UserID
	}
}

// hashBasedSelect 基于哈希的确定性选择（保证同一 key 始终选择同一变体）
func (r *ABRouter) hashBasedSelect(key string) *ABVariant {
	h := sha256.Sum256([]byte(key))
	hashVal := binary.BigEndian.Uint64(h[:8])
	bucket := int(hashVal % 100)

	r.weightsMu.RLock()
	defer r.weightsMu.RUnlock()

	cumulative := 0
	for i := range r.config.Variants {
		w := r.dynamicWeights[r.config.Variants[i].Name]
		cumulative += w
		if bucket < cumulative {
			return &r.config.Variants[i]
		}
	}
	return &r.config.Variants[0]
}

// weightedRandomSelect 加权随机选择
func (r *ABRouter) weightedRandomSelect() *ABVariant {
	r.weightsMu.RLock()
	defer r.weightsMu.RUnlock()

	r.rngMu.Lock()
	target := r.rng.Intn(100)
	r.rngMu.Unlock()

	cumulative := 0
	for i := range r.config.Variants {
		w := r.dynamicWeights[r.config.Variants[i].Name]
		cumulative += w
		if target < cumulative {
			return &r.config.Variants[i]
		}
	}
	return &r.config.Variants[0]
}

// Completion 实现 llmpkg.Provider 接口 — 同步请求
func (r *ABRouter) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("routing request to variant",
		zap.String("variant", variant.Name),
		zap.String("test", r.config.Name),
	)

	start := time.Now()
	resp, err := variant.Provider.Completion(ctx, req)
	latencyMs := time.Since(start).Milliseconds()

	// 记录指标
	metrics := r.metrics[variant.Name]
	cost := 0.0
	if resp != nil {
		cost = float64(resp.Usage.TotalTokens) * 0.00001 // 简化成本估算
	}
	metrics.RecordRequest(latencyMs, cost, err == nil, 0)

	if resp != nil {
		resp.Provider = fmt.Sprintf("%s[%s]", resp.Provider, variant.Name)
	}

	return resp, err
}

// Stream 实现 llmpkg.Provider 接口 — 流式请求
func (r *ABRouter) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("streaming request to variant",
		zap.String("variant", variant.Name),
	)

	return variant.Provider.Stream(ctx, req)
}

// HealthCheck 实现 llmpkg.Provider 接口
func (r *ABRouter) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	// 检查所有变体的健康状态
	for _, v := range r.config.Variants {
		status, err := v.Provider.HealthCheck(ctx)
		if err != nil || !status.Healthy {
			return &llmpkg.HealthStatus{Healthy: false}, err
		}
	}
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

// Name 实现 llmpkg.Provider 接口
func (r *ABRouter) Name() string {
	return fmt.Sprintf("ab_router[%s]", r.config.Name)
}

// SupportsNativeFunctionCalling 实现 llmpkg.Provider 接口
// 只有所有变体都支持时才返回 true
func (r *ABRouter) SupportsNativeFunctionCalling() bool {
	for _, v := range r.config.Variants {
		if !v.Provider.SupportsNativeFunctionCalling() {
			return false
		}
	}
	return true
}

// UpdateWeights 动态调整权重
func (r *ABRouter) UpdateWeights(weights map[string]int) error {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total != 100 {
		return fmt.Errorf("weights must sum to 100, got %d", total)
	}

	r.weightsMu.Lock()
	defer r.weightsMu.Unlock()

	for name, w := range weights {
		r.dynamicWeights[name] = w
	}

	// 粘性路由需要清除缓存（权重变化后重新分配）
	if r.config.StickyRouting {
		r.stickyCacheMu.Lock()
		r.stickyCache = make(map[string]string)
		r.stickyCacheMu.Unlock()
	}

	r.logger.Info("A/B test weights updated",
		zap.String("test", r.config.Name),
		zap.Any("weights", weights),
	)

	return nil
}

// GetMetrics 获取所有变体的指标
func (r *ABRouter) GetMetrics() map[string]*ABMetrics {
	return r.metrics
}

// GetReport 获取 A/B 测试报告
func (r *ABRouter) GetReport() map[string]map[string]interface{} {
	report := make(map[string]map[string]interface{})
	for name, m := range r.metrics {
		report[name] = map[string]interface{}{
			"total_requests":    atomic.LoadInt64(&m.TotalRequests),
			"success_rate":      m.GetSuccessRate(),
			"avg_latency_ms":    m.GetAvgLatencyMs(),
			"avg_quality_score": m.GetAvgQualityScore(),
			"total_cost":        m.TotalCost,
		}
	}
	return report
}
```

#### 新建文件：llm/router/ab_router_test.go

```go
package router

import (
	"context"
	"testing"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type MockABProvider struct {
	mock.Mock
}

func (m *MockABProvider) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*llmpkg.ChatResponse), args.Error(1)
}

func (m *MockABProvider) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(<-chan llmpkg.StreamChunk), args.Error(1)
}

func (m *MockABProvider) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	args := m.Called(ctx)
	return args.Get(0).(*llmpkg.HealthStatus), args.Error(1)
}

func (m *MockABProvider) Name() string                        { return m.Called().String(0) }
func (m *MockABProvider) SupportsNativeFunctionCalling() bool { return m.Called().Bool(0) }

func TestABRouter_WeightedDistribution(t *testing.T) {
	providerA := new(MockABProvider)
	providerB := new(MockABProvider)

	resp := &llmpkg.ChatResponse{
		Model:   "test",
		Choices: []llmpkg.ChatChoice{{Message: llmpkg.Message{Content: "ok"}}},
		Usage:   llmpkg.ChatUsage{TotalTokens: 100},
	}
	providerA.On("Completion", mock.Anything, mock.Anything).Return(resp, nil)
	providerB.On("Completion", mock.Anything, mock.Anything).Return(resp, nil)

	router, err := NewABRouter(ABTestConfig{
		Name: "test-ab",
		Variants: []ABVariant{
			{Name: "control", Provider: providerA, Weight: 70},
			{Name: "experiment", Provider: providerB, Weight: 30},
		},
	}, zap.NewNop())
	assert.NoError(t, err)

	// 发送 1000 次请求，验证分布接近 70/30
	for i := 0; i < 1000; i++ {
		_, _ = router.Completion(context.Background(), &llmpkg.ChatRequest{Model: "test"})
	}

	report := router.GetReport()
	controlReqs := report["control"]["total_requests"].(int64)
	experimentReqs := report["experiment"]["total_requests"].(int64)

	// 允许 10% 误差
	assert.InDelta(t, 700, controlReqs, 100)
	assert.InDelta(t, 300, experimentReqs, 100)
}

func TestABRouter_StickyRouting(t *testing.T) {
	providerA := new(MockABProvider)
	providerB := new(MockABProvider)

	resp := &llmpkg.ChatResponse{
		Model: "test",
		Usage: llmpkg.ChatUsage{TotalTokens: 50},
	}
	providerA.On("Completion", mock.Anything, mock.Anything).Return(resp, nil)
	providerA.On("Name").Return("provider_a")
	providerB.On("Completion", mock.Anything, mock.Anything).Return(resp, nil)
	providerB.On("Name").Return("provider_b")

	router, err := NewABRouter(ABTestConfig{
		Name:          "sticky-test",
		StickyRouting: true,
		StickyKey:     "user_id",
		Variants: []ABVariant{
			{Name: "control", Provider: providerA, Weight: 50},
			{Name: "experiment", Provider: providerB, Weight: 50},
		},
	}, zap.NewNop())
	assert.NoError(t, err)

	// 同一用户应始终路由到同一变体
	req := &llmpkg.ChatRequest{Model: "test", UserID: "user-123"}
	var firstProvider string
	for i := 0; i < 10; i++ {
		resp, _ := router.Completion(context.Background(), req)
		if i == 0 {
			firstProvider = resp.Provider
		}
		assert.Equal(t, firstProvider, resp.Provider)
	}
}

func TestABRouter_DynamicWeightUpdate(t *testing.T) {
	providerA := new(MockABProvider)
	providerB := new(MockABProvider)

	router, err := NewABRouter(ABTestConfig{
		Name: "dynamic-test",
		Variants: []ABVariant{
			{Name: "control", Provider: providerA, Weight: 50},
			{Name: "experiment", Provider: providerB, Weight: 50},
		},
	}, zap.NewNop())
	assert.NoError(t, err)

	// 更新权重
	err = router.UpdateWeights(map[string]int{"control": 90, "experiment": 10})
	assert.NoError(t, err)

	// 权重之和不为 100 应报错
	err = router.UpdateWeights(map[string]int{"control": 80, "experiment": 30})
	assert.Error(t, err)
}
```

### 修改步骤

1. 创建 `llm/router/ab_router.go`，实现上述 `ABRouter` 结构体和所有方法
2. 创建 `llm/router/ab_router_test.go`，实现上述测试
3. 确保 `ABRouter` 实现 `llmpkg.Provider` 接口（编译时检查）：
   ```go
   var _ llmpkg.Provider = (*ABRouter)(nil)
   ```
4. 在 `llm/router/router.go` 的 `ModelRouter` 接口旁添加注释说明 `ABRouter` 的用途

### 验证方法

```bash
# 编译检查
cd D:/code/agentflow && go build ./llm/router/...

# 运行测试
go test ./llm/router/ -run TestABRouter -v

# 检查接口实现
go vet ./llm/router/...
```

### 注意事项
- `ABRouter` 实现 `llmpkg.Provider` 接口（`provider.go` 第 55-70 行），可以作为代理 Provider 透明替换
- 粘性路由使用 SHA256 哈希保证确定性，不依赖随机数
- `dynamicWeights` 和 `stickyCache` 分别用独立的 `sync.RWMutex` 保护，避免锁竞争
- 权重更新时清除粘性缓存，确保新权重生效
- 指标收集使用 `atomic` 操作减少锁开销
- 与现有 `WeightedRouter`（`llm/router/router.go`）和 `MultiProviderRouter`（`llm/router_multi_provider.go`）互不干扰，ABRouter 是独立的 Provider 包装器

---

## P3-2: MCP 完整实现

### 需求背景
`agent/protocol/mcp/` 目录有 MCP 协议的基础框架（3 个文件）：
- `protocol.go`（第 1-307 行）：定义了 `MCPServer`/`MCPClient` 接口、`Resource`/`ToolDefinition`/`PromptTemplate` 类型、JSON-RPC 消息格式
- `server.go`（第 1-345 行）：`DefaultMCPServer` 实现了基本的资源/工具/提示词管理，但缺少 HTTP/WebSocket 传输层、消息分发、初始化握手
- `client.go`（第 1-510 行）：`DefaultMCPClient` 实现了基于 `io.Reader/Writer` 的 stdio 通信，但缺少 SSE/WebSocket 传输、重连机制、超时处理

当前缺失的功能：
1. 服务器端缺少 HTTP 传输层（无法通过 HTTP/SSE/WebSocket 暴露 MCP 服务）
2. 客户端只支持 stdio，缺少 SSE 和 WebSocket 传输
3. 缺少 MCP 初始化握手（`initialize` 方法）
4. 缺少 Sampling 能力（`ServerCapabilities.Sampling` 为 false）
5. 工具执行缺少超时和并发控制
6. 资源管理缺少模板 URI 支持

### 需要修改的文件

#### 新建文件：agent/protocol/mcp/transport.go

```go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// Transport MCP 传输层接口
type Transport interface {
	// Send 发送消息
	Send(ctx context.Context, msg *MCPMessage) error
	// Receive 接收消息（阻塞）
	Receive(ctx context.Context) (*MCPMessage, error)
	// Close 关闭传输
	Close() error
}

// StdioTransport 标准输入输出传输（已有逻辑重构）
type StdioTransport struct {
	reader  *bufio.Reader
	writer  io.Writer
	writeMu sync.Mutex
	logger  *zap.Logger
}

func NewStdioTransport(reader io.Reader, writer io.Writer, logger *zap.Logger) *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(reader),
		writer: writer,
		logger: logger,
	}
}

func (t *StdioTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := t.writer.Write([]byte(header)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := t.writer.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

func (t *StdioTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	// 读取 Content-Length 头
	var contentLength int
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength); err == nil {
			continue
		}
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, err
	}

	var msg MCPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (t *StdioTransport) Close() error {
	return nil
}

```

#### 新建文件：agent/protocol/mcp/transport.go（续 — SSE 传输）

```go
// SSETransport Server-Sent Events 传输
type SSETransport struct {
	endpoint   string
	httpClient *http.Client
	eventChan  chan *MCPMessage
	sendURL    string // POST 端点，用于客户端发送消息
	logger     *zap.Logger
	cancel     context.CancelFunc
}

func NewSSETransport(endpoint string, logger *zap.Logger) *SSETransport {
	return &SSETransport{
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: 0}, // SSE 不设超时
		eventChan:  make(chan *MCPMessage, 100),
		sendURL:    endpoint + "/message",
		logger:     logger,
	}
}

func (t *SSETransport) Connect(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, "GET", t.endpoint+"/sse", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connect failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connect: unexpected status %d", resp.StatusCode)
	}

	// 后台读取 SSE 事件
	go t.readSSEEvents(ctx, resp.Body)

	return nil
}

func (t *SSETransport) readSSEEvents(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	scanner := bufio.NewScanner(body)

	var dataBuffer string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			// 空行表示事件结束
			if dataBuffer != "" {
				var msg MCPMessage
				if err := json.Unmarshal([]byte(dataBuffer), &msg); err != nil {
					t.logger.Error("SSE parse error", zap.Error(err))
				} else {
					t.eventChan <- &msg
				}
				dataBuffer = ""
			}
			continue
		}
		if len(line) > 5 && line[:5] == "data:" {
			dataBuffer += line[5:]
		}
	}
}

func (t *SSETransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.sendURL, io.NopCloser(
		io.LimitReader(bytes.NewReader(body), int64(len(body))),
	))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("SSE send: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (t *SSETransport) Receive(ctx context.Context) (*MCPMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.eventChan:
		return msg, nil
	}
}

func (t *SSETransport) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}
```

注意：SSETransport 的 `Send` 方法需要 `import "bytes"`，请在文件头部 import 中添加。

#### 新建文件：agent/protocol/mcp/transport_ws.go

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// WebSocketTransport WebSocket 传输
type WebSocketTransport struct {
	url    string
	conn   *websocket.Conn
	logger *zap.Logger
}

func NewWebSocketTransport(url string, logger *zap.Logger) *WebSocketTransport {
	return &WebSocketTransport{
		url:    url,
		logger: logger,
	}
}

func (t *WebSocketTransport) Connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, t.url, &websocket.DialOptions{
		Subprotocols: []string{"mcp"},
	})
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	t.conn = conn
	return nil
}

func (t *WebSocketTransport) Send(ctx context.Context, msg *MCPMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return t.conn.Write(ctx, websocket.MessageText, body)
}

func (t *WebSocketTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	_, data, err := t.conn.Read(ctx)
	if err != nil {
		return nil, err
	}

	var msg MCPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (t *WebSocketTransport) Close() error {
	if t.conn != nil {
		return t.conn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}
```

#### 新建文件：agent/protocol/mcp/handler.go

```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MCPHandler HTTP 处理器，将 MCP 服务器暴露为 HTTP 端点
type MCPHandler struct {
	server *DefaultMCPServer
	logger *zap.Logger

	// SSE 客户端管理
	sseClients   map[string]chan []byte
	sseClientsMu sync.RWMutex
}

func NewMCPHandler(server *DefaultMCPServer, logger *zap.Logger) *MCPHandler {
	return &MCPHandler{
		server:     server,
		logger:     logger,
		sseClients: make(map[string]chan []byte),
	}
}

// ServeHTTP 实现 http.Handler
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/mcp/sse":
		h.handleSSE(w, r)
	case "/mcp/message":
		h.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleSSE 处理 SSE 连接
func (h *MCPHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())
	ch := make(chan []byte, 100)

	h.sseClientsMu.Lock()
	h.sseClients[clientID] = ch
	h.sseClientsMu.Unlock()

	defer func() {
		h.sseClientsMu.Lock()
		delete(h.sseClients, clientID)
		h.sseClientsMu.Unlock()
		close(ch)
	}()

	// 发送 endpoint 事件（告知客户端 POST 地址）
	fmt.Fprintf(w, "event: endpoint\ndata: /mcp/message?clientId=%s\n\n", clientID)
	flusher.Flush()

	// 持续发送事件
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleMessage 处理 JSON-RPC 消息
func (h *MCPHandler) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg MCPMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		resp := NewMCPError(nil, ErrorCodeParseError, "parse error", nil)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 分发请求
	response := h.dispatch(r.Context(), &msg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	// 如果有 SSE 客户端，也推送响应
	clientID := r.URL.Query().Get("clientId")
	if clientID != "" {
		h.pushToSSEClient(clientID, response)
	}
}

// dispatch 分发 JSON-RPC 请求到对应的处理方法
func (h *MCPHandler) dispatch(ctx context.Context, msg *MCPMessage) *MCPMessage {
	switch msg.Method {
	case "initialize":
		info := h.server.GetServerInfo()
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"protocolVersion": MCPVersion,
			"capabilities":    info.Capabilities,
			"serverInfo":      info,
		})

	case "tools/list":
		tools, err := h.server.ListTools(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"tools": tools})

	case "tools/call":
		name, _ := msg.Params["name"].(string)
		args, _ := msg.Params["arguments"].(map[string]interface{})
		result, err := h.server.CallTool(ctx, name, args)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("%v", result)},
			},
		})

	case "resources/list":
		resources, err := h.server.ListResources(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"resources": resources})

	case "resources/read":
		uri, _ := msg.Params["uri"].(string)
		resource, err := h.server.GetResource(ctx, uri)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"contents": []interface{}{resource},
		})

	case "prompts/list":
		prompts, err := h.server.ListPrompts(ctx)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{"prompts": prompts})

	case "prompts/get":
		name, _ := msg.Params["name"].(string)
		varsRaw, _ := msg.Params["arguments"].(map[string]interface{})
		vars := make(map[string]string)
		for k, v := range varsRaw {
			vars[k] = fmt.Sprintf("%v", v)
		}
		result, err := h.server.GetPrompt(ctx, name, vars)
		if err != nil {
			return NewMCPError(msg.ID, ErrorCodeInternalError, err.Error(), nil)
		}
		return NewMCPResponse(msg.ID, map[string]interface{}{
			"messages": []map[string]interface{}{
				{"role": "user", "content": map[string]interface{}{"type": "text", "text": result}},
			},
		})

	case "logging/setLevel":
		level, _ := msg.Params["level"].(string)
		h.server.SetLogLevel(level)
		return NewMCPResponse(msg.ID, map[string]interface{}{})

	default:
		return NewMCPError(msg.ID, ErrorCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", msg.Method), nil)
	}
}

// pushToSSEClient 推送消息到 SSE 客户端
func (h *MCPHandler) pushToSSEClient(clientID string, msg *MCPMessage) {
	h.sseClientsMu.RLock()
	ch, exists := h.sseClients[clientID]
	h.sseClientsMu.RUnlock()

	if !exists {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case ch <- data:
	default:
		h.logger.Warn("SSE client channel full", zap.String("client_id", clientID))
	}
}
```

#### 修改文件：agent/protocol/mcp/client.go

**改动 1 — 重构 DefaultMCPClient 使用 Transport 接口（替换 reader/writer）**

将第 15-37 行的结构体定义改为：

```go
// DefaultMCPClient MCP 客户端默认实现
type DefaultMCPClient struct {
	transport  Transport    // 替换原来的 reader/writer
	serverURL  string
	serverInfo *ServerInfo

	// 请求管理
	nextID    int64
	pending   map[int64]chan *MCPMessage
	pendingMu sync.RWMutex

	// 资源订阅
	subscriptions map[string]chan Resource
	subsMu        sync.RWMutex

	// 状态
	connected bool
	mu        sync.RWMutex

	// 初始化握手完成
	initialized bool

	logger *zap.Logger
}
```

**改动 2 — 新增构造函数支持不同传输**

在第 39 行 `NewMCPClient` 后添加：

```go
// NewMCPClientWithTransport 使用指定传输层创建客户端
func NewMCPClientWithTransport(transport Transport, logger *zap.Logger) *DefaultMCPClient {
	return &DefaultMCPClient{
		transport:     transport,
		pending:       make(map[int64]chan *MCPMessage),
		subscriptions: make(map[string]chan Resource),
		logger:        logger,
	}
}

// NewSSEClient 创建 SSE 客户端
func NewSSEClient(endpoint string, logger *zap.Logger) *DefaultMCPClient {
	transport := NewSSETransport(endpoint, logger)
	return NewMCPClientWithTransport(transport, logger)
}

// NewWebSocketClient 创建 WebSocket 客户端
func NewWebSocketClient(url string, logger *zap.Logger) *DefaultMCPClient {
	transport := NewWebSocketTransport(url, logger)
	return NewMCPClientWithTransport(transport, logger)
}
```

**改动 3 — Connect 方法添加 initialize 握手（第 51-75 行）**

```go
func (c *DefaultMCPClient) Connect(ctx context.Context, serverURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	c.serverURL = serverURL

	// 如果传输层支持连接（SSE/WebSocket），先建立连接
	if connectable, ok := c.transport.(interface {
		Connect(ctx context.Context) error
	}); ok {
		if err := connectable.Connect(ctx); err != nil {
			return fmt.Errorf("transport connect failed: %w", err)
		}
	}

	// 启动消息循环
	go c.messageLoop(ctx)

	// MCP 初始化握手
	initResult, err := c.sendRequest(ctx, "initialize", map[string]interface{}{
		"protocolVersion": MCPVersion,
		"capabilities": map[string]interface{}{
			"roots": map[string]interface{}{"listChanged": true},
		},
		"clientInfo": map[string]interface{}{
			"name":    "agentflow-mcp-client",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize handshake failed: %w", err)
	}

	// 解析服务器信息
	var initResp struct {
		ServerInfo      ServerInfo         `json:"serverInfo"`
		Capabilities    ServerCapabilities `json:"capabilities"`
		ProtocolVersion string             `json:"protocolVersion"`
	}
	if err := json.Unmarshal(initResult, &initResp); err != nil {
		return fmt.Errorf("parse initialize response: %w", err)
	}

	c.serverInfo = &initResp.ServerInfo
	c.connected = true
	c.initialized = true

	// 发送 initialized 通知
	notifyMsg := &MCPMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	c.transport.Send(ctx, notifyMsg)

	c.logger.Info("MCP client initialized",
		zap.String("server", c.serverInfo.Name),
		zap.String("protocol", initResp.ProtocolVersion))

	return nil
}
```

**改动 4 — 替换 readMessage/writeMessage 为 transport 调用**

将 `sendRequest` 方法（第 304 行）中的 `c.writeMessage(msg)` 改为 `c.transport.Send(ctx, msg)`。

将 `Start` 方法（第 283 行）重命名为 `messageLoop` 并改为使用 transport：

```go
func (c *DefaultMCPClient) messageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := c.transport.Receive(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("transport receive error", zap.Error(err))
				continue
			}
			c.handleMessage(msg)
		}
	}
}
```

### 修改步骤

1. 创建 `agent/protocol/mcp/transport.go` — `Transport` 接口 + `StdioTransport` + `SSETransport`
2. 创建 `agent/protocol/mcp/transport_ws.go` — `WebSocketTransport`
3. 创建 `agent/protocol/mcp/handler.go` — `MCPHandler` HTTP 处理器
4. 修改 `agent/protocol/mcp/client.go` — 重构为使用 `Transport` 接口，添加 `initialize` 握手
5. 修改 `agent/protocol/mcp/server.go` — 在 `DefaultMCPServer` 中添加工具执行超时：
   ```go
   // CallTool 中添加超时控制（第 184 行附近）
   func (s *DefaultMCPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
       // ... 查找 handler ...
       // 添加 30 秒超时
       callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
       defer cancel()
       result, err := handler(callCtx, args)
       // ...
   }
   ```
6. 添加 `go.sum` 依赖：`nhooyr.io/websocket`（如果选择该库）

### 验证方法

```bash
# 添加 WebSocket 依赖
cd D:/code/agentflow && go get nhooyr.io/websocket

# 编译检查
go build ./agent/protocol/mcp/...

# 运行现有测试确保不破坏
go test ./agent/protocol/mcp/... -v

# 验证 Transport 接口实现
go vet ./agent/protocol/mcp/...
```

### 注意事项
- `Transport` 接口是对现有 `reader/writer` 的抽象，`StdioTransport` 保持向后兼容
- `SSETransport` 遵循 MCP 规范：GET `/sse` 接收事件，POST `/message` 发送请求
- `MCPHandler.dispatch` 方法覆盖了 MCP 规范的所有标准方法（initialize, tools/list, tools/call, resources/list, resources/read, prompts/list, prompts/get, logging/setLevel）
- `initialize` 握手是 MCP 规范要求的第一步，客户端必须先发送 `initialize` 请求，收到响应后发送 `notifications/initialized` 通知
- WebSocket 传输使用 `nhooyr.io/websocket` 库（比 `gorilla/websocket` 更现代，支持 context）
- 工具执行超时默认 30 秒，防止工具调用阻塞整个服务器

---

## P3-3: Agent 编排 DSL

### 需求背景
当前 Agent 工作流编排需要手写 Go 代码（通过 `workflow/dag_builder.go` 的 `DAGBuilder` 或直接构造 `DAGGraph`）。`workflow/dag_serialization.go` 已支持 YAML/JSON 序列化（`DAGDefinition`），但这只是数据格式，不是完整的 DSL。缺少：
- Agent 和工具的声明式定义
- 变量插值和表达式求值
- 条件表达式的文本化定义（当前 `ConditionFunc` 是 Go 函数）
- 完整的 DSL schema 校验
- 从 DSL 到可执行 `DAGGraph` 的完整转换

现有关键类型（`workflow/dag.go`）：
- `DAGGraph`：节点图（`nodes map[string]*DAGNode`，`edges map[string][]string`）
- `DAGNode`：节点（ID, Type, Step, Condition, LoopConfig, SubGraph, ErrorConfig）
- `NodeType`：action, condition, loop, parallel, subgraph, checkpoint
- `Step` 接口（`workflow/workflow.go` 第 20-25 行）：`Execute(ctx, input) (output, error)` + `Name() string`
- 内置 Step 类型（`workflow/steps.go`）：`LLMStep`, `ToolStep`, `HumanInputStep`, `CodeStep`, `PassthroughStep`

### 需要修改的文件

#### 新建文件：workflow/dsl/schema.go

```go
package dsl

// WorkflowDSL 工作流 DSL 顶层结构
type WorkflowDSL struct {
	// Version DSL 版本
	Version string `yaml:"version" json:"version"`
	// Name 工作流名称
	Name string `yaml:"name" json:"name"`
	// Description 工作流描述
	Description string `yaml:"description" json:"description"`

	// Variables 全局变量定义
	Variables map[string]VariableDef `yaml:"variables,omitempty" json:"variables,omitempty"`

	// Agents Agent 定义
	Agents map[string]AgentDef `yaml:"agents,omitempty" json:"agents,omitempty"`

	// Tools 工具定义
	Tools map[string]ToolDef `yaml:"tools,omitempty" json:"tools,omitempty"`

	// Steps 步骤定义（可复用）
	Steps map[string]StepDef `yaml:"steps,omitempty" json:"steps,omitempty"`

	// Workflow 工作流节点定义
	Workflow WorkflowNodesDef `yaml:"workflow" json:"workflow"`

	// Metadata 元数据
	Metadata map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// VariableDef 变量定义
type VariableDef struct {
	Type        string      `yaml:"type" json:"type"`                                   // string, int, float, bool, list, map
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`          // 默认值
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
}

// AgentDef Agent 定义
type AgentDef struct {
	Model       string            `yaml:"model" json:"model"`
	Provider    string            `yaml:"provider,omitempty" json:"provider,omitempty"`
	SystemPrompt string           `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
	Temperature float64           `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int               `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Tools       []string          `yaml:"tools,omitempty" json:"tools,omitempty"` // 引用 tools 中定义的工具
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// ToolDef 工具定义
type ToolDef struct {
	Type        string                 `yaml:"type" json:"type"` // builtin, mcp, http, code
	Description string                 `yaml:"description" json:"description"`
	Config      map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	InputSchema map[string]interface{} `yaml:"input_schema,omitempty" json:"input_schema,omitempty"`
}

// StepDef 步骤定义
type StepDef struct {
	Type   string                 `yaml:"type" json:"type"` // llm, tool, human_input, code, passthrough
	Agent  string                 `yaml:"agent,omitempty" json:"agent,omitempty"`   // 引用 agents 中的 agent
	Tool   string                 `yaml:"tool,omitempty" json:"tool,omitempty"`     // 引用 tools 中的工具
	Prompt string                 `yaml:"prompt,omitempty" json:"prompt,omitempty"` // 支持 ${variable} 插值
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

// WorkflowNodesDef 工作流节点定义
type WorkflowNodesDef struct {
	Entry string        `yaml:"entry" json:"entry"`
	Nodes []NodeDef     `yaml:"nodes" json:"nodes"`
}

// NodeDef 节点定义
type NodeDef struct {
	ID        string            `yaml:"id" json:"id"`
	Type      string            `yaml:"type" json:"type"` // action, condition, loop, parallel, subgraph, checkpoint
	Step      string            `yaml:"step,omitempty" json:"step,omitempty"`           // 引用 steps 中的步骤，或内联定义
	StepDef   *StepDef          `yaml:"step_def,omitempty" json:"step_def,omitempty"`   // 内联步骤定义
	Next      []string          `yaml:"next,omitempty" json:"next,omitempty"`
	Condition string            `yaml:"condition,omitempty" json:"condition,omitempty"` // 条件表达式
	OnTrue    []string          `yaml:"on_true,omitempty" json:"on_true,omitempty"`
	OnFalse   []string          `yaml:"on_false,omitempty" json:"on_false,omitempty"`
	Loop      *LoopDef          `yaml:"loop,omitempty" json:"loop,omitempty"`
	Parallel  []string          `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	SubGraph  *WorkflowNodesDef `yaml:"subgraph,omitempty" json:"subgraph,omitempty"`
	Error     *ErrorDef         `yaml:"error,omitempty" json:"error,omitempty"`
	Metadata  map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// LoopDef 循环定义
type LoopDef struct {
	Type          string `yaml:"type" json:"type"` // while, for, foreach
	Condition     string `yaml:"condition,omitempty" json:"condition,omitempty"` // 条件表达式
	MaxIterations int    `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"`
	Collection    string `yaml:"collection,omitempty" json:"collection,omitempty"` // foreach 的集合表达式
	ItemVar       string `yaml:"item_var,omitempty" json:"item_var,omitempty"`     // foreach 的迭代变量名
}

// ErrorDef 错误处理定义
type ErrorDef struct {
	Strategy      string      `yaml:"strategy" json:"strategy"` // fail_fast, skip, retry
	MaxRetries    int         `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	RetryDelayMs  int         `yaml:"retry_delay_ms,omitempty" json:"retry_delay_ms,omitempty"`
	FallbackValue interface{} `yaml:"fallback_value,omitempty" json:"fallback_value,omitempty"`
}
```

#### 新建文件：workflow/dsl/parser.go

```go
package dsl

import (
	"fmt"
	"os"
	"strings"

	"github.com/BaSui01/agentflow/workflow"
	"gopkg.in/yaml.v3"
)

// Parser DSL 解析器
type Parser struct {
	// stepRegistry 步骤注册表（step name -> Step 工厂函数）
	stepRegistry map[string]func(config map[string]interface{}) (workflow.Step, error)
	// conditionRegistry 条件表达式注册表
	conditionRegistry map[string]workflow.ConditionFunc
}

// NewParser 创建 DSL 解析器
func NewParser() *Parser {
	p := &Parser{
		stepRegistry:      make(map[string]func(config map[string]interface{}) (workflow.Step, error)),
		conditionRegistry: make(map[string]workflow.ConditionFunc),
	}
	// 注册内置步骤
	p.registerBuiltinSteps()
	return p
}

// RegisterStep 注册自定义步骤工厂
func (p *Parser) RegisterStep(name string, factory func(config map[string]interface{}) (workflow.Step, error)) {
	p.stepRegistry[name] = factory
}

// RegisterCondition 注册命名条件
func (p *Parser) RegisterCondition(name string, fn workflow.ConditionFunc) {
	p.conditionRegistry[name] = fn
}

// ParseFile 从文件解析 DSL
func (p *Parser) ParseFile(filename string) (*workflow.DAGWorkflow, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read DSL file: %w", err)
	}
	return p.Parse(data)
}

// Parse 从 YAML 字节解析 DSL
func (p *Parser) Parse(data []byte) (*workflow.DAGWorkflow, error) {
	var dsl WorkflowDSL
	if err := yaml.Unmarshal(data, &dsl); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	// 1. 验证 DSL
	if err := p.validate(&dsl); err != nil {
		return nil, fmt.Errorf("validate DSL: %w", err)
	}

	// 2. 解析变量，构建插值上下文
	vars := p.resolveVariables(dsl.Variables)

	// 3. 构建 DAGGraph
	graph, err := p.buildGraph(&dsl, vars)
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	// 4. 创建 DAGWorkflow
	wf := workflow.NewDAGWorkflow(dsl.Name, dsl.Description, graph)
	for k, v := range dsl.Metadata {
		wf.SetMetadata(k, v)
	}

	return wf, nil
}

// resolveVariables 解析变量默认值
func (p *Parser) resolveVariables(varDefs map[string]VariableDef) map[string]interface{} {
	vars := make(map[string]interface{})
	for name, def := range varDefs {
		if def.Default != nil {
			vars[name] = def.Default
		}
	}
	return vars
}

// interpolate 变量插值（替换 ${var_name}）
func (p *Parser) interpolate(template string, vars map[string]interface{}) string {
	result := template
	for name, value := range vars {
		placeholder := "${" + name + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// buildGraph 从 DSL 构建 DAGGraph
func (p *Parser) buildGraph(dsl *WorkflowDSL, vars map[string]interface{}) (*workflow.DAGGraph, error) {
	graph := workflow.NewDAGGraph()

	for _, nodeDef := range dsl.Workflow.Nodes {
		node, err := p.buildNode(&nodeDef, dsl, vars)
		if err != nil {
			return nil, fmt.Errorf("build node %s: %w", nodeDef.ID, err)
		}
		graph.AddNode(node)

		// 添加边
		for _, nextID := range nodeDef.Next {
			graph.AddEdge(nodeDef.ID, nextID)
		}

		// 条件节点的分支也作为边
		for _, trueID := range nodeDef.OnTrue {
			graph.AddEdge(nodeDef.ID, trueID)
		}
		for _, falseID := range nodeDef.OnFalse {
			graph.AddEdge(nodeDef.ID, falseID)
		}
	}

	graph.SetEntry(dsl.Workflow.Entry)
	return graph, nil
}

// buildNode 构建单个节点
func (p *Parser) buildNode(def *NodeDef, dsl *WorkflowDSL, vars map[string]interface{}) (*workflow.DAGNode, error) {
	node := &workflow.DAGNode{
		ID:       def.ID,
		Type:     workflow.NodeType(def.Type),
		Metadata: make(map[string]any),
	}

	// 复制 metadata
	for k, v := range def.Metadata {
		node.Metadata[k] = v
	}

	switch workflow.NodeType(def.Type) {
	case workflow.NodeTypeAction:
		step, err := p.resolveStep(def, dsl, vars)
		if err != nil {
			return nil, err
		}
		node.Step = step

	case workflow.NodeTypeCondition:
		condFn, err := p.resolveCondition(def.Condition, vars)
		if err != nil {
			return nil, err
		}
		node.Condition = condFn
		if len(def.OnTrue) > 0 {
			node.Metadata["on_true"] = def.OnTrue
		}
		if len(def.OnFalse) > 0 {
			node.Metadata["on_false"] = def.OnFalse
		}

	case workflow.NodeTypeLoop:
		if def.Loop == nil {
			return nil, fmt.Errorf("loop node requires loop definition")
		}
		loopConfig, err := p.resolveLoop(def.Loop, vars)
		if err != nil {
			return nil, err
		}
		node.LoopConfig = loopConfig

	case workflow.NodeTypeParallel:
		// parallel 节点的边在 buildGraph 中处理

	case workflow.NodeTypeSubGraph:
		if def.SubGraph != nil {
			subDSL := &WorkflowDSL{
				Name:     dsl.Name + "_sub",
				Workflow: *def.SubGraph,
			}
			subGraph, err := p.buildGraph(subDSL, vars)
			if err != nil {
				return nil, fmt.Errorf("build subgraph: %w", err)
			}
			node.SubGraph = subGraph
		}
	}

	// 错误处理配置
	if def.Error != nil {
		node.ErrorConfig = &workflow.ErrorConfig{
			Strategy:      workflow.ErrorStrategy(def.Error.Strategy),
			MaxRetries:    def.Error.MaxRetries,
			RetryDelayMs:  def.Error.RetryDelayMs,
			FallbackValue: def.Error.FallbackValue,
		}
	}

	return node, nil
}

// resolveStep 解析步骤（引用或内联）
func (p *Parser) resolveStep(def *NodeDef, dsl *WorkflowDSL, vars map[string]interface{}) (workflow.Step, error) {
	var stepDef *StepDef

	if def.StepDef != nil {
		// 内联步骤定义
		stepDef = def.StepDef
	} else if def.Step != "" {
		// 引用已定义的步骤
		sd, ok := dsl.Steps[def.Step]
		if !ok {
			return nil, fmt.Errorf("step %q not found in steps definitions", def.Step)
		}
		stepDef = &sd
	} else {
		return nil, fmt.Errorf("action node requires step or step_def")
	}

	// 根据类型创建 Step
	switch stepDef.Type {
	case "llm":
		prompt := p.interpolate(stepDef.Prompt, vars)
		return &workflow.LLMStep{
			Model:  stepDef.Agent,
			Prompt: prompt,
		}, nil

	case "tool":
		params := make(map[string]any)
		for k, v := range stepDef.Config {
			if s, ok := v.(string); ok {
				params[k] = p.interpolate(s, vars)
			} else {
				params[k] = v
			}
		}
		return &workflow.ToolStep{
			ToolName: stepDef.Tool,
			Params:   params,
		}, nil

	case "passthrough":
		return &workflow.PassthroughStep{}, nil

	default:
		// 查找注册的自定义步骤
		factory, ok := p.stepRegistry[stepDef.Type]
		if !ok {
			return nil, fmt.Errorf("unknown step type: %s", stepDef.Type)
		}
		return factory(stepDef.Config)
	}
}

// resolveCondition 解析条件表达式
func (p *Parser) resolveCondition(expr string, vars map[string]interface{}) (workflow.ConditionFunc, error) {
	// 1. 检查是否是注册的命名条件
	if fn, ok := p.conditionRegistry[expr]; ok {
		return fn, nil
	}

	// 2. 简单表达式解析（支持 ${var} == value, ${var} > value 等）
	return p.parseSimpleExpression(expr, vars)
}

// parseSimpleExpression 解析简单条件表达式
func (p *Parser) parseSimpleExpression(expr string, vars map[string]interface{}) (workflow.ConditionFunc, error) {
	// 支持格式：${var} op value
	// op: ==, !=, >, <, >=, <=, contains
	return func(ctx context.Context, input interface{}) (bool, error) {
		// 运行时求值
		resolved := p.interpolate(expr, vars)
		// 简化实现：非空字符串为 true
		return resolved != "" && resolved != "false" && resolved != "0", nil
	}, nil
}

// resolveLoop 解析循环配置
func (p *Parser) resolveLoop(def *LoopDef, vars map[string]interface{}) (*workflow.LoopConfig, error) {
	config := &workflow.LoopConfig{
		Type:          workflow.LoopType(def.Type),
		MaxIterations: def.MaxIterations,
	}

	if def.Condition != "" {
		condFn, err := p.resolveCondition(def.Condition, vars)
		if err != nil {
			return nil, err
		}
		config.Condition = condFn
	}

	return config, nil
}

// registerBuiltinSteps 注册内置步骤
func (p *Parser) registerBuiltinSteps() {
	p.RegisterStep("passthrough", func(config map[string]interface{}) (workflow.Step, error) {
		return &workflow.PassthroughStep{}, nil
	})
}
```

注意：`parseSimpleExpression` 中需要 `import "context"`。

#### 新建文件：workflow/dsl/validator.go

```go
package dsl

import (
	"fmt"
	"strings"
)

// Validator DSL 验证器
type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

// Validate 验证 DSL 定义
func (v *Validator) Validate(dsl *WorkflowDSL) []error {
	var errs []error

	// 基础字段验证
	if dsl.Version == "" {
		errs = append(errs, fmt.Errorf("version is required"))
	}
	if dsl.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}
	if dsl.Workflow.Entry == "" {
		errs = append(errs, fmt.Errorf("workflow.entry is required"))
	}
	if len(dsl.Workflow.Nodes) == 0 {
		errs = append(errs, fmt.Errorf("workflow.nodes must have at least one node"))
	}

	// 收集所有节点 ID
	nodeIDs := make(map[string]bool)
	for _, node := range dsl.Workflow.Nodes {
		if node.ID == "" {
			errs = append(errs, fmt.Errorf("node ID is required"))
			continue
		}
		if nodeIDs[node.ID] {
			errs = append(errs, fmt.Errorf("duplicate node ID: %s", node.ID))
		}
		nodeIDs[node.ID] = true
	}

	// 验证 entry 节点存在
	if !nodeIDs[dsl.Workflow.Entry] {
		errs = append(errs, fmt.Errorf("entry node %q does not exist", dsl.Workflow.Entry))
	}

	// 验证每个节点
	for _, node := range dsl.Workflow.Nodes {
		errs = append(errs, v.validateNode(&node, dsl, nodeIDs)...)
	}

	// 验证引用完整性
	errs = append(errs, v.validateReferences(dsl, nodeIDs)...)

	return errs
}

// validateNode 验证单个节点
func (v *Validator) validateNode(node *NodeDef, dsl *WorkflowDSL, nodeIDs map[string]bool) []error {
	var errs []error

	validTypes := map[string]bool{
		"action": true, "condition": true, "loop": true,
		"parallel": true, "subgraph": true, "checkpoint": true,
	}
	if !validTypes[node.Type] {
		errs = append(errs, fmt.Errorf("node %s: invalid type %q", node.ID, node.Type))
	}

	switch node.Type {
	case "action":
		if node.Step == "" && node.StepDef == nil {
			errs = append(errs, fmt.Errorf("node %s: action node requires step or step_def", node.ID))
		}
		if node.Step != "" {
			if _, ok := dsl.Steps[node.Step]; !ok {
				errs = append(errs, fmt.Errorf("node %s: step %q not found in steps", node.ID, node.Step))
			}
		}

	case "condition":
		if node.Condition == "" {
			errs = append(errs, fmt.Errorf("node %s: condition node requires condition expression", node.ID))
		}
		if len(node.OnTrue) == 0 && len(node.OnFalse) == 0 {
			errs = append(errs, fmt.Errorf("node %s: condition node requires on_true or on_false", node.ID))
		}

	case "loop":
		if node.Loop == nil {
			errs = append(errs, fmt.Errorf("node %s: loop node requires loop definition", node.ID))
		} else {
			if node.Loop.Type == "" {
				errs = append(errs, fmt.Errorf("node %s: loop type is required", node.ID))
			}
			if node.Loop.Type == "while" && node.Loop.Condition == "" {
				errs = append(errs, fmt.Errorf("node %s: while loop requires condition", node.ID))
			}
			if node.Loop.Type == "for" && node.Loop.MaxIterations <= 0 {
				errs = append(errs, fmt.Errorf("node %s: for loop requires positive max_iterations", node.ID))
			}
		}

	case "parallel":
		if len(node.Next) < 2 && len(node.Parallel) < 2 {
			errs = append(errs, fmt.Errorf("node %s: parallel node requires at least 2 branches", node.ID))
		}

	case "subgraph":
		if node.SubGraph == nil {
			errs = append(errs, fmt.Errorf("node %s: subgraph node requires subgraph definition", node.ID))
		}
	}

	// 验证引用的节点存在
	for _, nextID := range node.Next {
		if !nodeIDs[nextID] {
			errs = append(errs, fmt.Errorf("node %s: next node %q does not exist", node.ID, nextID))
		}
	}
	for _, id := range node.OnTrue {
		if !nodeIDs[id] {
			errs = append(errs, fmt.Errorf("node %s: on_true node %q does not exist", node.ID, id))
		}
	}
	for _, id := range node.OnFalse {
		if !nodeIDs[id] {
			errs = append(errs, fmt.Errorf("node %s: on_false node %q does not exist", node.ID, id))
		}
	}

	return errs
}

// validateReferences 验证所有引用的完整性
func (v *Validator) validateReferences(dsl *WorkflowDSL, nodeIDs map[string]bool) []error {
	var errs []error

	// 验证 step 中引用的 agent 和 tool 存在
	for stepName, step := range dsl.Steps {
		if step.Agent != "" {
			if _, ok := dsl.Agents[step.Agent]; !ok {
				errs = append(errs, fmt.Errorf("step %s: agent %q not found", stepName, step.Agent))
			}
		}
		if step.Tool != "" {
			if _, ok := dsl.Tools[step.Tool]; !ok {
				errs = append(errs, fmt.Errorf("step %s: tool %q not found", stepName, step.Tool))
			}
		}
	}

	// 验证 agent 中引用的 tool 存在
	for agentName, agent := range dsl.Agents {
		for _, toolName := range agent.Tools {
			if _, ok := dsl.Tools[toolName]; !ok {
				errs = append(errs, fmt.Errorf("agent %s: tool %q not found", agentName, toolName))
			}
		}
	}

	// 验证变量插值引用
	for stepName, step := range dsl.Steps {
		if step.Prompt != "" {
			refs := extractVariableRefs(step.Prompt)
			for _, ref := range refs {
				if _, ok := dsl.Variables[ref]; !ok {
					errs = append(errs, fmt.Errorf("step %s: variable %q referenced in prompt not defined", stepName, ref))
				}
			}
		}
	}

	return errs
}

// extractVariableRefs 提取 ${var} 引用
func extractVariableRefs(s string) []string {
	var refs []string
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		ref := s[start+2 : start+end]
		refs = append(refs, ref)
		s = s[start+end+1:]
	}
	return refs
}
```

#### 示例 DSL 文件：workflow/dsl/examples/customer_support.yaml

```yaml
version: "1.0"
name: customer-support-workflow
description: 客户支持自动化工作流

variables:
  language:
    type: string
    default: "zh-CN"
    description: 响应语言
  max_search_results:
    type: int
    default: 5

agents:
  classifier:
    model: gpt-4o-mini
    system_prompt: "你是一个客户问题分类器。将问题分为：billing, technical, general"
    temperature: 0.1
    max_tokens: 100

  responder:
    model: claude-sonnet-4-20250514
    system_prompt: "你是一个专业的客户支持代表。使用 ${language} 回复。"
    temperature: 0.7
    max_tokens: 2000
    tools:
      - knowledge_search

tools:
  knowledge_search:
    type: builtin
    description: 搜索知识库
    config:
      index: customer_support_kb
      top_k: ${max_search_results}

steps:
  classify:
    type: llm
    agent: classifier
    prompt: "请分类以下客户问题：${input}"

  search_kb:
    type: tool
    tool: knowledge_search

  generate_response:
    type: llm
    agent: responder
    prompt: "基于以下知识库结果回答客户问题"

  escalate:
    type: human_input
    config:
      prompt: "需要人工介入处理此问题"
      timeout: 300

workflow:
  entry: classify_input
  nodes:
    - id: classify_input
      type: action
      step: classify
      next: [check_category]

    - id: check_category
      type: condition
      condition: "${classification} == technical"
      on_true: [search_knowledge]
      on_false: [check_billing]

    - id: check_billing
      type: condition
      condition: "${classification} == billing"
      on_true: [escalate_to_human]
      on_false: [generate_general_response]

    - id: search_knowledge
      type: action
      step: search_kb
      next: [generate_tech_response]
      error:
        strategy: skip
        fallback_value: "未找到相关知识"

    - id: generate_tech_response
      type: action
      step: generate_response
      next: []

    - id: generate_general_response
      type: action
      step: generate_response
      next: []

    - id: escalate_to_human
      type: action
      step: escalate
      next: []
```

#### 新建文件：workflow/dsl/parser_test.go

```go
package dsl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_ParseSimpleWorkflow(t *testing.T) {
	yamlData := []byte(`
version: "1.0"
name: test-workflow
description: 测试工作流

steps:
  step1:
    type: passthrough

workflow:
  entry: node1
  nodes:
    - id: node1
      type: action
      step: step1
      next: [node2]
    - id: node2
      type: action
      step: step1
`)

	parser := NewParser()
	wf, err := parser.Parse(yamlData)
	require.NoError(t, err)
	assert.Equal(t, "test-workflow", wf.Name())
	assert.Equal(t, "测试工作流", wf.Description())

	graph := wf.Graph()
	assert.NotNil(t, graph)
	assert.Equal(t, "node1", graph.GetEntry())

	node1, exists := graph.GetNode("node1")
	assert.True(t, exists)
	assert.NotNil(t, node1.Step)

	edges := graph.GetEdges("node1")
	assert.Equal(t, []string{"node2"}, edges)
}

func TestValidator_DetectErrors(t *testing.T) {
	dsl := &WorkflowDSL{
		Name: "test",
		Workflow: WorkflowNodesDef{
			Entry: "missing_node",
			Nodes: []NodeDef{
				{ID: "node1", Type: "action"}, // 缺少 step
			},
		},
	}

	validator := NewValidator()
	errs := validator.Validate(dsl)
	assert.True(t, len(errs) > 0)

	// 应检测到：version 缺失、entry 不存在、action 缺少 step
	errMsgs := make([]string, len(errs))
	for i, e := range errs {
		errMsgs[i] = e.Error()
	}
	assert.Contains(t, errMsgs[0], "version")
}

func TestParser_VariableInterpolation(t *testing.T) {
	parser := NewParser()
	result := parser.interpolate("Hello ${name}, you have ${count} items", map[string]interface{}{
		"name":  "Alice",
		"count": 42,
	})
	assert.Equal(t, "Hello Alice, you have 42 items", result)
}
```

### 修改步骤

1. 创建 `workflow/dsl/` 目录
2. 创建 `workflow/dsl/schema.go` — DSL 类型定义
3. 创建 `workflow/dsl/parser.go` — DSL 解析器（YAML -> DAGGraph）
4. 创建 `workflow/dsl/validator.go` — DSL 验证器
5. 创建 `workflow/dsl/parser_test.go` — 测试
6. 创建 `workflow/dsl/examples/customer_support.yaml` — 示例 DSL
7. 确保 `Parser` 的 `validate` 方法内部调用 `Validator.Validate`：
   ```go
   func (p *Parser) validate(dsl *WorkflowDSL) error {
       v := NewValidator()
       errs := v.Validate(dsl)
       if len(errs) > 0 {
           msgs := make([]string, len(errs))
           for i, e := range errs {
               msgs[i] = e.Error()
           }
           return fmt.Errorf("validation errors: %s", strings.Join(msgs, "; "))
       }
       return nil
   }
   ```

### 验证方法

```bash
# 创建目录
mkdir -p D:/code/agentflow/workflow/dsl/examples

# 编译检查
cd D:/code/agentflow && go build ./workflow/dsl/...

# 运行测试
go test ./workflow/dsl/ -v

# 验证示例 DSL 文件可解析
go test ./workflow/dsl/ -run TestParser_ParseSimpleWorkflow -v
```

### 注意事项
- DSL schema 与现有 `DAGDefinition`（`workflow/dag.go` 第 161-172 行）兼容但更丰富，增加了 agents/tools/variables/steps 声明
- `Parser` 将 DSL 转换为 `DAGGraph`（`workflow/dag.go` 第 100-109 行），复用现有的 `DAGExecutor` 执行
- 条件表达式当前使用简化实现（字符串非空判断），后续可集成 `expr` 或 `cel-go` 表达式引擎
- 变量插值使用 `${var}` 语法，与 shell 变量一致
- `StepDef` 支持引用（`step: step_name`）和内联（`step_def: {...}`）两种方式
- 验证器检查所有引用完整性：节点引用、步骤引用、Agent 引用、工具引用、变量引用

---

## P3-4: 统一 Token 计数器

### 需求背景
不同 LLM Provider 的 token 计数方式不同（OpenAI 系列用 tiktoken/BPE，Anthropic 用自己的 tokenizer，中文模型可能用 SentencePiece）。当前代码中：
- `llm/provider.go` 的 `ChatUsage`（第 121-125 行）只在响应中返回 token 数，没有预估能力
- `llm/tools/cost_control.go` 的 `CalculateCost` 使用 `len(args)/4` 粗略估算 token 数
- 各 Provider 实现（`llm/providers/` 下 13+ 目录）没有统一的 tokenizer 接口
- `ChatRequest`（第 80-100 行）有 `MaxTokens` 字段但无法预估请求会消耗多少 token

需要统一的 token 计数接口，用于：成本预估、上下文窗口管理、请求前的 token 预算检查。

### 需要修改的文件

#### 新建文件：llm/tokenizer/tokenizer.go

```go
package tokenizer

import (
	"fmt"
	"sync"
)

// Tokenizer 统一 token 计数器接口
type Tokenizer interface {
	// CountTokens 计算文本的 token 数
	CountTokens(text string) (int, error)

	// CountMessages 计算消息列表的 token 数（包含角色标记等开销）
	CountMessages(messages []Message) (int, error)

	// Encode 将文本编码为 token ID 列表
	Encode(text string) ([]int, error)

	// Decode 将 token ID 列表解码为文本
	Decode(tokens []int) (string, error)

	// MaxTokens 返回模型的最大 token 数
	MaxTokens() int

	// Name 返回 tokenizer 名称
	Name() string
}

// Message 简化的消息结构（避免循环依赖）
type Message struct {
	Role    string
	Content string
}

// ModelTokenizerMap 模型到 tokenizer 的映射
var (
	modelTokenizers   = make(map[string]Tokenizer)
	modelTokenizersMu sync.RWMutex
)

// RegisterTokenizer 注册模型的 tokenizer
func RegisterTokenizer(model string, t Tokenizer) {
	modelTokenizersMu.Lock()
	defer modelTokenizersMu.Unlock()
	modelTokenizers[model] = t
}

// GetTokenizer 获取模型的 tokenizer
func GetTokenizer(model string) (Tokenizer, error) {
	modelTokenizersMu.RLock()
	defer modelTokenizersMu.RUnlock()

	if t, ok := modelTokenizers[model]; ok {
		return t, nil
	}

	// 尝试前缀匹配（如 "gpt-4o" 匹配 "gpt-4o-mini"）
	for prefix, t := range modelTokenizers {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return t, nil
		}
	}

	return nil, fmt.Errorf("no tokenizer registered for model: %s", model)
}

// GetTokenizerOrEstimator 获取 tokenizer，如果没有注册则返回通用估算器
func GetTokenizerOrEstimator(model string) Tokenizer {
	t, err := GetTokenizer(model)
	if err != nil {
		return NewEstimatorTokenizer(model, 0)
	}
	return t
}
```

#### 新建文件：llm/tokenizer/estimator.go

```go
package tokenizer

import (
	"unicode/utf8"
)

// EstimatorTokenizer 基于字符数的通用估算器
// 适用于没有精确 tokenizer 的模型
type EstimatorTokenizer struct {
	model     string
	maxTokens int

	// 不同语言的字符/token 比率
	// 英文约 4 字符/token，中文约 1.5 字符/token
	// 混合文本取平均约 2.5 字符/token
	charsPerToken float64
}

// NewEstimatorTokenizer 创建通用估算器
func NewEstimatorTokenizer(model string, maxTokens int) *EstimatorTokenizer {
	if maxTokens <= 0 {
		maxTokens = 4096 // 默认
	}
	return &EstimatorTokenizer{
		model:         model,
		maxTokens:     maxTokens,
		charsPerToken: 2.5, // 混合语言默认值
	}
}

// WithCharsPerToken 设置字符/token 比率
func (e *EstimatorTokenizer) WithCharsPerToken(ratio float64) *EstimatorTokenizer {
	e.charsPerToken = ratio
	return e
}

func (e *EstimatorTokenizer) CountTokens(text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	// 智能估算：根据文本中的中文/英文比例动态调整
	totalChars := utf8.RuneCountInString(text)
	cjkCount := 0
	for _, r := range text {
		if isCJK(r) {
			cjkCount++
		}
	}

	// CJK 字符约 1.5 char/token，ASCII 约 4 char/token
	cjkTokens := float64(cjkCount) / 1.5
	asciiTokens := float64(totalChars-cjkCount) / 4.0
	estimated := int(cjkTokens + asciiTokens)

	if estimated == 0 {
		estimated = 1
	}

	return estimated, nil
}

func (e *EstimatorTokenizer) CountMessages(messages []Message) (int, error) {
	total := 0
	for _, msg := range messages {
		// 每条消息有 ~4 token 的开销（角色标记等）
		tokens, err := e.CountTokens(msg.Content)
		if err != nil {
			return 0, err
		}
		total += tokens + 4
	}
	// 对话结尾有 ~3 token 的开销
	total += 3
	return total, nil
}

func (e *EstimatorTokenizer) Encode(text string) ([]int, error) {
	// 估算器无法真正编码，返回伪 token ID
	count, err := e.CountTokens(text)
	if err != nil {
		return nil, err
	}
	tokens := make([]int, count)
	for i := range tokens {
		tokens[i] = i
	}
	return tokens, nil
}

func (e *EstimatorTokenizer) Decode(tokens []int) (string, error) {
	// 估算器无法真正解码
	return "", fmt.Errorf("estimator tokenizer does not support decode")
}

func (e *EstimatorTokenizer) MaxTokens() int {
	return e.maxTokens
}

func (e *EstimatorTokenizer) Name() string {
	return "estimator"
}

// isCJK 判断是否为 CJK 字符
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Halfwidth and Fullwidth Forms
}
```

注意：`Decode` 方法中需要 `import "fmt"`。

#### 新建文件：llm/tokenizer/tiktoken.go

```go
package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TiktokenTokenizer tiktoken 适配器（OpenAI 系列模型）
type TiktokenTokenizer struct {
	model     string
	encoding  string
	maxTokens int
	enc       *tiktoken.Tiktoken
	once      sync.Once
	initErr   error
}

// 模型到 encoding 的映射
var modelEncodings = map[string]struct {
	encoding  string
	maxTokens int
}{
	"gpt-4o":          {encoding: "o200k_base", maxTokens: 128000},
	"gpt-4o-mini":     {encoding: "o200k_base", maxTokens: 128000},
	"gpt-4-turbo":     {encoding: "cl100k_base", maxTokens: 128000},
	"gpt-4":           {encoding: "cl100k_base", maxTokens: 8192},
	"gpt-3.5-turbo":   {encoding: "cl100k_base", maxTokens: 16385},
	"text-embedding-3-large": {encoding: "cl100k_base", maxTokens: 8191},
	"text-embedding-3-small": {encoding: "cl100k_base", maxTokens: 8191},
}

// NewTiktokenTokenizer 创建 tiktoken tokenizer
func NewTiktokenTokenizer(model string) (*TiktokenTokenizer, error) {
	info, ok := modelEncodings[model]
	if !ok {
		// 尝试前缀匹配
		for prefix, i := range modelEncodings {
			if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
				info = i
				ok = true
				break
			}
		}
	}

	if !ok {
		// 默认使用 cl100k_base
		info = struct {
			encoding  string
			maxTokens int
		}{encoding: "cl100k_base", maxTokens: 8192}
	}

	return &TiktokenTokenizer{
		model:     model,
		encoding:  info.encoding,
		maxTokens: info.maxTokens,
	}, nil
}

// init 延迟初始化 encoding（tiktoken 初始化可能需要下载数据）
func (t *TiktokenTokenizer) init() error {
	t.once.Do(func() {
		enc, err := tiktoken.GetEncoding(t.encoding)
		if err != nil {
			t.initErr = fmt.Errorf("init tiktoken encoding %s: %w", t.encoding, err)
			return
		}
		t.enc = enc
	})
	return t.initErr
}

func (t *TiktokenTokenizer) CountTokens(text string) (int, error) {
	if err := t.init(); err != nil {
		return 0, err
	}
	tokens := t.enc.Encode(text, nil, nil)
	return len(tokens), nil
}

func (t *TiktokenTokenizer) CountMessages(messages []Message) (int, error) {
	if err := t.init(); err != nil {
		return 0, err
	}

	total := 0
	for _, msg := range messages {
		// 每条消息的开销：<|start|>role\n content <|end|>\n
		total += 4
		tokens := t.enc.Encode(msg.Content, nil, nil)
		total += len(tokens)
		roleTokens := t.enc.Encode(msg.Role, nil, nil)
		total += len(roleTokens)
	}
	total += 3 // 对话结尾开销
	return total, nil
}

func (t *TiktokenTokenizer) Encode(text string) ([]int, error) {
	if err := t.init(); err != nil {
		return nil, err
	}
	return t.enc.Encode(text, nil, nil), nil
}

func (t *TiktokenTokenizer) Decode(tokens []int) (string, error) {
	if err := t.init(); err != nil {
		return "", err
	}
	return t.enc.Decode(tokens), nil
}

func (t *TiktokenTokenizer) MaxTokens() int {
	return t.maxTokens
}

func (t *TiktokenTokenizer) Name() string {
	return fmt.Sprintf("tiktoken[%s]", t.encoding)
}

// RegisterOpenAITokenizers 注册所有 OpenAI 模型的 tokenizer
func RegisterOpenAITokenizers() {
	for model := range modelEncodings {
		t, err := NewTiktokenTokenizer(model)
		if err != nil {
			continue
		}
		RegisterTokenizer(model, t)
	}
}
```

#### 新建文件：llm/tokenizer/tokenizer_test.go

```go
package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimatorTokenizer_English(t *testing.T) {
	est := NewEstimatorTokenizer("test-model", 4096)

	count, err := est.CountTokens("Hello, world! This is a test.")
	require.NoError(t, err)
	// 30 ASCII chars / 4 ≈ 7-8 tokens
	assert.InDelta(t, 7, count, 3)
}

func TestEstimatorTokenizer_Chinese(t *testing.T) {
	est := NewEstimatorTokenizer("test-model", 4096)

	count, err := est.CountTokens("你好世界，这是一个测试。")
	require.NoError(t, err)
	// 12 CJK chars / 1.5 ≈ 8 tokens
	assert.InDelta(t, 8, count, 3)
}

func TestEstimatorTokenizer_Mixed(t *testing.T) {
	est := NewEstimatorTokenizer("test-model", 4096)

	count, err := est.CountTokens("Hello 你好 World 世界")
	require.NoError(t, err)
	assert.True(t, count > 0)
}

func TestEstimatorTokenizer_Empty(t *testing.T) {
	est := NewEstimatorTokenizer("test-model", 4096)

	count, err := est.CountTokens("")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestEstimatorTokenizer_CountMessages(t *testing.T) {
	est := NewEstimatorTokenizer("test-model", 4096)

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
	}

	count, err := est.CountMessages(messages)
	require.NoError(t, err)
	// 2 messages * 4 overhead + content tokens + 3 end overhead
	assert.True(t, count > 10)
}

func TestTiktokenTokenizer_GPT4o(t *testing.T) {
	tk, err := NewTiktokenTokenizer("gpt-4o")
	require.NoError(t, err)

	count, err := tk.CountTokens("Hello, world!")
	require.NoError(t, err)
	assert.True(t, count > 0)
	assert.True(t, count < 10)

	assert.Equal(t, 128000, tk.MaxTokens())
	assert.Contains(t, tk.Name(), "tiktoken")
}

func TestTiktokenTokenizer_EncodeDecode(t *testing.T) {
	tk, err := NewTiktokenTokenizer("gpt-4")
	require.NoError(t, err)

	text := "Hello, world!"
	tokens, err := tk.Encode(text)
	require.NoError(t, err)
	assert.True(t, len(tokens) > 0)

	decoded, err := tk.Decode(tokens)
	require.NoError(t, err)
	assert.Equal(t, text, decoded)
}

func TestGetTokenizerOrEstimator(t *testing.T) {
	// 注册一个 tokenizer
	RegisterTokenizer("test-model", NewEstimatorTokenizer("test-model", 4096))

	// 已注册的模型
	tk := GetTokenizerOrEstimator("test-model")
	assert.Equal(t, "estimator", tk.Name())

	// 未注册的模型应返回估算器
	tk2 := GetTokenizerOrEstimator("unknown-model")
	assert.Equal(t, "estimator", tk2.Name())
}

func TestRegisterOpenAITokenizers(t *testing.T) {
	RegisterOpenAITokenizers()

	tk, err := GetTokenizer("gpt-4o")
	require.NoError(t, err)
	assert.Contains(t, tk.Name(), "tiktoken")
}
```

#### 修改文件：llm/provider.go — 扩展 Provider 接口（可选）

在 `Provider` 接口（第 55-70 行）旁添加可选的 `TokenizerProvider` 接口：

```go
// TokenizerProvider 可选接口，Provider 可以实现此接口返回对应的 Tokenizer
// 不修改现有 Provider 接口，保持向后兼容
type TokenizerProvider interface {
	// GetTokenizer 返回该 Provider 对应的 Tokenizer
	GetTokenizer(model string) (interface{}, error)
}
```

注意：这里返回 `interface{}` 而不是直接引用 `tokenizer.Tokenizer`，避免 `llm` 包对 `llm/tokenizer` 的循环依赖。调用方需要类型断言。

#### 修改文件：llm/tools/cost_control.go — 集成 Tokenizer

在 `CalculateCost` 方法中（P0-2 已改进的版本），将 `TokenCounter` 接口替换为使用 `tokenizer` 包：

```go
import "github.com/BaSui01/agentflow/llm/tokenizer"

// 在 DefaultCostController 中替换 tokenCounter 字段
type DefaultCostController struct {
    // ... 其他字段 ...
    tokenizer tokenizer.Tokenizer // 替换原来的 TokenCounter
}

// CalculateCost 中使用 tokenizer
func (cc *DefaultCostController) CalculateCost(toolName string, args json.RawMessage) (float64, error) {
    // ... 查找 toolCost ...

    if toolCost.CostPerUnit > 0 && len(args) > 0 {
        switch toolCost.Unit {
        case CostUnitTokens:
            if cc.tokenizer != nil {
                tokens, err := cc.tokenizer.CountTokens(string(args))
                if err == nil {
                    cost += float64(tokens) * toolCost.CostPerUnit
                    break
                }
            }
            // fallback: 使用通用估算器
            est := tokenizer.NewEstimatorTokenizer("", 0)
            tokens, _ := est.CountTokens(string(args))
            cost += float64(tokens) * toolCost.CostPerUnit
        // ...
        }
    }
    return cost, nil
}
```

### 修改步骤

1. 创建 `llm/tokenizer/` 目录
2. 创建 `llm/tokenizer/tokenizer.go` — `Tokenizer` 接口 + 全局注册表
3. 创建 `llm/tokenizer/estimator.go` — 通用估算器（基于字符数，区分 CJK/ASCII）
4. 创建 `llm/tokenizer/tiktoken.go` — tiktoken 适配器（OpenAI 系列）
5. 创建 `llm/tokenizer/tokenizer_test.go` — 测试
6. 添加依赖：`go get github.com/pkoukk/tiktoken-go`
7. 在 `llm/provider.go` 中添加 `TokenizerProvider` 可选接口
8. 修改 `llm/tools/cost_control.go` 集成 tokenizer（如果 P0-2 已完成）

### 验证方法

```bash
# 添加 tiktoken 依赖
cd D:/code/agentflow && go get github.com/pkoukk/tiktoken-go

# 创建目录
mkdir -p D:/code/agentflow/llm/tokenizer

# 编译检查
go build ./llm/tokenizer/...

# 运行测试
go test ./llm/tokenizer/ -v

# 验证估算器精度（中英文混合）
go test ./llm/tokenizer/ -run TestEstimatorTokenizer -v

# 验证 tiktoken 集成
go test ./llm/tokenizer/ -run TestTiktokenTokenizer -v

# 确保不破坏现有代码
go build ./llm/...
go test ./llm/... -count=1
```

### 注意事项
- `Tokenizer` 接口定义在独立的 `llm/tokenizer` 包中，避免与 `llm` 包循环依赖
- `EstimatorTokenizer` 区分 CJK 和 ASCII 字符，比简单的 `len/4` 更准确（CJK 约 1.5 char/token，ASCII 约 4 char/token）
- `TiktokenTokenizer` 使用延迟初始化（`sync.Once`），因为 tiktoken 初始化可能需要下载编码数据
- `RegisterOpenAITokenizers()` 应在应用启动时调用一次
- `GetTokenizerOrEstimator` 提供 fallback 机制，确保任何模型都能获得 token 计数能力
- `TokenizerProvider` 是可选接口，不修改现有 `Provider` 接口，保持向后兼容
- tiktoken-go 库（`github.com/pkoukk/tiktoken-go`）是 OpenAI tiktoken 的 Go 移植，支持 `cl100k_base` 和 `o200k_base` 编码
