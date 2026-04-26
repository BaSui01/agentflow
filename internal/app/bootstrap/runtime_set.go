package bootstrap

import (
	"github.com/BaSui01/agentflow/llm/cache"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/redis/go-redis/v9"
)

// LLMRuntimeSet aggregates LLM-related runtime dependencies.
type LLMRuntimeSet struct {
	Provider      llm.Provider
	ToolProvider  llm.Provider
	BudgetManager *llmpolicy.TokenBudgetManager
	CostTracker   *observability.CostTracker
	LLMCache      *cache.MultiLevelCache
	LLMMetrics    *observability.Metrics
	Ledger        observability.Ledger

	MultimodalRedis   *redis.Client
	ToolApprovalRedis *redis.Client
}

// IsAvailable returns true if the main LLM provider is configured.
func (s *LLMRuntimeSet) IsAvailable() bool {
	return s.Provider != nil
}
