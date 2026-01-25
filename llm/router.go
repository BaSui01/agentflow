package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RoutingStrategy string

const (
	StrategyTagBased    RoutingStrategy = "tag"
	StrategyCostBased   RoutingStrategy = "cost"
	StrategyQPSBased    RoutingStrategy = "qps"
	StrategyHealthBased RoutingStrategy = "health"
	StrategyCanary      RoutingStrategy = "canary"
)

type Router struct {
	db            *gorm.DB
	providers     map[string]Provider // provider_code -> Provider
	healthMonitor *HealthMonitor
	canaryConfig  *CanaryConfig
	logger        *zap.Logger

	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	healthCheckCancel   context.CancelFunc
}

type RouterOptions struct {
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	Logger              *zap.Logger
}

func normalizeRouterOptions(opts RouterOptions) RouterOptions {
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	if opts.HealthCheckInterval <= 0 {
		opts.HealthCheckInterval = 60 * time.Second
	}
	if opts.HealthCheckTimeout <= 0 {
		opts.HealthCheckTimeout = 10 * time.Second
	}
	return opts
}

type ProviderSelection struct {
	Provider     Provider
	ProviderID   uint
	ProviderCode string
	ModelID      uint
	ModelName    string
	IsCanary     bool
	Strategy     RoutingStrategy
}

func NewRouter(db *gorm.DB, providers map[string]Provider, opts RouterOptions) *Router {
	opts = normalizeRouterOptions(opts)
	r := &Router{
		db:                  db,
		providers:           providers,
		healthMonitor:       NewHealthMonitor(db),
		canaryConfig:        NewCanaryConfig(db),
		logger:              opts.Logger,
		healthCheckInterval: opts.HealthCheckInterval,
		healthCheckTimeout:  opts.HealthCheckTimeout,
	}

	// 周期性探活（默认 60 秒/次，单次 10 秒超时）。探活结果用于路由熔断与指标采集。
	r.startProviderHealthChecks()
	return r
}

func (r *Router) Stop() {
	if r.healthCheckCancel != nil {
		r.healthCheckCancel()
	}
	if r.healthMonitor != nil {
		r.healthMonitor.Stop()
	}
	if r.canaryConfig != nil {
		r.canaryConfig.Stop()
	}
}

func (r *Router) startProviderHealthChecks() {
	if r == nil || len(r.providers) == 0 {
		// providers 为空时跳过健康检查（Provider 从数据库动态管理）
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.healthCheckCancel = cancel

	ticker := time.NewTicker(r.healthCheckInterval)
	go func() {
		defer ticker.Stop()
		// 启动时先跑一次，便于尽快发现不可用 Provider。
		r.probeProviders(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.probeProviders(ctx)
			}
		}
	}()
}

func (r *Router) probeProviders(parent context.Context) {
	for code, p := range r.providers {
		probeCtx, cancel := context.WithTimeout(parent, r.healthCheckTimeout)
		start := time.Now()
		st, err := p.HealthCheck(probeCtx)
		cancel()

		latency := time.Since(start)
		healthy := err == nil
		if st != nil {
			if st.Latency > 0 {
				latency = st.Latency
			}
			healthy = healthy && st.Healthy
		} else {
			healthy = false
			st = &HealthStatus{Healthy: false, Latency: latency}
		}

		if r.healthMonitor != nil {
			r.healthMonitor.UpdateProbe(code, st, err)
		}
		observeProviderHealthCheck(code, healthy, latency, err)

		if err != nil {
			r.logger.Warn("llm provider health check failed",
				zap.String("provider", code),
				zap.Duration("latency", latency),
				zap.Error(err),
			)
		}
	}
}

func (r *Router) GetHealthMonitor() *HealthMonitor {
	return r.healthMonitor
}

func (r *Router) GetCanaryConfig() *CanaryConfig {
	return r.canaryConfig
}

// SelectProvider 根据策略选择最佳 Provider
func (r *Router) SelectProvider(ctx context.Context, req *ChatRequest, strategy RoutingStrategy) (*ProviderSelection, error) {
	// 1. 优先检查金丝雀部署
	canarySelection, isCanary := r.tryCanaryRouting(ctx, req)
	if isCanary {
		return canarySelection, nil
	}

	// 2. 根据策略路由
	switch strategy {
	case StrategyTagBased:
		return r.selectByTags(ctx, req)
	case StrategyCostBased:
		return r.selectByCost(ctx, req)
	case StrategyQPSBased:
		return r.selectByQPS(ctx, req)
	case StrategyHealthBased:
		return r.selectByHealth(ctx, req)
	default:
		return r.selectByHealth(ctx, req) // 默认健康优先
	}
}

