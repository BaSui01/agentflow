package llm

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MultiProviderRouter 多提供商路由器（支持同模型多提供商）
type MultiProviderRouter struct {
	*Router // 继承原有路由器

	apiKeyPools     map[uint]*APIKeyPool // providerID -> APIKeyPool
	providerFactory ProviderFactory      // Provider 工厂
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
		pool := NewAPIKeyPool(r.db, provider.ID, StrategyWeightedRandom, r.logger)
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
	// 1. 查询所有提供该模型的提供商实例
	var candidates []struct {
		LLMProviderModel
		ProviderCode   string
		ProviderStatus int16
		ModelName      string
	}

	query := r.db.WithContext(ctx).Table("sc_llm_provider_models").
		Select("sc_llm_provider_models.*, p.code as provider_code, p.status as provider_status, m.model_name").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_provider_models.provider_id").
		Joins("JOIN sc_llm_models m ON m.id = sc_llm_provider_models.model_id").
		Where("m.model_name = ? AND sc_llm_provider_models.enabled = TRUE AND p.status = ?",
			modelName, LLMProviderStatusActive)

	if err := query.Find(&candidates).Error; err != nil {
		return nil, &Error{Code: "BUSINESS_LLM_ROUTING_FAILED", Message: "Failed to query provider models"}
	}

	if len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_MODEL_NOT_FOUND", Message: fmt.Sprintf("Model %s not found", modelName)}
	}

	// 2. 根据策略选择最佳提供商
	switch strategy {
	case StrategyCostBased:
		return r.selectByCostMulti(ctx, candidates)
	case StrategyHealthBased:
		return r.selectByHealthMulti(ctx, candidates)
	case StrategyQPSBased:
		return r.selectByQPSMulti(ctx, candidates)
	default:
		return r.selectByHealthMulti(ctx, candidates)
	}
}

// selectByCostMulti 成本优先选择（多提供商）
func (r *MultiProviderRouter) selectByCostMulti(_ context.Context, candidates []struct {
	LLMProviderModel
	ProviderCode   string
	ProviderStatus int16
	ModelName      string
}) (*ProviderSelection, error) {
	// 过滤不健康的提供商
	var healthyCandidates []struct {
		LLMProviderModel
		ProviderCode   string
		ProviderStatus int16
		ModelName      string
	}

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
	return r.buildSelectionMulti(best.LLMProviderModel, best.ProviderCode, best.ModelName, StrategyCostBased)
}

// selectByHealthMulti 健康优先选择（多提供商）
func (r *MultiProviderRouter) selectByHealthMulti(_ context.Context, candidates []struct {
	LLMProviderModel
	ProviderCode   string
	ProviderStatus int16
	ModelName      string
}) (*ProviderSelection, error) {
	type candidateWithScore struct {
		LLMProviderModel
		ProviderCode   string
		ProviderStatus int16
		ModelName      string
		HealthScore    float64
	}

	candidatesWithScore := make([]candidateWithScore, 0, len(candidates))
	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			candidatesWithScore = append(candidatesWithScore, candidateWithScore{
				LLMProviderModel: c.LLMProviderModel,
				ProviderCode:     c.ProviderCode,
				ProviderStatus:   c.ProviderStatus,
				ModelName:        c.ModelName,
				HealthScore:      score,
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
	return r.buildSelectionMulti(best.LLMProviderModel, best.ProviderCode, best.ModelName, StrategyHealthBased)
}

// selectByQPSMulti QPS 负载均衡选择（多提供商）
func (r *MultiProviderRouter) selectByQPSMulti(_ context.Context, candidates []struct {
	LLMProviderModel
	ProviderCode   string
	ProviderStatus int16
	ModelName      string
}) (*ProviderSelection, error) {
	// 过滤不健康的提供商
	var healthyCandidates []struct {
		LLMProviderModel
		ProviderCode   string
		ProviderStatus int16
		ModelName      string
	}

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
	var bestCandidate *struct {
		LLMProviderModel
		ProviderCode   string
		ProviderStatus int16
		ModelName      string
	}

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

	return r.buildSelectionMulti(bestCandidate.LLMProviderModel, bestCandidate.ProviderCode, bestCandidate.ModelName, StrategyQPSBased)
}

// buildSelectionMulti 构建 ProviderSelection（多提供商版本）
func (r *MultiProviderRouter) buildSelectionMulti(
	providerModel LLMProviderModel,
	providerCode string,
	modelName string,
	strategy RoutingStrategy,
) (*ProviderSelection, error) {
	// 从 API Key 池选择一个可用的 Key
	apiKey, err := r.SelectAPIKey(context.Background(), providerModel.ProviderID)
	if err != nil {
		return nil, &Error{
			Code:    "BUSINESS_LLM_API_KEY_UNAVAILABLE",
			Message: fmt.Sprintf("No available API key for provider %s: %v", providerCode, err),
		}
	}

	// 使用工厂创建 Provider 实例（带 API Key 和 BaseURL）
	baseURL := providerModel.BaseURL
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
		ProviderCode: providerCode,
		ModelID:      providerModel.ModelID,
		ModelName:    modelName,
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
