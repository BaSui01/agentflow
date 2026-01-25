package router

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/config"

	"go.uber.org/zap"
)

var (
	ErrNoAvailableModel = errors.New("no available model")
	ErrBudgetExceeded   = errors.New("budget exceeded")
)

// ModelRouter 模型路由器接口
type ModelRouter interface {
	// Select 选择最佳模型
	Select(ctx context.Context, req *RouteRequest) (*RouteResult, error)
	// UpdateHealth 更新模型健康状态
	UpdateHealth(modelID string, health *ModelHealth)
	// UpdateWeights 更新路由权重
	UpdateWeights(weights []config.RoutingWeight)
}

// RouteRequest 路由请求
type RouteRequest struct {
	TaskType     string   // 任务类型：chat/completion/embedding
	TenantID     string   // 租户 ID
	Tags         []string // 期望的标签：jsonify/cheap/fast
	MaxCost      float64  // 最大成本预算
	MaxLatencyMs int      // 最大延迟要求
	PreferModel  string   // 优先模型（可选）
}

// RouteResult 路由结果
type RouteResult struct {
	ProviderCode string
	ModelName    string
	ModelID      string
	Score        float64
	Reason       string
}

// ModelHealth 模型健康状态
type ModelHealth struct {
	ModelID      string
	IsHealthy    bool
	SuccessRate  float64 // 成功率 (0-1)
	AvgLatencyMs int     // 平均延迟
	LastError    string
	LastErrorAt  *time.Time
	UpdatedAt    time.Time
}

// ModelCandidate 候选模型
type ModelCandidate struct {
	ProviderCode   string
	ModelID        string
	ModelName      string
	Tags           []string
	PriceInput     float64
	PriceOutput    float64
	MaxTokens      int
	Weight         int
	CostWeight     float64
	LatencyWeight  float64
	QualityWeight  float64
	MaxCostPerReq  float64 // SLA: 单次请求最大成本
	MaxLatencyMs   int     // SLA: 最大延迟（毫秒）
	MinSuccessRate float64 // SLA: 最小成功率
	Health         *ModelHealth
	Enabled        bool
}

// WeightedRouter 加权路由器实现
type WeightedRouter struct {
	mu           sync.RWMutex
	candidates   map[string]*ModelCandidate        // key: modelID
	weights      map[string][]config.RoutingWeight // key: taskType
	health       map[string]*ModelHealth           // key: modelID
	prefixRouter *PrefixRouter                     // 前缀路由器（快速路径）
	logger       *zap.Logger
	rngMu        sync.Mutex // 保护 rng 的并发访问
	rng          *rand.Rand
}

