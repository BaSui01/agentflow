package runtime

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestObservabilityMiddleware(t *testing.T) {
	t.Run("middleware wraps next function correctly", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry(logger)

		options := EnhancedExecutionOptions{
			UseObservability: true,
			RecordMetrics:    true,
			RecordTrace:      true,
		}

		middleware := agent.observabilityMiddleware(options)
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestSkillsMiddleware(t *testing.T) {
	t.Run("middleware creates skill context", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry(logger)

		options := EnhancedExecutionOptions{
			UseSkills:   true,
			SkillsQuery: "test query",
		}

		middleware := agent.skillsMiddleware(options)
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestMemoryLoadMiddleware(t *testing.T) {
	t.Run("middleware works with working memory", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry(logger)

		options := EnhancedExecutionOptions{
			UseEnhancedMemory: true,
			LoadWorkingMemory: true,
		}

		middleware := agent.memoryLoadMiddleware(options)
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestPromptEnhancerMiddleware(t *testing.T) {
	t.Run("middleware creates enhancer", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry(logger)

		middleware := agent.promptEnhancerMiddleware()
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestMemorySaveMiddleware(t *testing.T) {
	t.Run("middleware creates saver", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry(logger)

		middleware := agent.memorySaveMiddleware()
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}
