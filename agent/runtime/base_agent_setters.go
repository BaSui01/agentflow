package runtime

import (
	"context"
	agentadapters "github.com/BaSui01/agentflow/agent/adapters"
	reasoning "github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	semaphore "golang.org/x/sync/semaphore"
)

// SetMaxConcurrency 设置 Agent 的最大并发执行数（默认 1）。
// 如果当前有执行在进行，会等待它们完成后才生效。
func (b *BaseAgent) SetMaxConcurrency(n int) {
	if n <= 0 {
		n = 1
	}
	b.configMu.Lock()
	defer b.configMu.Unlock()
	// 获取全部旧容量，确保没有正在执行的请求
	_ = b.execSem.Acquire(context.Background(), 1)
	b.execSem.Release(1)
	b.execSem = semaphore.NewWeighted(int64(n))
}
// SetRetrievalProvider configures retrieval-backed context injection.
func (b *BaseAgent) SetRetrievalProvider(provider RetrievalProvider) {
	b.retriever = provider
}

// SetToolStateProvider configures tool/artifact state-backed context injection.
func (b *BaseAgent) SetToolStateProvider(provider ToolStateProvider) {
	b.toolState = provider
}
// SetContextManager 设置上下文管理器
func (b *BaseAgent) SetContextManager(cm ContextManager) {
	b.contextManager = cm
	b.contextEngineEnabled = cm != nil
	if cm != nil {
		b.logger.Info("context manager enabled")
	}
}
// SetPromptStore sets the prompt store provider.
func (b *BaseAgent) SetPromptStore(store PromptStoreProvider) {
	b.persistence.SetPromptStore(store)
}

// SetConversationStore sets the conversation store provider.
func (b *BaseAgent) SetConversationStore(store ConversationStoreProvider) {
	b.persistence.SetConversationStore(store)
}

// SetRunStore sets the run store provider.
func (b *BaseAgent) SetRunStore(store RunStoreProvider) {
	b.persistence.SetRunStore(store)
}
// SetReasoningRegistry stores the reasoning registry used by the default loop executor.
func (b *BaseAgent) SetReasoningRegistry(registry *reasoning.PatternRegistry) {
	b.reasoningRegistry = registry
}

// ReasoningRegistry returns the configured reasoning registry.
func (b *BaseAgent) ReasoningRegistry() *reasoning.PatternRegistry {
	return b.reasoningRegistry
}

// SetReasoningModeSelector stores the mode selector used by the default loop executor.
func (b *BaseAgent) SetReasoningModeSelector(selector ReasoningModeSelector) {
	b.reasoningSelector = selector
}

// SetExecutionOptionsResolver stores the resolver used by request preparation.
func (b *BaseAgent) SetExecutionOptionsResolver(resolver ExecutionOptionsResolver) {
	if resolver == nil {
		b.optionsResolver = NewDefaultExecutionOptionsResolver()
		return
	}
	b.optionsResolver = resolver
}

func (b *BaseAgent) executionOptionsResolver() ExecutionOptionsResolver {
	if b.optionsResolver == nil {
		return NewDefaultExecutionOptionsResolver()
	}
	return b.optionsResolver
}
// SetChatRequestAdapter stores the adapter used to build ChatRequest DTOs.
func (b *BaseAgent) SetChatRequestAdapter(adapter agentadapters.ChatRequestAdapter) {
	if adapter == nil {
		b.requestAdapter = agentadapters.NewDefaultChatRequestAdapter()
		return
	}
	b.requestAdapter = adapter
}

func (b *BaseAgent) chatRequestAdapter() agentadapters.ChatRequestAdapter {
	if b.requestAdapter == nil {
		return agentadapters.NewDefaultChatRequestAdapter()
	}
	return b.requestAdapter
}

// SetToolProtocolRuntime stores the runtime that materializes tool execution.
func (b *BaseAgent) SetToolProtocolRuntime(runtime ToolProtocolRuntime) {
	if runtime == nil {
		b.toolProtocol = NewDefaultToolProtocolRuntime()
		return
	}
	b.toolProtocol = runtime
}

func (b *BaseAgent) toolProtocolRuntime() ToolProtocolRuntime {
	if b.toolProtocol == nil {
		return NewDefaultToolProtocolRuntime()
	}
	return b.toolProtocol
}
// SetReasoningRuntime stores the runtime that unifies reasoning selection,
// execution, and reflection for the default loop executor.
func (b *BaseAgent) SetReasoningRuntime(runtime ReasoningRuntime) {
	b.reasoningRuntime = runtime
}
// SetTraceFeedbackPlanner stores the planner used to decide whether recent
// trace synopsis/history should be injected back into runtime prompt layers.
func (b *BaseAgent) SetTraceFeedbackPlanner(planner TraceFeedbackPlanner) {
	if planner == nil {
		b.traceFeedbackPlanner = NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
		return
	}
	b.traceFeedbackPlanner = planner
}

// SetMemoryRuntime stores memory recall/observe runtime used by execute path.
func (b *BaseAgent) SetMemoryRuntime(runtime MemoryRuntime) {
	if runtime == nil {
		b.memoryRuntime = NewDefaultMemoryRuntime(func() *UnifiedMemoryFacade { return b.memoryFacade }, func() MemoryManager { return b.memory }, b.logger)
		return
	}
	b.memoryRuntime = runtime
}

// SetCompletionJudge stores the completion judge used by the default loop executor.
func (b *BaseAgent) SetCompletionJudge(judge CompletionJudge) {
	b.completionJudge = judge
}

// SetCheckpointManager stores the checkpoint manager used by the default loop executor.
func (b *BaseAgent) SetCheckpointManager(manager *CheckpointManager) {
	b.checkpointManager = manager
}