// NewWeightedRouter 创建加权路由器
func NewWeightedRouter(logger *zap.Logger, prefixRules []config.PrefixRule) *WeightedRouter {
	// 转换配置格式
	routerRules := make([]PrefixRule, len(prefixRules))
	for i, r := range prefixRules {
		routerRules[i] = PrefixRule{
			Prefix:   r.Prefix,
			Provider: r.Provider,
		}
	}

	return &WeightedRouter{
		candidates:   make(map[string]*ModelCandidate),
		weights:      make(map[string][]config.RoutingWeight),
		health:       make(map[string]*ModelHealth),
		prefixRouter: NewPrefixRouter(routerRules),
		logger:       logger,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// LoadCandidates 加载候选模型
func (r *WeightedRouter) LoadCandidates(cfg *config.LLMConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.candidates = make(map[string]*ModelCandidate)

	for providerCode, provider := range cfg.Providers {
		if !provider.Enabled {
			continue
		}
		for _, model := range provider.Models {
			if !model.Enabled {
				continue
			}
			r.candidates[model.ID] = &ModelCandidate{
				ProviderCode:   providerCode,
				ModelID:        model.ID,
				ModelName:      model.Name,
				Tags:           model.Tags,
				PriceInput:     model.PriceInput,
				PriceOutput:    model.PriceOutput,
				MaxTokens:      model.MaxTokens,
				Weight:         100, // 默认权重
				CostWeight:     1.0,
				LatencyWeight:  1.0,
				QualityWeight:  1.0,
				MaxCostPerReq:  0, // 从权重配置加载
				MaxLatencyMs:   0, // 从权重配置加载
				MinSuccessRate: 0, // 从权重配置加载
				Enabled:        true,
			}
		}
	}

	// 应用路由权重和 SLA 配置
	for _, weights := range cfg.RoutingWeights {
		for _, w := range weights {
			if c, ok := r.candidates[w.ModelID]; ok {
				c.Weight = w.Weight
				c.CostWeight = w.CostWeight
				c.LatencyWeight = w.LatencyWeight
				c.QualityWeight = w.QualityWeight
				c.MaxCostPerReq = w.MaxCostPerReq
				c.MaxLatencyMs = w.MaxLatencyMs
				c.MinSuccessRate = w.MinSuccessRate
				c.Enabled = w.Enabled
			}
		}
	}

	r.logger.Info("candidates loaded", zap.Int("count", len(r.candidates)))
}

// Select 选择最佳模型
func (r *WeightedRouter) Select(ctx context.Context, req *RouteRequest) (*RouteResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. 快速路径：前缀路由（仅当指定 PreferModel 时）
	if req.PreferModel != "" {
		if providerCode, ok := r.prefixRouter.RouteByModelID(req.PreferModel); ok {
			// 在候选中查找匹配的 Provider
			for _, c := range r.candidates {
				if c.ProviderCode == providerCode && c.Enabled {
					r.logger.Debug("prefix route hit",
						zap.String("model", req.PreferModel),
						zap.String("provider", providerCode))

					return &RouteResult{
						ProviderCode: providerCode,
						ModelName:    c.ModelName,
						ModelID:      c.ModelID,
						Score:        1000,
						Reason:       "prefix_match",
					}, nil
				}
			}
		}
	}

	// 2. 完整路径：加权路由（原有逻辑）
	filtered := r.filterCandidates(req)
	if len(filtered) == 0 {
		return nil, ErrNoAvailableModel
	}

	// 3. 计算得分
	scored := r.scoreCandidates(filtered, req)

	// 4. 加权随机选择
	selected := r.weightedSelect(scored)
	if selected == nil {
		return nil, ErrNoAvailableModel
	}

	return &RouteResult{
		ProviderCode: selected.ProviderCode,
		ModelName:    selected.ModelName,
		ModelID:      selected.ModelID,
		Score:        selected.score,
		Reason:       selected.reason,
	}, nil
}

// filterCandidates 过滤候选模型
func (r *WeightedRouter) filterCandidates(req *RouteRequest) []*ModelCandidate {
	var result []*ModelCandidate

	for _, c := range r.candidates {
		if !c.Enabled {
			continue
		}

		health, hasHealth := r.health[c.ModelID]

		// 检查健康状态
		if hasHealth && !health.IsHealthy {
			continue
		}

		// 检查延迟要求（请求级 + 模型级 SLA）
		if hasHealth && health.AvgLatencyMs > 0 {
			maxLatency := req.MaxLatencyMs
			if c.MaxLatencyMs > 0 && (maxLatency == 0 || c.MaxLatencyMs < maxLatency) {
				maxLatency = c.MaxLatencyMs
			}
			if maxLatency > 0 && health.AvgLatencyMs > maxLatency {
				continue
			}
		}

		// 检查成功率 SLA
		if hasHealth && c.MinSuccessRate > 0 && health.SuccessRate < c.MinSuccessRate {
			continue
		}

		// 检查成本预算（请求级 + 模型级 SLA）
		estimatedCost := (c.PriceInput + c.PriceOutput) * 2 // 简单估算 2K tokens
		maxCost := req.MaxCost
		if c.MaxCostPerReq > 0 && (maxCost == 0 || c.MaxCostPerReq < maxCost) {
			maxCost = c.MaxCostPerReq
		}
		if maxCost > 0 && estimatedCost > maxCost {
			continue
		}

		// 检查标签匹配
		if len(req.Tags) > 0 && !r.matchTags(c.Tags, req.Tags) {
			continue
		}

		result = append(result, c)
	}

	return result
}

// matchTags 检查标签匹配（至少匹配一个）
func (r *WeightedRouter) matchTags(modelTags, reqTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range modelTags {
		tagSet[t] = true
	}
	for _, t := range reqTags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

type scoredCandidate struct {
	*ModelCandidate
	score  float64
	reason string
}

// scoreCandidates 计算候选模型得分
func (r *WeightedRouter) scoreCandidates(candidates []*ModelCandidate, req *RouteRequest) []scoredCandidate {
	result := make([]scoredCandidate, 0, len(candidates))

	for _, c := range candidates {
		score := float64(c.Weight)

		// 成本因子（成本越低分数越高）
		costScore := 1.0 / (1.0 + (c.PriceInput+c.PriceOutput)*100)
		score += costScore * c.CostWeight * 50

		// 延迟因子
		if health, ok := r.health[c.ModelID]; ok && health.AvgLatencyMs > 0 {
			latencyScore := 1.0 / (1.0 + float64(health.AvgLatencyMs)/1000)
			score += latencyScore * c.LatencyWeight * 50

			// 成功率因子
			score += health.SuccessRate * c.QualityWeight * 100
		} else {
			// 无健康数据，给予中等分数
			score += 50
		}

		// 优先模型加分
		if req.PreferModel != "" && c.ModelName == req.PreferModel {
			score += 200
		}

		result = append(result, scoredCandidate{
			ModelCandidate: c,
			score:          score,
			reason:         "weighted_score",
		})
	}

	// 按分数排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].score > result[j].score
	})

	return result
}

// weightedSelect 加权随机选择
func (r *WeightedRouter) weightedSelect(candidates []scoredCandidate) *scoredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// 计算总权重
	var totalWeight float64
	for _, c := range candidates {
		totalWeight += c.score
	}

	// 随机选择（加锁保护 rng）
	r.rngMu.Lock()
	target := r.rng.Float64() * totalWeight
	r.rngMu.Unlock()
	var cumulative float64

	for i := range candidates {
		cumulative += candidates[i].score
		if cumulative >= target {
			return &candidates[i]
		}
	}

	// 兜底返回第一个
	return &candidates[0]
}

