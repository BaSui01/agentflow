package router

import (
	"strings"
)

// PrefixRule 前缀路由规则
type PrefixRule struct {
	Prefix   string // 模型 ID 前缀（如 "gpt-4o", "claude-3-5-sonnet"）
	Provider string // Provider 代码（如 "openai", "anthropic"）
}

// PrefixRouter 前缀路由器
// 通过模型 ID 前缀快速路由到指定 Provider
type PrefixRouter struct {
	rules []PrefixRule
}

// NewPrefixRouter 创建前缀路由器
// rules 应按前缀长度降序排列（最长前缀优先）
func NewPrefixRouter(rules []PrefixRule) *PrefixRouter {
	// 按前缀长度降序排序（确保最长前缀优先匹配）
	sortedRules := make([]PrefixRule, len(rules))
	copy(sortedRules, rules)

	// 简单冒泡排序（规则数量通常很小，性能足够）
	for i := 0; i < len(sortedRules)-1; i++ {
		for j := 0; j < len(sortedRules)-i-1; j++ {
			if len(sortedRules[j].Prefix) < len(sortedRules[j+1].Prefix) {
				sortedRules[j], sortedRules[j+1] = sortedRules[j+1], sortedRules[j]
			}
		}
	}

	return &PrefixRouter{
		rules: sortedRules,
	}
}

// RouteByModelID 根据模型 ID 前缀路由
// 返回：providerCode, found
func (r *PrefixRouter) RouteByModelID(modelID string) (string, bool) {
	if r == nil || len(r.rules) == 0 || modelID == "" {
		return "", false
	}

	// 按前缀长度降序匹配（最长前缀优先）
	for _, rule := range r.rules {
		if strings.HasPrefix(modelID, rule.Prefix) {
			return rule.Provider, true
		}
	}

	return "", false
}

// GetRules 获取所有路由规则（用于调试）
func (r *PrefixRouter) GetRules() []PrefixRule {
	return r.rules
}
