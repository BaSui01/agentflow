package config

import (
	"sort"
	"sync"
)

// ErrorCode 错误码类型
type ErrorCode string

// PolicyManager 降级策略管理器
type PolicyManager struct {
	mu       sync.RWMutex
	policies []FallbackPolicy
	// 索引：按 provider+model 快速查找
	byProvider map[string][]FallbackPolicy
	byError    map[string][]FallbackPolicy
}

// NewPolicyManager 创建策略管理器
func NewPolicyManager() *PolicyManager {
	return &PolicyManager{
		byProvider: make(map[string][]FallbackPolicy),
		byError:    make(map[string][]FallbackPolicy),
	}
}

// Update 更新策略列表
func (pm *PolicyManager) Update(policies []FallbackPolicy) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 按优先级排序
	sorted := make([]FallbackPolicy, len(policies))
	copy(sorted, policies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	pm.policies = sorted
	pm.rebuildIndex()
}

// rebuildIndex 重建索引
func (pm *PolicyManager) rebuildIndex() {
	pm.byProvider = make(map[string][]FallbackPolicy)
	pm.byError = make(map[string][]FallbackPolicy)

	for _, p := range pm.policies {
		if !p.Enabled {
			continue
		}
		// 按 provider 索引
		key := p.TriggerProvider
		if key == "" {
			key = "*" // 全局策略
		}
		pm.byProvider[key] = append(pm.byProvider[key], p)

		// 按错误码索引
		for _, errCode := range p.TriggerErrors {
			pm.byError[errCode] = append(pm.byError[errCode], p)
		}
	}
}

// FindPolicy 查找匹配的降级策略
func (pm *PolicyManager) FindPolicy(provider, model string, errCode ErrorCode) *FallbackPolicy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 1. 精确匹配：provider + model + error
	for _, p := range pm.policies {
		if !p.Enabled {
			continue
		}
		if pm.matchPolicy(&p, provider, model, errCode) {
			return &p
		}
	}
	return nil
}

// matchPolicy 检查策略是否匹配
func (pm *PolicyManager) matchPolicy(p *FallbackPolicy, provider, model string, errCode ErrorCode) bool {
	// 检查 provider
	if p.TriggerProvider != "" && p.TriggerProvider != provider {
		return false
	}
	// 检查 model
	if p.TriggerModel != "" && p.TriggerModel != model {
		return false
	}
	// 检查错误码
	if len(p.TriggerErrors) > 0 {
		found := false
		for _, e := range p.TriggerErrors {
			if e == string(errCode) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// GetFallbackChain 获取完整的降级链
func (pm *PolicyManager) GetFallbackChain(provider, model string) []FallbackPolicy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var chain []FallbackPolicy
	for _, p := range pm.policies {
		if !p.Enabled {
			continue
		}
		// 匹配 provider 或全局
		if p.TriggerProvider == "" || p.TriggerProvider == provider {
			// 匹配 model 或全部
			if p.TriggerModel == "" || p.TriggerModel == model {
				chain = append(chain, p)
			}
		}
	}
	return chain
}

// FallbackAction 降级动作
type FallbackAction struct {
	Type     FallbackType
	Target   string // 目标 model 或 provider
	Template string // 模板响应
	Policy   *FallbackPolicy
}

// GetFallbackAction 根据错误获取降级动作
func (pm *PolicyManager) GetFallbackAction(provider, model string, errCode ErrorCode) *FallbackAction {
	policy := pm.FindPolicy(provider, model, errCode)
	if policy == nil {
		return nil
	}
	return &FallbackAction{
		Type:     policy.FallbackType,
		Target:   policy.FallbackTarget,
		Template: policy.FallbackTemplate,
		Policy:   policy,
	}
}

// ShouldRetry 判断是否应该重试
func (pm *PolicyManager) ShouldRetry(provider, model string, errCode ErrorCode, attempt int) bool {
	policy := pm.FindPolicy(provider, model, errCode)
	if policy == nil {
		return false
	}
	return attempt < policy.RetryMax
}

// GetRetryDelay 获取重试延迟（毫秒）
func (pm *PolicyManager) GetRetryDelay(provider, model string, errCode ErrorCode, attempt int) int {
	policy := pm.FindPolicy(provider, model, errCode)
	if policy == nil {
		return 1000
	}
	delay := float64(policy.RetryDelayMs)
	for i := 0; i < attempt; i++ {
		delay *= policy.RetryMultiplier
	}
	return int(delay)
}