// UpdateHealth 更新模型健康状态
func (r *WeightedRouter) UpdateHealth(modelID string, health *ModelHealth) {
	r.mu.Lock()
	defer r.mu.Unlock()

	health.UpdatedAt = time.Now()
	r.health[modelID] = health

	r.logger.Debug("health updated",
		zap.String("model_id", modelID),
		zap.Bool("healthy", health.IsHealthy),
		zap.Float64("success_rate", health.SuccessRate))
}

// UpdateWeights 更新路由权重
func (r *WeightedRouter) UpdateWeights(weights []config.RoutingWeight) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, w := range weights {
		if c, ok := r.candidates[w.ModelID]; ok {
			c.Weight = w.Weight
			c.CostWeight = w.CostWeight
			c.LatencyWeight = w.LatencyWeight
			c.QualityWeight = w.QualityWeight
			c.Enabled = w.Enabled
		}
	}

	r.logger.Info("weights updated", zap.Int("count", len(weights)))
}

// GetCandidates 获取所有候选模型（用于调试）
func (r *WeightedRouter) GetCandidates() map[string]*ModelCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*ModelCandidate)
	for k, v := range r.candidates {
		result[k] = v
	}
	return result
}

// HealthChecker 健康检查器
type HealthChecker struct {
	router    *WeightedRouter
	interval  time.Duration
	timeout   time.Duration
	providers map[string]llmpkg.Provider
	stopCh    chan struct{}
	logger    *zap.Logger
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(router *WeightedRouter, interval time.Duration, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		router:   router,
		interval: interval,
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
}

// NewHealthCheckerWithProviders 创建健康检查器（带 Provider 探活能力）。
func NewHealthCheckerWithProviders(router *WeightedRouter, providers map[string]llmpkg.Provider, interval, timeout time.Duration, logger *zap.Logger) *HealthChecker {
	h := NewHealthChecker(router, interval, logger)
	h.providers = providers
	h.timeout = timeout
	return h
}

// Start 启动健康检查
func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkAll(ctx)
		}
	}
}

// Stop 停止健康检查
func (h *HealthChecker) Stop() {
	close(h.stopCh)
}

func (h *HealthChecker) checkAll(ctx context.Context) {
	if h.router == nil {
		return
	}
	candidates := h.router.GetCandidates()

	// 未注入 providers 时，保守跳过：避免误报“健康”影响路由决策。
	if len(h.providers) == 0 {
		h.logger.Debug("health checker skipped (no providers injected)")
		return
	}

	// 按 Provider 聚合，避免重复探活。
	modelIDsByProvider := make(map[string][]string, len(candidates))
	for modelID, c := range candidates {
		if c == nil || c.ProviderCode == "" {
			continue
		}
		modelIDsByProvider[c.ProviderCode] = append(modelIDsByProvider[c.ProviderCode], modelID)
	}

	for providerCode, modelIDs := range modelIDsByProvider {
		p, ok := h.providers[providerCode]
		if !ok || p == nil {
			for _, modelID := range modelIDs {
				h.router.UpdateHealth(modelID, &ModelHealth{
					ModelID:     modelID,
					IsHealthy:   false,
					SuccessRate: 0,
					LastError:   fmt.Sprintf("provider not found: %s", providerCode),
				})
			}
			continue
		}

		timeout := h.timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		status, err := p.HealthCheck(probeCtx)
		cancel()

		latency := time.Since(start)
		healthy := err == nil
		if status != nil {
			if status.Latency > 0 {
				latency = status.Latency
			}
			healthy = healthy && status.Healthy
		}

		var lastErr string
		var lastErrAt *time.Time
		if err != nil {
			lastErr = err.Error()
			now := time.Now()
			lastErrAt = &now
			h.logger.Warn("llm provider health check failed",
				zap.String("provider", providerCode),
				zap.Duration("latency", latency),
				zap.Error(err),
			)
		}

		successRate := 1.0
		if !healthy {
			successRate = 0
		}

		for _, modelID := range modelIDs {
			h.router.UpdateHealth(modelID, &ModelHealth{
				ModelID:      modelID,
				IsHealthy:    healthy,
				SuccessRate:  successRate,
				AvgLatencyMs: int(latency.Milliseconds()),
				LastError:    lastErr,
				LastErrorAt:  lastErrAt,
			})
		}
	}
}
