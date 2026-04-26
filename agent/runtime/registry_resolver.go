package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// =============================================================================
// Resolver (merged from resolver.go)
// =============================================================================
// CachingResolver resolves agent IDs to live Agent instances, creating them
// on demand via AgentRegistry and caching them for reuse. It uses singleflight
// to ensure concurrent requests for the same agentID only trigger one
// Create+Init cycle.
type CachingResolver struct {
	registry       *AgentRegistry
	gateway        llmcore.Gateway
	memory         MemoryManager // optional; nil means stateless agents
	enhancedMemory EnhancedMemoryRunner
	tools          ToolManager
	logger         *zap.Logger
	agents         sync.Map
	group          singleflight.Group
	toolNames      []string
	modelHint      string

	// MongoDB persistence stores (required)
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider
}

// NewCachingResolver creates a CachingResolver backed by the given registry
// and main LLM gateway.
func NewCachingResolver(registry *AgentRegistry, gateway llmcore.Gateway, logger *zap.Logger) *CachingResolver {
	return &CachingResolver{
		registry: registry,
		gateway:  gateway,
		logger:   logger,
	}
}

// WithMemory sets the MemoryManager used when creating new agent instances.
// When non-nil, agents created by this resolver will have memory capabilities.
func (r *CachingResolver) WithMemory(m MemoryManager) *CachingResolver {
	r.memory = m
	return r
}

// WithEnhancedMemory sets the enhanced memory system used when creating new
// agent instances. When non-nil, resolved BaseAgent instances will use this
// shared enhanced memory runtime instead of a per-agent default instance.
func (r *CachingResolver) WithEnhancedMemory(mem EnhancedMemoryRunner) *CachingResolver {
	r.enhancedMemory = mem
	return r
}

// WithToolManager sets the ToolManager used when creating new agent instances.
// When non-nil, resolved agents can call tools during execution.
func (r *CachingResolver) WithToolManager(m ToolManager) *CachingResolver {
	r.tools = m
	return r
}

// WithRuntimeTools sets a default tool whitelist for resolved agents.
// If empty, the resolver derives tool names from ToolManager.GetAllowedTools(agentID).
func (r *CachingResolver) WithRuntimeTools(toolNames []string) *CachingResolver {
	if len(toolNames) == 0 {
		r.toolNames = nil
		return r
	}
	out := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		r.toolNames = nil
		return r
	}
	r.toolNames = out
	return r
}

// WithDefaultModel sets the default model used for resolved agents.
// Agent request can still override it at runtime via model routing params.
func (r *CachingResolver) WithDefaultModel(model string) *CachingResolver {
	r.modelHint = strings.TrimSpace(model)
	return r
}

// WithPromptStore sets the PromptStoreProvider for resolved agents.
func (r *CachingResolver) WithPromptStore(s PromptStoreProvider) *CachingResolver {
	r.promptStore = s
	return r
}

// WithConversationStore sets the ConversationStoreProvider for resolved agents.
func (r *CachingResolver) WithConversationStore(s ConversationStoreProvider) *CachingResolver {
	r.conversationStore = s
	return r
}

// WithRunStore sets the RunStoreProvider for resolved agents.
func (r *CachingResolver) WithRunStore(s RunStoreProvider) *CachingResolver {
	r.runStore = s
	return r
}

// Resolve returns a cached Agent for agentID, or creates and initialises one.
func (r *CachingResolver) Resolve(ctx context.Context, agentID string) (Agent, error) {
	// Fast path: already cached.
	if cached, ok := r.agents.Load(agentID); ok {
		return cached.(Agent), nil
	}

	// Deduplicate concurrent creation for the same ID.
	result, err, _ := r.group.Do(agentID, func() (any, error) {
		// Double-check after acquiring the flight.
		if cached, ok := r.agents.Load(agentID); ok {
			return cached, nil
		}

		cfg := types.AgentConfig{
			Core: types.CoreConfig{
				ID:   agentID,
				Name: agentID,
				Type: string(TypeGeneric),
			},
			LLM: types.LLMConfig{
				Model: r.defaultResolverModel(),
			},
		}
		toolNames := r.toolNames
		if len(toolNames) == 0 && r.tools != nil {
			schemas := r.tools.GetAllowedTools(agentID)
			if len(schemas) > 0 {
				toolNames = make([]string, 0, len(schemas))
				for _, schema := range schemas {
					name := strings.TrimSpace(schema.Name)
					if name == "" {
						continue
					}
					toolNames = append(toolNames, name)
				}
			}
		}
		if len(toolNames) > 0 {
			cfg.Tools.AllowedTools = append([]string(nil), toolNames...)
			cfg.Runtime.Tools = append([]string(nil), toolNames...)
		}
		ag, err := r.registry.Create(cfg, r.gateway, r.memory, r.tools, nil, r.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent %q: %w", agentID, err)
		}

		// Inject MongoDB persistence stores.
		if ba, ok := ag.(*BaseAgent); ok {
			ba.SetPromptStore(r.promptStore)
			ba.SetConversationStore(r.conversationStore)
			ba.SetRunStore(r.runStore)
			if r.enhancedMemory != nil {
				ba.EnableEnhancedMemory(r.enhancedMemory)
			}
		}

		if err := ag.Init(ctx); err != nil {
			return nil, fmt.Errorf("failed to init agent %q: %w", agentID, err)
		}

		r.agents.Store(agentID, ag)
		return ag, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(Agent), nil
}

func (r *CachingResolver) defaultResolverModel() string {
	if model := strings.TrimSpace(r.modelHint); model != "" {
		return model
	}
	if provider := compatProviderFromGateway(r.gateway); provider != nil {
		if name := strings.TrimSpace(provider.Name()); name != "" {
			return name
		}
	}
	return "resolver-default"
}

// TeardownAll tears down all cached agent instances. Intended to be called
// during graceful shutdown.
func (r *CachingResolver) TeardownAll(ctx context.Context) {
	r.agents.Range(func(key, value any) bool {
		if ag, ok := value.(Agent); ok {
			if err := ag.Teardown(ctx); err != nil {
				r.logger.Warn("Failed to teardown cached agent",
					zap.String("agent_id", key.(string)),
					zap.Error(err))
			}
		}
		return true
	})
}

// ResetCache tears down and removes all cached agent instances.
// Future Resolve calls will recreate agents with latest runtime settings.
func (r *CachingResolver) ResetCache(ctx context.Context) {
	r.agents.Range(func(key, value any) bool {
		if ag, ok := value.(Agent); ok {
			if err := ag.Teardown(ctx); err != nil {
				r.logger.Warn("Failed to teardown cached agent during reset",
					zap.String("agent_id", key.(string)),
					zap.Error(err))
			}
		}
		r.agents.Delete(key)
		return true
	})
}