// tryCanaryRouting 尝试金丝雀路由（优先级最高）
func (r *Router) tryCanaryRouting(ctx context.Context, req *ChatRequest) (*ProviderSelection, bool) {
	if req.Model == "" {
		return nil, false
	}

	// 查询模型信息
	var model LLMModel
	err := r.db.WithContext(ctx).Table("sc_llm_models").
		Where("model_name = ? AND enabled = TRUE", req.Model).
		First(&model).Error
	if err != nil {
		return nil, false
	}

	// 检查是否有活跃的金丝雀部署
	deployment := r.canaryConfig.GetDeployment(model.ProviderID)
	if deployment == nil || deployment.Stage == CanaryStage100Pct || deployment.Stage == CanaryStageRollback {
		return nil, false
	}

	// 根据流量百分比决定是否使用金丝雀版本
	trafficPercent := deployment.TrafficPercent
	randomValue := rand.Intn(100) // 0-99

	useCanary := randomValue < trafficPercent
	selectedVersion := deployment.StableVersion
	if useCanary {
		selectedVersion = deployment.CanaryVersion
	}

	provider, exists := r.providers[selectedVersion]
	if !exists {
		return nil, false
	}

	// 记录 QPS
	r.healthMonitor.IncrementQPS(selectedVersion)

	return &ProviderSelection{
		Provider:     provider,
		ProviderID:   model.ProviderID,
		ProviderCode: selectedVersion,
		ModelID:      model.ID,
		ModelName:    model.ModelName,
		IsCanary:     useCanary,
		Strategy:     StrategyCanary,
	}, true
}

// selectByTags Tag-Based 路由：根据模型标签匹配
func (r *Router) selectByTags(ctx context.Context, req *ChatRequest) (*ProviderSelection, error) {
	requiredTags := req.Tags
	if len(requiredTags) == 0 {
		return r.selectByHealth(ctx, req) // 无标签时降级为健康优先
	}

	var candidates []struct {
		LLMModel
		ProviderCode   string
		ProviderStatus int16
	}

	query := r.db.WithContext(ctx).Table("sc_llm_models").
		Select("sc_llm_models.*, p.code as provider_code, p.status as provider_status").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_models.provider_id").
		Where("sc_llm_models.enabled = TRUE AND p.status = ?", LLMProviderStatusActive)

	// 使用 PostgreSQL JSONB 查询匹配 tags（AND 逻辑）
	for _, tag := range requiredTags {
		tagJSON, _ := json.Marshal(tag)
		query = query.Where("llm_models.tags @> ?", fmt.Sprintf("[%s]", string(tagJSON)))
	}

	if err := query.Find(&candidates).Error; err != nil {
		return nil, &Error{Code: "BUSINESS_LLM_ROUTING_FAILED", Message: "Failed to query models by tags"}
	}

	if len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "No model found matching required tags"}
	}

	// 过滤不健康的 Provider 并按成本排序
	var healthyCandidates []struct {
		LLMModel
		ProviderCode   string
		ProviderStatus int16
	}
	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 {
			healthyCandidates = append(healthyCandidates, c)
		}
	}

	if len(healthyCandidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All matching providers are unhealthy"}
	}

	// 按成本排序（成本优化）
	sort.Slice(healthyCandidates, func(i, j int) bool {
		costI := healthyCandidates[i].PriceInput + healthyCandidates[i].PriceCompletion
		costJ := healthyCandidates[j].PriceInput + healthyCandidates[j].PriceCompletion
		return costI < costJ
	})

	best := healthyCandidates[0]
	return r.buildSelection(best.LLMModel, best.ProviderCode, StrategyTagBased)
}

// selectByCost Cost-Based 路由：选择最便宜的模型
func (r *Router) selectByCost(ctx context.Context, req *ChatRequest) (*ProviderSelection, error) {
	var candidates []struct {
		LLMModel
		ProviderCode string
	}

	err := r.db.WithContext(ctx).Table("sc_llm_models").
		Select("sc_llm_models.*, p.code as provider_code").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_models.provider_id").
		Where("sc_llm_models.enabled = TRUE AND p.status = ?", LLMProviderStatusActive).
		Order("(price_input + price_completion) ASC").
		Find(&candidates).Error

	if err != nil || len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "No active models available"}
	}

	// 过滤不健康的 Provider
	var healthyCandidates []struct {
		LLMModel
		ProviderCode string
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

	best := healthyCandidates[0]
	return r.buildSelection(best.LLMModel, best.ProviderCode, StrategyCostBased)
}

