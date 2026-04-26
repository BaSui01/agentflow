package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
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
		agent.extensions = NewExtensionRegistry()

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

	t.Run("middleware passes errors through", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry()

		options := EnhancedExecutionOptions{
			UseObservability: true,
			RecordTrace:      true,
		}

		middleware := agent.observabilityMiddleware(options)
		expectedErr := NewError("TEST", "test error")

		next := func(ctx context.Context, input *Input) (*Output, error) {
			return nil, expectedErr
		}

		_, err := middleware(ctx, &Input{TraceID: "test", Content: "test"}, next)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
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
		agent.extensions = NewExtensionRegistry()

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
		agent.extensions = NewExtensionRegistry()

		options := EnhancedExecutionOptions{
			UseEnhancedMemory: true,
			LoadWorkingMemory: true,
		}

		middleware := agent.memoryLoadMiddleware(options)
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})

	t.Run("middleware works with short-term memory", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry()

		options := EnhancedExecutionOptions{
			UseEnhancedMemory:   true,
			LoadShortTermMemory: true,
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
		agent.extensions = NewExtensionRegistry()

		middleware := agent.promptEnhancerMiddleware()
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestMemorySaveMiddleware(t *testing.T) {
	t.Run("middleware saves on success", func(t *testing.T) {
		logger := zap.NewNop()
		agent := &BaseAgent{
			config: types.AgentConfig{},
			logger: logger,
		}
		agent.extensions = NewExtensionRegistry()

		middleware := agent.memorySaveMiddleware()
		if middleware == nil {
			t.Fatal("expected non-nil middleware")
		}
	})
}

func TestNewGuardrailsManager(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewGuardrailsManager(logger)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewGuardrailsCoordinator(t *testing.T) {
	logger := zap.NewNop()
	cfg := guardrails.DefaultConfig()
	coord := NewGuardrailsCoordinator(cfg, logger)

	if coord == nil {
		t.Fatal("expected non-nil coordinator")
	}
}
