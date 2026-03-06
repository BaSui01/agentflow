package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MultiProviderRouter 多提供商路由器（支持同模型多提供商）
type MultiProviderRouter struct {
	*Router // 继承原有路由器

	apiKeyPools     map[uint]*APIKeyPool // providerID -> APIKeyPool
	providerFactory ProviderFactory      // Provider 工厂
}

type multiProviderCandidate struct {
	LLMProviderModel
	ProviderCode   string
	ProviderStatus int16
	ModelName      string
}

// Stop 停止后台监控资源。
func (r *MultiProviderRouter) Stop() {
	if r == nil || r.Router == nil {
		return
	}
	if r.healthMonitor != nil {
		r.healthMonitor.Stop()
	}
	if r.canaryConfig != nil {
		r.canaryConfig.Stop()
	}
}

// NewMultiProviderRouter 创建多提供商路由器
func NewMultiProviderRouter(db *gorm.DB, providerFactory ProviderFactory, opts RouterOptions) *MultiProviderRouter {
	// 注意：这里不传入 providers map，因为会动态创建
	baseRouter := NewRouter(db, make(map[string]Provider), opts)

	return &MultiProviderRouter{
		Router:          baseRouter,
		apiKeyPools:     make(map[uint]*APIKeyPool),
		providerFactory: providerFactory,
	}
}

// InitAPIKeyPools 初始化 API Key 池
func (r *MultiProviderRouter) InitAPIKeyPools(ctx context.Context) error {
	// 查询所有启用的提供商
	var providers []LLMProvider
	err := r.db.WithContext(ctx).
		Where("status = ?", LLMProviderStatusActive).
		Find(&providers).Error

	if err != nil {
		return err
	}

	// 为每个提供商创建 API Key 池
	for _, provider := range providers {
		pool, err := NewAPIKeyPool(r.db, provider.ID, StrategyWeightedRandom, r.logger)
		if err != nil {
			r.logger.Error("failed to create API key pool",
				zap.Uint("provider_id", provider.ID),
				zap.String("provider_code", provider.Code),
				zap.Error(err))
			continue
		}
		if err := pool.LoadKeys(ctx); err != nil {
			r.logger.Error("failed to load API keys",
				zap.Uint("provider_id", provider.ID),
				zap.String("provider_code", provider.Code),
				zap.Error(err))
			continue
		}
		r.apiKeyPools[provider.ID] = pool
	}

	r.logger.Info("API key pools initialized", zap.Int("count", len(r.apiKeyPools)))
	return nil
}

// GetAPIKeyPool 获取指定提供商的 API Key 池
func (r *MultiProviderRouter) GetAPIKeyPool(providerID uint) *APIKeyPool {
	return r.apiKeyPools[providerID]
}

// SelectProviderWithModel 根据模型名选择最佳提供商（支持多对多）
func (r *MultiProviderRouter) SelectProviderWithModel(ctx context.Context, modelName string, strategy RoutingStrategy) (*ProviderSelection, error) {
	candidates, err := r.queryCandidates(ctx, modelName, "")
	if err != nil {
		return nil, &Error{Code: "BUSINESS_LLM_ROUTING_FAILED", Message: "Failed to query provider models"}
	}

	if len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_MODEL_NOT_FOUND", Message: fmt.Sprintf("Model %s not found", modelName)}
	}

	return r.selectByStrategy(ctx, candidates, strategy)
}

// SelectProviderByCodeWithModel selects a provider by explicit provider code and model.
func (r *MultiProviderRouter) SelectProviderByCodeWithModel(ctx context.Context, providerCode, modelName string, strategy RoutingStrategy) (*ProviderSelection, error) {
	candidates, err := r.queryCandidates(ctx, modelName, providerCode)
	if err != nil {
		return nil, &Error{Code: "BUSINESS_LLM_ROUTING_FAILED", Message: "Failed to query provider models"}
	}
	if len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_MODEL_NOT_FOUND", Message: fmt.Sprintf("Model %s not found for provider %s", modelName, providerCode)}
	}
	return r.selectByStrategy(ctx, candidates, strategy)
}

func (r *MultiProviderRouter) queryCandidates(ctx context.Context, modelName, providerCode string) ([]multiProviderCandidate, error) {
	var candidates []multiProviderCandidate
	query := r.db.WithContext(ctx).Table("sc_llm_provider_models").
		Select("sc_llm_provider_models.*, p.code as provider_code, p.status as provider_status, m.model_name").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_provider_models.provider_id").
		Joins("JOIN sc_llm_models m ON m.id = sc_llm_provider_models.model_id").
		Where("m.model_name = ? AND sc_llm_provider_models.enabled = TRUE AND p.status = ?",
			modelName, LLMProviderStatusActive)
	if providerCode != "" {
		query = query.Where("p.code = ?", providerCode)
	}
	if err := query.Limit(100).Find(&candidates).Error; err != nil {
		return nil, err
	}
	return candidates, nil
}