// selectByQPS QPS-Based 路由：负载均衡
func (r *Router) selectByQPS(ctx context.Context, req *ChatRequest) (*ProviderSelection, error) {
	var candidates []struct {
		LLMModel
		ProviderCode string
		ProviderID   uint
	}

	err := r.db.WithContext(ctx).Table("sc_llm_models").
		Select("sc_llm_models.*, p.code as provider_code, p.id as provider_id").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_models.provider_id").
		Where("sc_llm_models.enabled = TRUE AND p.status = ?", LLMProviderStatusActive).
		Find(&candidates).Error

	if err != nil || len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "No active models available"}
	}

	// 过滤不健康的 Provider
	var healthyCandidates []struct {
		LLMModel
		ProviderCode string
		ProviderID   uint
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

	// 选择当前 QPS 最低的 Provider
	minQPS := int(^uint(0) >> 1) // 最大整数
	var bestCandidate *struct {
		LLMModel
		ProviderCode string
		ProviderID   uint
	}

	for i := range healthyCandidates {
		c := &healthyCandidates[i]
		currentQPS := r.healthMonitor.GetCurrentQPS(c.ProviderCode)
		if currentQPS < minQPS {
			minQPS = currentQPS
			bestCandidate = c
		}
	}

	if bestCandidate == nil {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "Failed to select provider by QPS"}
	}

	// 增加 QPS 计数
	r.healthMonitor.IncrementQPS(bestCandidate.ProviderCode)

	return r.buildSelection(bestCandidate.LLMModel, bestCandidate.ProviderCode, StrategyQPSBased)
}

// selectByHealth Health-Based 路由：健康优先
func (r *Router) selectByHealth(ctx context.Context, req *ChatRequest) (*ProviderSelection, error) {
	var candidates []struct {
		LLMModel
		ProviderCode string
		ProviderID   uint
	}

	err := r.db.WithContext(ctx).Table("sc_llm_models").
		Select("sc_llm_models.*, p.code as provider_code, p.id as provider_id").
		Joins("JOIN sc_llm_providers p ON p.id = sc_llm_models.provider_id").
		Where("sc_llm_models.enabled = TRUE AND p.status = ?", LLMProviderStatusActive).
		Find(&candidates).Error

	if err != nil || len(candidates) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "No active models available"}
	}

	// 注入健康分数
	type candidateWithScore struct {
		LLMModel
		ProviderCode string
		ProviderID   uint
		HealthScore  float64
	}

	candidatesWithScore := make([]candidateWithScore, 0, len(candidates))
	for _, c := range candidates {
		score := r.healthMonitor.GetHealthScore(c.ProviderCode)
		if score >= 0.5 { // 健康阈值
			candidatesWithScore = append(candidatesWithScore, candidateWithScore{
				LLMModel:     c.LLMModel,
				ProviderCode: c.ProviderCode,
				ProviderID:   c.ProviderID,
				HealthScore:  score,
			})
		}
	}

	if len(candidatesWithScore) == 0 {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: "All providers are unhealthy"}
	}

	// 按健康分数排序（高到低）
	sort.Slice(candidatesWithScore, func(i, j int) bool {
		return candidatesWithScore[i].HealthScore > candidatesWithScore[j].HealthScore
	})

	best := candidatesWithScore[0]
	return r.buildSelection(best.LLMModel, best.ProviderCode, StrategyHealthBased)
}

// buildSelection 构建 ProviderSelection
func (r *Router) buildSelection(model LLMModel, providerCode string, strategy RoutingStrategy) (*ProviderSelection, error) {
	provider, exists := r.providers[providerCode]
	if !exists {
		return nil, &Error{Code: "BUSINESS_LLM_PROVIDER_UNAVAILABLE", Message: fmt.Sprintf("Provider %s not found", providerCode)}
	}

	// 记录 QPS
	r.healthMonitor.IncrementQPS(providerCode)

	return &ProviderSelection{
		Provider:     provider,
		ProviderID:   model.ProviderID,
		ProviderCode: providerCode,
		ModelID:      model.ID,
		ModelName:    model.ModelName,
		IsCanary:     false,
		Strategy:     strategy,
	}, nil
}
