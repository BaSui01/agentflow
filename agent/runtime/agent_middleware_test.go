package runtime

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestObservabilityMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := types.AgentConfig{
		Core: types.AgentCoreConfig{
			ID:   "test-obs",
			Name: "Test Obs",
		},
		ExecutionOptions_: types.AgentExecutionOptions{
			Model: types.ModelConfig{Model: "gpt-4"},
		},
	}
	gateway := &mockGateway{}
	agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

	// Enable observability
	agent.extensions.EnableObservability(&mockObservabilityRunner{})

	t.Run("records trace on successful execution", func(t *testing.T) {
		options := EnhancedExecutionOptions{
			UseObservability: true,
			RecordMetrics:    true,
			RecordTrace:      true,
		}

		middleware := agent.observabilityMiddleware(options)
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-1",
			Content: "Hello",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{
				Content:    "World",
				TokensUsed: 100,
				Cost:       0.01,
			}, nil
		}

		output, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Content != "World" {
			t.Errorf("Content: expected World, got %s", output.Content)
		}
	})

	t.Run("records error trace", func(t *testing.T) {
		options := EnhancedExecutionOptions{
			UseObservability: true,
			RecordTrace:      true,
		}

		middleware := agent.observabilityMiddleware(options)
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-2",
			Content: "Error test",
		}

		expectedErr := NewError("TEST_ERROR", "test error")
		next := func(ctx context.Context, input *Input) (*Output, error) {
			return nil, expectedErr
		}

		output, err := middleware(ctx, input, next)
		if err != expectedErr {
			t.Errorf("expected error, got %v", err)
		}
		if output != nil {
			t.Error("expected nil output on error")
		}
	})
}

func TestSkillsMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := types.AgentConfig{
		Core: types.AgentCoreConfig{
			ID:   "test-skills",
			Name: "Test Skills",
		},
		ExecutionOptions_: types.AgentExecutionOptions{
			Model: types.ModelConfig{Model: "gpt-4"},
		},
	}
	gateway := &mockGateway{}
	agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

	// Enable skills
	agent.extensions.EnableSkills(&mockSkillDiscoverer{})

	t.Run("injects skill instructions", func(t *testing.T) {
		options := EnhancedExecutionOptions{
			UseSkills:   true,
			SkillsQuery: "test query",
		}

		middleware := agent.skillsMiddleware(options)
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-3",
			Content: "Hello",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			// Check if skill context was injected
			if input.Context == nil {
				t.Error("expected context to be set")
			}
			return &Output{Content: "Response"}, nil
		}

		_, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMemoryLoadMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := types.AgentConfig{
		Core: types.AgentCoreConfig{
			ID:   "test-memory",
			Name: "Test Memory",
		},
		ExecutionOptions_: types.AgentExecutionOptions{
			Model: types.ModelConfig{Model: "gpt-4"},
		},
	}
	gateway := &mockGateway{}
	agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

	// Enable enhanced memory
	agent.extensions.EnableEnhancedMemory(&mockEnhancedMemoryRunner{})

	t.Run("loads working memory", func(t *testing.T) {
		options := EnhancedExecutionOptions{
			UseEnhancedMemory:  true,
			LoadWorkingMemory:  true,
			LoadShortTermMemory: false,
		}

		middleware := agent.memoryLoadMiddleware(options)
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-4",
			Content: "Hello",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "Response"}, nil
		}

		_, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("loads short-term memory", func(t *testing.T) {
		options := EnhancedExecutionOptions{
			UseEnhancedMemory:   true,
			LoadWorkingMemory:   false,
			LoadShortTermMemory: true,
		}

		middleware := agent.memoryLoadMiddleware(options)
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-5",
			Content: "Hello",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "Response"}, nil
		}

		_, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestPromptEnhancerMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := types.AgentConfig{
		Core: types.AgentCoreConfig{
			ID:   "test-enhancer",
			Name: "Test Enhancer",
		},
		ExecutionOptions_: types.AgentExecutionOptions{
			Model: types.ModelConfig{Model: "gpt-4"},
		},
	}
	gateway := &mockGateway{}
	agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

	// Enable prompt enhancer
	agent.extensions.EnablePromptEnhancer(&mockPromptEnhancerRunner{})

	t.Run("enhances prompt", func(t *testing.T) {
		middleware := agent.promptEnhancerMiddleware()
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-6",
			Content: "Simple prompt",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			if input.Content != "Enhanced: Simple prompt" {
				t.Errorf("expected enhanced prompt, got %s", input.Content)
			}
			return &Output{Content: "Response"}, nil
		}

		_, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMemorySaveMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := types.AgentConfig{
		Core: types.AgentCoreConfig{
			ID:   "test-save",
			Name: "Test Save",
		},
		ExecutionOptions_: types.AgentExecutionOptions{
			Model: types.ModelConfig{Model: "gpt-4"},
		},
	}
	gateway := &mockGateway{}
	agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

	// Enable enhanced memory
	agent.extensions.EnableEnhancedMemory(&mockEnhancedMemoryRunner{})

	t.Run("saves to memory on success", func(t *testing.T) {
		middleware := agent.memorySaveMiddleware()
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-7",
			Content: "Input",
		}

		next := func(ctx context.Context, input *Input) (*Output, error) {
			return &Output{Content: "Success response"}, nil
		}

		output, err := middleware(ctx, input, next)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Content != "Success response" {
			t.Errorf("Content: expected 'Success response', got %s", output.Content)
		}
	})

	t.Run("does not save on error", func(t *testing.T) {
		middleware := agent.memorySaveMiddleware()
		ctx := context.Background()
		input := &Input{
			TraceID: "test-trace-8",
			Content: "Input",
		}

		expectedErr := NewError("TEST", "error")
		next := func(ctx context.Context, input *Input) (*Output, error) {
			return nil, expectedErr
		}

		output, err := middleware(ctx, input, next)
		if err != expectedErr {
			t.Errorf("expected error, got %v", err)
		}
		if output != nil {
			t.Error("expected nil output on error")
		}
	})
}