func (r *MultiProviderRouter) selectByStrategy(ctx context.Context, candidates []multiProviderCandidate, strategy RoutingStrategy) (*ProviderSelection, error) {
	switch strategy {
	case StrategyCostBased:
		return r.selectByCostMulti(ctx, candidates)
	case StrategyHealthBased:
		return r.selectByHealthMulti(ctx, candidates)
	case StrategyLatencyBased:
		return r.selectByLatencyMulti(ctx, candidates)
	case StrategyQPSBased, StrategyTagBased, StrategyCanary:
		return r.selectByQPSMulti(ctx, candidates)
	default:
		return nil, &Error{Code: "BUSINESS_LLM_INVALID_STRATEGY", Message: fmt.Sprintf("Unsupported routing strategy: %s", strategy)}
	}
}

// selectByCostMulti 成本优先选择（多提供商）
func (r *MultiProviderRouter) selectByCostMulti(ctx context.Context, candidates []multiProviderCandidate) (*ProviderSelection, error) {
	// 过滤不健康的提供商
	var healthyCandidates []multiProviderCandidate

	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			healthyCandidates = append(healthyCandidates, c)
		}
	}

	if len(healthyCandidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All providers are unhealthy"}
	}

	// 按成本排序（价格越低越优先）
	sort.Slice(healthyCandidates, func(i, j int) bool {
		costI := healthyCandidates[i].PriceInput + healthyCandidates[i].PriceCompletion
		costJ := healthyCandidates[j].PriceInput + healthyCandidates[j].PriceCompletion
		if costI != costJ {
			return costI < costJ
		}
		// 成本相同时按优先级排序
		return healthyCandidates[i].Priority < healthyCandidates[j].Priority
	})

	best := healthyCandidates[0]
	return r.buildSelectionMulti(ctx, best.LLMProviderModel, best.ProviderCode, best.ModelName, StrategyCostBased)
}

// selectByHealthMulti 健康优先选择（多提供商）
func (r *MultiProviderRouter) selectByHealthMulti(ctx context.Context, candidates []multiProviderCandidate) (*ProviderSelection, error) {
	type candidateWithScore struct {
		multiProviderCandidate
		HealthScore float64
	}

	candidatesWithScore := make([]candidateWithScore, 0, len(candidates))
	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			candidatesWithScore = append(candidatesWithScore, candidateWithScore{
				multiProviderCandidate: c,
				HealthScore:            score,
			})
		}
	}

	if len(candidatesWithScore) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All providers are unhealthy"}
	}

	// 按健康分数排序（高到低）
	sort.Slice(candidatesWithScore, func(i, j int) bool {
		if candidatesWithScore[i].HealthScore != candidatesWithScore[j].HealthScore {
			return candidatesWithScore[i].HealthScore > candidatesWithScore[j].HealthScore
		}
		// 健康分数相同时按优先级排序
		return candidatesWithScore[i].Priority < candidatesWithScore[j].Priority
	})

	best := candidatesWithScore[0]
	return r.buildSelectionMulti(ctx, best.LLMProviderModel, best.ProviderCode, best.ModelName, StrategyHealthBased)
}

// selectByLatencyMulti picks provider with the lowest recent latency.
func (r *MultiProviderRouter) selectByLatencyMulti(ctx context.Context, candidates []multiProviderCandidate) (*ProviderSelection, error) {
	var healthyCandidates []multiProviderCandidate
	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			healthyCandidates = append(healthyCandidates, c)
		}
	}
	if len(healthyCandidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All providers are unhealthy"}
	}

	best := healthyCandidates[0]
	bestLatency := r.recentProviderLatencyMS(ctx, best.ProviderCode)
	for i := 1; i < len(healthyCandidates); i++ {
		current := healthyCandidates[i]
		latency := r.recentProviderLatencyMS(ctx, current.ProviderCode)
		if latency < bestLatency || (latency == bestLatency && current.Priority < best.Priority) {
			best = current
			bestLatency = latency
		}
	}
	return r.buildSelectionMulti(ctx, best.LLMProviderModel, best.ProviderCode, best.ModelName, StrategyLatencyBased)
}

func (r *MultiProviderRouter) recentProviderLatencyMS(ctx context.Context, providerCode string) float64 {
	var result struct {
		AvgLatency float64 `gorm:"column:avg_latency"`
	}
	err := r.db.WithContext(ctx).
		Table("sc_llm_usage_logs").
		Select("AVG(latency_ms) as avg_latency").
		Where("provider = ? AND created_at >= ?", providerCode, time.Now().Add(-5*time.Minute)).
		Scan(&result).Error
	if err != nil || result.AvgLatency <= 0 {
		return math.MaxFloat64
	}
	return result.AvgLatency
}

