package llm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrNoAvailableAPIKey  = errors.New("no available API key")
	ErrAllKeysRateLimited = errors.New("all API keys are rate limited")
)

// APIKeySelectionStrategy API Key 选择策略
type APIKeySelectionStrategy string

const (
	StrategyRoundRobin     APIKeySelectionStrategy = "round_robin"     // 轮询
	StrategyWeightedRandom APIKeySelectionStrategy = "weighted_random" // 加权随机
	StrategyPriority       APIKeySelectionStrategy = "priority"        // 优先级
	StrategyLeastUsed      APIKeySelectionStrategy = "least_used"      // 最少使用
)

// APIKeyPool API Key 池管理器
type APIKeyPool struct {
	mu            sync.RWMutex
	db            *gorm.DB
	providerID    uint
	keys          []*LLMProviderAPIKey
	strategy      APIKeySelectionStrategy
	roundRobinIdx int
	logger        *zap.Logger
	rng           *rand.Rand
}

// NewAPIKeyPool 创建 API Key 池
func NewAPIKeyPool(db *gorm.DB, providerID uint, strategy APIKeySelectionStrategy, logger *zap.Logger) *APIKeyPool {
	if logger == nil {
		logger = zap.NewNop()
	}
	if strategy == "" {
		strategy = StrategyWeightedRandom
	}

	pool := &APIKeyPool{
		db:         db,
		providerID: providerID,
		strategy:   strategy,
		logger:     logger,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	return pool
}

// LoadKeys 从数据库加载 API Keys
func (p *APIKeyPool) LoadKeys(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var keys []*LLMProviderAPIKey
	err := p.db.WithContext(ctx).
		Where("provider_id = ? AND enabled = TRUE", p.providerID).
		Order("priority ASC, weight DESC").
		Find(&keys).Error

	if err != nil {
		return fmt.Errorf("load API keys from database: %w", err)
	}

	p.keys = keys
	p.logger.Info("API keys loaded",
		zap.Uint("provider_id", p.providerID),
		zap.Int("count", len(keys)))

	return nil
}

// SelectKey 选择一个可用的 API Key
func (p *APIKeyPool) SelectKey(ctx context.Context) (*LLMProviderAPIKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.keys) == 0 {
		return nil, ErrNoAvailableAPIKey
	}

	// 过滤健康的 Keys
	healthyKeys := make([]*LLMProviderAPIKey, 0, len(p.keys))
	for _, key := range p.keys {
		if key.IsHealthy() {
			healthyKeys = append(healthyKeys, key)
		}
	}

	if len(healthyKeys) == 0 {
		return nil, ErrAllKeysRateLimited
	}

	// 根据策略选择
	var selected *LLMProviderAPIKey
	switch p.strategy {
	case StrategyRoundRobin:
		selected = p.selectRoundRobin(healthyKeys)
	case StrategyWeightedRandom:
		selected = p.selectWeightedRandom(healthyKeys)
	case StrategyPriority:
		selected = p.selectPriority(healthyKeys)
	case StrategyLeastUsed:
		selected = p.selectLeastUsed(healthyKeys)
	default:
		selected = p.selectWeightedRandom(healthyKeys)
	}

	if selected == nil {
		return nil, ErrNoAvailableAPIKey
	}

	return selected, nil
}

// selectRoundRobin 轮询选择
func (p *APIKeyPool) selectRoundRobin(keys []*LLMProviderAPIKey) *LLMProviderAPIKey {
	if len(keys) == 0 {
		return nil
	}

	selected := keys[p.roundRobinIdx%len(keys)]
	p.roundRobinIdx++
	return selected
}

// selectWeightedRandom 加权随机选择
func (p *APIKeyPool) selectWeightedRandom(keys []*LLMProviderAPIKey) *LLMProviderAPIKey {
	if len(keys) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := 0
	for _, key := range keys {
		totalWeight += key.Weight
	}

	if totalWeight == 0 {
		return keys[0]
	}

	// 随机选择
	target := p.rng.Intn(totalWeight)
	cumulative := 0

	for _, key := range keys {
		cumulative += key.Weight
		if cumulative > target {
			return key
		}
	}

	return keys[0]
}

// selectPriority 优先级选择（选择优先级最高的）
func (p *APIKeyPool) selectPriority(keys []*LLMProviderAPIKey) *LLMProviderAPIKey {
	if len(keys) == 0 {
		return nil
	}

	// 已经按 priority ASC 排序，直接返回第一个
	return keys[0]
}

// selectLeastUsed 最少使用选择
func (p *APIKeyPool) selectLeastUsed(keys []*LLMProviderAPIKey) *LLMProviderAPIKey {
	if len(keys) == 0 {
		return nil
	}

	// 复制切片以避免修改原始顺序
	keysCopy := make([]*LLMProviderAPIKey, len(keys))
	copy(keysCopy, keys)

	// 按使用次数排序
	sort.Slice(keysCopy, func(i, j int) bool {
		return keysCopy[i].TotalRequests < keysCopy[j].TotalRequests
	})

	return keysCopy[0]
}

