package router

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// 编译时间界面检查.
var _ llmpkg.Provider = (*ABRouter)(nil)

// ABVariant代表A/B测试中的一个变体.
type ABVariant struct {
	// 名称是变体标识符(例如"control","experiment a").
	Name string
	// 提供方是这个变体所使用的LLM提供者.
	Provider llmpkg.Provider
	// 重量为交通重量(0-100). 所有变相权重必须相加为100.
	Weight int
	// 元数据为这个变体持有任意的密钥值对.
	Metadata map[string]string
}

// ABMetrics收集了每个变量的请求量度.
type ABMetrics struct {
	VariantName    string
	TotalRequests  int64
	SuccessCount   int64
	FailureCount   int64
	TotalLatencyMs int64
	TotalCost      float64
	QualityScores  []float64
	mu             sync.Mutex
}

// 记录请求记录一个请求结果。
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

// GetAvgLatencyMs 返回以毫秒为单位的平均纬度.
func (m *ABMetrics) GetAvgLatencyMs() float64 {
	total := atomic.LoadInt64(&m.TotalRequests)
	if total == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.TotalLatencyMs)) / float64(total)
}

// GetSuccessRate 返回成功率为 0 到 1 之间的值 。
func (m *ABMetrics) GetSuccessRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.SuccessCount + m.FailureCount
	if total == 0 {
		return 0
	}
	return float64(m.SuccessCount) / float64(total)
}

// GetAvg质量Score返回平均质量分.
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

// ABTestConfig持有A/B测试的配置.
type ABTestConfig struct {
	// 名称表示此测试 。
	Name string
	// 变体列出了测试变体.
	Variants []ABVariant
	// 粘接可以确定同一用户/会话的路径。
	StickyRouting bool
	// 粘接Key选择请求字段使用的:"user id","session id",或"tenant id".
	StickyKey string
	// 开始时间是测试开始的时候.
	StartTime time.Time
	// EndTime是测试结束的时候(零值表示无限期).
	EndTime time.Time
}

// ABRouter是一个A/B测试路由器,用于执行lmpkg. 供养者.
type ABRouter struct {
	config  ABTestConfig
	metrics map[string]*ABMetrics // variantName -> metrics

	// 粘接路由缓存.
	stickyCache   map[string]string // stickyKey -> variantName
	stickyCacheMu sync.RWMutex

	// 动态重量调整.
	dynamicWeights map[string]int // variantName -> weight
	weightsMu      sync.RWMutex

	logger *zap.Logger
	rng    *rand.Rand
	rngMu  sync.Mutex
}

// NewAB Router创建了新的A/B测试路由器.
func NewABRouter(config ABTestConfig, logger *zap.Logger) (*ABRouter, error) {
	if len(config.Variants) < 2 {
		return nil, fmt.Errorf("A/B test requires at least 2 variants")
	}

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

	if logger == nil {
		logger = zap.NewNop()
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

// 选择给定请求的变体。
func (r *ABRouter) selectVariant(ctx context.Context, req *llmpkg.ChatRequest) (*ABVariant, error) {
	// 1. 检查测试是否仍在进行。
	now := time.Now()
	if !r.config.EndTime.IsZero() && now.After(r.config.EndTime) {
		return &r.config.Variants[0], nil
	}

	// 2. 粘接路线。
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

			// 第一次请求此键 - 使用决定散列 。
			variant := r.hashBasedSelect(stickyKey)
			r.stickyCacheMu.Lock()
			r.stickyCache[stickyKey] = variant.Name
			r.stickyCacheMu.Unlock()
			return variant, nil
		}
	}

	// 3. 加权随机选择。
	return r.weightedRandomSelect(), nil
}

func (r *ABRouter) extractStickyKey(req *llmpkg.ChatRequest) string {
	switch r.config.StickyKey {
	case "user_id":
		return req.UserID
	case "session_id":
		return req.TraceID
	case "tenant_id":
		return req.TenantID
	default:
		return req.UserID
	}
}

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