// selectByQPSMulti QPS 负载均衡选择（多提供商）
func (r *MultiProviderRouter) selectByQPSMulti(ctx context.Context, candidates []multiProviderCandidate) (*ProviderSelection, error) {
	// 过滤不健康的提供商
	var healthyCandidates []multiProviderCandidate

	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			healthyCandidates = append(healthyCandidates, c)
		}
	}

	if len(healthyCandidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All providers are unhealthy"}
	}

	// 选择当前 QPS 最低的提供商
	minQPS := int(^uint(0) >> 1)
	var bestCandidate *multiProviderCandidate

	for i := range healthyCandidates {
		c := &healthyCandidates[i]
		currentQPS := r.healthMonitor.GetCurrentQPS(c.ProviderCode)
		if currentQPS < minQPS {
			minQPS = currentQPS
			bestCandidate = c
		} else if currentQPS == minQPS {
			// QPS 相同时按优先级选择
			if bestCandidate == nil || c.Priority < bestCandidate.Priority {
				bestCandidate = c
			}
		}
	}

	if bestCandidate == nil {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "Failed to select provider by QPS"}
	}

	// 增加 QPS 计数
	r.healthMonitor.IncrementQPS(bestCandidate.ProviderCode)

	return r.buildSelectionMulti(ctx, bestCandidate.LLMProviderModel, bestCandidate.ProviderCode, bestCandidate.ModelName, StrategyQPSBased)
}

// buildSelectionMulti 构建 ProviderSelection（多提供商版本）
func (r *MultiProviderRouter) buildSelectionMulti(
	ctx context.Context,
	providerModel LLMProviderModel,
	providerCode string,
	modelName string,
	strategy RoutingStrategy,
) (*ProviderSelection, error) {
	// 从 API Key 池选择一个可用的 Key
	apiKey, err := r.SelectAPIKey(ctx, providerModel.ProviderID)
	if err != nil {
		return nil, &Error{
			Code:    "BUSINESS_LLM_API_KEY_UNAVAILABLE",
			Message: fmt.Sprintf("No available API key for provider %s: %v", providerCode, err),
		}
	}

	// 使用工厂创建 Provider 实例（带 API Key 和 BaseURL）
	// BaseURL 优先级：APIKey.BaseURL > ProviderModel.BaseURL
	baseURL := providerModel.BaseURL
	if apiKey.BaseURL != "" {
		baseURL = apiKey.BaseURL
	}
	provider, err := r.providerFactory.CreateProvider(providerCode, apiKey.APIKey, baseURL)
	if err != nil {
		return nil, &Error{
			Code:    "BUSINESS_LLM_PROVIDER_UNAVAILABLE",
			Message: fmt.Sprintf("Failed to create provider %s: %v", providerCode, err),
		}
	}

	// 记录 QPS
	r.healthMonitor.IncrementQPS(providerCode)

	return &ProviderSelection{
		Provider:     provider,
		ProviderID:   providerModel.ProviderID,
		APIKeyID:     apiKey.ID,
		ProviderCode: providerCode,
		ModelID:      providerModel.ModelID,
		ModelName:    modelName,
		RemoteModel:  providerModel.RemoteModelName,
		IsCanary:     false,
		Strategy:     strategy,
	}, nil
}

// SelectAPIKey 为指定提供商选择 API Key
func (r *MultiProviderRouter) SelectAPIKey(ctx context.Context, providerID uint) (*LLMProviderAPIKey, error) {
	pool, exists := r.apiKeyPools[providerID]
	if !exists {
		return nil, &Error{
			Code:    "BUSINESS_LLM_PROVIDER_UNAVAILABLE",
			Message: fmt.Sprintf("API key pool not found for provider %d", providerID),
		}
	}

	key, err := pool.SelectKey(ctx)
	if err != nil {
		return nil, &Error{
			Code:    "BUSINESS_LLM_API_KEY_UNAVAILABLE",
			Message: fmt.Sprintf("Failed to select API key: %v", err),
		}
	}

	return key, nil
}

// RecordAPIKeyUsage 记录 API Key 使用情况
func (r *MultiProviderRouter) RecordAPIKeyUsage(ctx context.Context, providerID uint, keyID uint, success bool, errMsg string) error {
	pool, exists := r.apiKeyPools[providerID]
	if !exists {
		return fmt.Errorf("API key pool not found for provider %d", providerID)
	}

	if success {
		return pool.RecordSuccess(ctx, keyID)
	}
	return pool.RecordFailure(ctx, keyID, errMsg)
}

// GetAPIKeyStats 获取所有 API Key 统计信息
func (r *MultiProviderRouter) GetAPIKeyStats() map[uint]map[uint]*APIKeyStats {
	stats := make(map[uint]map[uint]*APIKeyStats)

	for providerID, pool := range r.apiKeyPools {
		stats[providerID] = pool.GetStats()
	}

	return stats
}