// RecordSuccess 记录成功使用
func (p *APIKeyPool) RecordSuccess(ctx context.Context, keyID uint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, key := range p.keys {
		if key.ID == keyID {
			key.IncrementUsage(true)

			// 复制需要的字段值，避免数据竞争
			snapshot := struct {
				ID            uint
				TotalRequests int64
				LastUsedAt    *time.Time
				CurrentRPM    int
				CurrentRPD    int
				RPMResetAt    time.Time
				RPDResetAt    time.Time
			}{
				ID:            key.ID,
				TotalRequests: key.TotalRequests,
				LastUsedAt:    key.LastUsedAt,
				CurrentRPM:    key.CurrentRPM,
				CurrentRPD:    key.CurrentRPD,
				RPMResetAt:    key.RPMResetAt,
				RPDResetAt:    key.RPDResetAt,
			}

			// 异步更新数据库（带 panic 恢复）
			go func(s struct {
				ID            uint
				TotalRequests int64
				LastUsedAt    *time.Time
				CurrentRPM    int
				CurrentRPD    int
				RPMResetAt    time.Time
				RPDResetAt    time.Time
			}) {
				defer func() {
					if r := recover(); r != nil {
						p.logger.Error("panic in async API key update",
							zap.Uint("key_id", s.ID),
							zap.Any("panic", r))
					}
				}()

				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err := p.db.WithContext(updateCtx).Model(&LLMProviderAPIKey{}).
					Where("id = ?", s.ID).
					Updates(map[string]any{
						"total_requests": s.TotalRequests,
						"last_used_at":   s.LastUsedAt,
						"current_rpm":    s.CurrentRPM,
						"current_rpd":    s.CurrentRPD,
						"rpm_reset_at":   s.RPMResetAt,
						"rpd_reset_at":   s.RPDResetAt,
					}).Error

				if err != nil {
					p.logger.Error("failed to update API key usage",
						zap.Uint("key_id", s.ID),
						zap.Error(err))
				}
			}(snapshot)

			return nil
		}
	}

	return errors.New("API key not found")
}

// RecordFailure 记录失败使用
func (p *APIKeyPool) RecordFailure(ctx context.Context, keyID uint, errMsg string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, key := range p.keys {
		if key.ID == keyID {
			key.IncrementUsage(false)
			key.LastError = errMsg

			// 复制需要的字段值，避免数据竞争
			snapshot := struct {
				ID             uint
				TotalRequests  int64
				FailedRequests int64
				LastUsedAt     *time.Time
				LastErrorAt    *time.Time
				LastError      string
				CurrentRPM     int
				CurrentRPD     int
				RPMResetAt     time.Time
				RPDResetAt     time.Time
			}{
				ID:             key.ID,
				TotalRequests:  key.TotalRequests,
				FailedRequests: key.FailedRequests,
				LastUsedAt:     key.LastUsedAt,
				LastErrorAt:    key.LastErrorAt,
				LastError:      key.LastError,
				CurrentRPM:     key.CurrentRPM,
				CurrentRPD:     key.CurrentRPD,
				RPMResetAt:     key.RPMResetAt,
				RPDResetAt:     key.RPDResetAt,
			}

			// 异步更新数据库（带 panic 恢复）
			go func(s struct {
				ID             uint
				TotalRequests  int64
				FailedRequests int64
				LastUsedAt     *time.Time
				LastErrorAt    *time.Time
				LastError      string
				CurrentRPM     int
				CurrentRPD     int
				RPMResetAt     time.Time
				RPDResetAt     time.Time
			}) {
				defer func() {
					if r := recover(); r != nil {
						p.logger.Error("panic in async API key failure update",
							zap.Uint("key_id", s.ID),
							zap.Any("panic", r))
					}
				}()

				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err := p.db.WithContext(updateCtx).Model(&LLMProviderAPIKey{}).
					Where("id = ?", s.ID).
					Updates(map[string]any{
						"total_requests":  s.TotalRequests,
						"failed_requests": s.FailedRequests,
						"last_used_at":    s.LastUsedAt,
						"last_error_at":   s.LastErrorAt,
						"last_error":      s.LastError,
						"current_rpm":     s.CurrentRPM,
						"current_rpd":     s.CurrentRPD,
						"rpm_reset_at":    s.RPMResetAt,
						"rpd_reset_at":    s.RPDResetAt,
					}).Error

				if err != nil {
					p.logger.Error("failed to update API key failure",
						zap.Uint("key_id", s.ID),
						zap.Error(err))
				}
			}(snapshot)

			return nil
		}
	}

	return errors.New("API key not found")
}

// GetStats 获取统计信息
func (p *APIKeyPool) GetStats() map[uint]*APIKeyStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[uint]*APIKeyStats)
	for _, key := range p.keys {
		stats[key.ID] = &APIKeyStats{
			KeyID:          key.ID,
			Label:          key.Label,
			BaseURL:        key.BaseURL,
			Enabled:        key.Enabled,
			IsHealthy:      key.IsHealthy(),
			TotalRequests:  key.TotalRequests,
			FailedRequests: key.FailedRequests,
			SuccessRate:    p.calculateSuccessRate(key),
			CurrentRPM:     key.CurrentRPM,
			CurrentRPD:     key.CurrentRPD,
			LastUsedAt:     key.LastUsedAt,
			LastErrorAt:    key.LastErrorAt,
			LastError:      key.LastError,
		}
	}

	return stats
}

func (p *APIKeyPool) calculateSuccessRate(key *LLMProviderAPIKey) float64 {
	if key.TotalRequests == 0 {
		return 1.0
	}
	successRequests := key.TotalRequests - key.FailedRequests
	return float64(successRequests) / float64(key.TotalRequests)
}

// APIKeyStats API Key 统计信息
type APIKeyStats struct {
	KeyID          uint       `json:"key_id"`
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	Enabled        bool       `json:"enabled"`
	IsHealthy      bool       `json:"is_healthy"`
	TotalRequests  int64      `json:"total_requests"`
	FailedRequests int64      `json:"failed_requests"`
	SuccessRate    float64    `json:"success_rate"`
	CurrentRPM     int        `json:"current_rpm"`
	CurrentRPD     int        `json:"current_rpd"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	LastError      string     `json:"last_error"`
}