// Mock implementations

type mockObservabilityRunner struct{}

func (m *mockObservabilityRunner) StartTrace(traceID, agentID string)              {}
func (m *mockObservabilityRunner) EndTrace(traceID, status string, err error)       {}
func (m *mockObservabilityRunner) RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost float64, confidence float64) {
}
func (m *mockObservabilityRunner) StartExplainabilityTrace(traceID, sessionID, agentID string)    {}
func (m *mockObservabilityRunner) EndExplainabilityTrace(traceID string, success bool, result, errMsg string) {
}

type mockSkillDiscoverer struct{}

func (m *mockSkillDiscoverer) DiscoverSkills(ctx context.Context, query string) ([]Skill, error) {
	return []Skill{&mockSkill{}}, nil
}
func (m *mockSkillDiscoverer) GetSkill(name string) (Skill, error) { return &mockSkill{}, nil }
func (m *mockSkillDiscoverer) ListSkills() []Skill                 { return nil }

type mockSkill struct{}

func (m *mockSkill) GetInstructions() string { return "Mock skill instructions" }
func (m *mockSkill) GetName() string         { return "mock-skill" }

type mockEnhancedMemoryRunner struct{}

func (m *mockEnhancedMemoryRunner) LoadWorking(ctx context.Context, agentID string) ([]MemoryEntry, error) {
	return []MemoryEntry{{Content: "Working memory entry"}}, nil
}
func (m *mockEnhancedMemoryRunner) LoadShortTerm(ctx context.Context, agentID string, topK int) ([]MemoryEntry, error) {
	return []MemoryEntry{{Content: "Short-term memory entry"}}, nil
}
func (m *mockEnhancedMemoryRunner) Save(ctx context.Context, agentID string, entry MemoryEntry) error {
	return nil
}

type mockPromptEnhancerRunner struct{}

func (m *mockPromptEnhancerRunner) EnhanceUserPrompt(prompt, context string) (string, error) {
	return "Enhanced: " + prompt, nil
}