// 完成器件为lmpkg. 供养者.
func (r *ABRouter) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("routing request to variant",
		zap.String("variant", variant.Name),
		zap.String("test", r.config.Name))

	start := time.Now()
	resp, err := variant.Provider.Completion(ctx, req)
	latencyMs := time.Since(start).Milliseconds()

	metrics := r.metrics[variant.Name]
	cost := 0.0
	if resp != nil {
		cost = float64(resp.Usage.TotalTokens) * 0.00001
	}
	metrics.RecordRequest(latencyMs, cost, err == nil, 0)

	if resp != nil {
		resp.Provider = fmt.Sprintf("%s[%s]", resp.Provider, variant.Name)
	}

	return resp, err
}

// 流式设备 ltmpkg. 供养者.
func (r *ABRouter) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("streaming request to variant",
		zap.String("variant", variant.Name))

	return variant.Provider.Stream(ctx, req)
}

// 健康检查设备为lmpkg。 供养者. 所有变体必须健康.
func (r *ABRouter) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	for _, v := range r.config.Variants {
		status, err := v.Provider.HealthCheck(ctx)
		if err != nil || !status.Healthy {
			return &llmpkg.HealthStatus{Healthy: false}, err
		}
	}
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

// 名称工具 llmpkg 。 供养者.
func (r *ABRouter) Name() string {
	return fmt.Sprintf("ab_router[%s]", r.config.Name)
}

// 支持 NativeFunctionCalling 设备 llmpkg 。 供养者.
// 只有当所有变体都支持时, 才会返回真实 。
func (r *ABRouter) SupportsNativeFunctionCalling() bool {
	for _, v := range r.config.Variants {
		if !v.Provider.SupportsNativeFunctionCalling() {
			return false
		}
	}
	return true
}

// ListModels 执行 llmpkg 。 供养者.
// 它将来自所有变体的模型列表和由模型ID来分解.
func (r *ABRouter) ListModels(ctx context.Context) ([]llmpkg.Model, error) {
	modelsByID := make(map[string]llmpkg.Model)
	var lastErr error

	for _, v := range r.config.Variants {
		models, err := v.Provider.ListModels(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		for _, model := range models {
			if model.ID == "" {
				continue
			}
			modelsByID[model.ID] = model
		}
	}

	if len(modelsByID) == 0 {
		return nil, lastErr
	}

	keys := make([]string, 0, len(modelsByID))
	for id := range modelsByID {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	result := make([]llmpkg.Model, 0, len(keys))
	for _, id := range keys {
		result = append(result, modelsByID[id])
	}

	return result, nil
}

// 更新Weights动态地调整了变位权重. 重量必须等于100。
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

	// 清除粘稠的缓存 这样新的重量生效。
	if r.config.StickyRouting {
		r.stickyCacheMu.Lock()
		r.stickyCache = make(map[string]string)
		r.stickyCacheMu.Unlock()
	}

	r.logger.Info("A/B test weights updated",
		zap.String("test", r.config.Name),
		zap.Any("weights", weights))

	return nil
}

// GetMetrics 返回每个变体的指标.
func (r *ABRouter) GetMetrics() map[string]*ABMetrics {
	return r.metrics
}

// GetReport 返回所有变体的摘要报告 。
func (r *ABRouter) GetReport() map[string]map[string]any {
	report := make(map[string]map[string]any)
	for name, m := range r.metrics {
		report[name] = map[string]any{
			"total_requests":    atomic.LoadInt64(&m.TotalRequests),
			"success_rate":      m.GetSuccessRate(),
			"avg_latency_ms":    m.GetAvgLatencyMs(),
			"avg_quality_score": m.GetAvgQualityScore(),
			"total_cost":        m.TotalCost,
		}
	}
	return report
}
