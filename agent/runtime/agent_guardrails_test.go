package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestRuntimeGuardrailsFromTypes(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		got := runtimeGuardrailsFromTypes(nil)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("disabled config returns nil", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{Enabled: false}
		got := runtimeGuardrailsFromTypes(cfg)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("converts basic config", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{
			Enabled:            true,
			MaxInputLength:     5000,
			BlockedKeywords:    []string{"bad", "evil"},
			PIIDetection:       true,
			InjectionDetection: true,
			MaxRetries:         3,
			OnInputFailure:     "reject",
			OnOutputFailure:    "filter",
		}

		got := runtimeGuardrailsFromTypes(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.MaxInputLength != 5000 {
			t.Errorf("MaxInputLength: expected 5000, got %d", got.MaxInputLength)
		}
		if len(got.BlockedKeywords) != 2 {
			t.Errorf("BlockedKeywords: expected 2, got %d", len(got.BlockedKeywords))
		}
		if !got.PIIDetectionEnabled {
			t.Error("PIIDetectionEnabled: expected true")
		}
		if !got.InjectionDetection {
			t.Error("InjectionDetection: expected true")
		}
		if got.MaxRetries != 3 {
			t.Errorf("MaxRetries: expected 3, got %d", got.MaxRetries)
		}
		if got.OnInputFailure != guardrails.FailureAction("reject") {
			t.Errorf("OnInputFailure: expected reject, got %s", got.OnInputFailure)
		}
		if got.OnOutputFailure != guardrails.FailureAction("filter") {
			t.Errorf("OnOutputFailure: expected filter, got %s", got.OnOutputFailure)
		}
	})

	t.Run("trims whitespace from failure actions", func(t *testing.T) {
		cfg := &types.GuardrailsConfig{
			Enabled:        true,
			OnInputFailure: "  reject  ",
		}

		got := runtimeGuardrailsFromTypes(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.OnInputFailure != "reject" {
			t.Errorf("OnInputFailure: expected 'reject', got '%s'", got.OnInputFailure)
		}
	})
}

func TestTypesGuardrailsFromRuntime(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		got := typesGuardrailsFromRuntime(nil)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("converts runtime config to types", func(t *testing.T) {
		cfg := &guardrails.GuardrailsConfig{
			MaxInputLength:      3000,
			BlockedKeywords:     []string{"keyword1"},
			PIIDetectionEnabled: true,
			InjectionDetection:  true,
			MaxRetries:          2,
			OnInputFailure:      "reject",
			OnOutputFailure:     "warn",
		}

		got := typesGuardrailsFromRuntime(cfg)
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if !got.Enabled {
			t.Error("Enabled: expected true")
		}
		if got.MaxInputLength != 3000 {
			t.Errorf("MaxInputLength: expected 3000, got %d", got.MaxInputLength)
		}
		if len(got.BlockedKeywords) != 1 {
			t.Errorf("BlockedKeywords: expected 1, got %d", len(got.BlockedKeywords))
		}
		if got.BlockedKeywords[0] != "keyword1" {
			t.Errorf("BlockedKeywords[0]: expected keyword1, got %s", got.BlockedKeywords[0])
		}
		if got.OnInputFailure != "reject" {
			t.Errorf("OnInputFailure: expected reject, got %s", got.OnInputFailure)
		}
		if got.OnOutputFailure != "warn" {
			t.Errorf("OnOutputFailure: expected warn, got %s", got.OnOutputFailure)
		}
	})

	t.Run("does not mutate original slice", func(t *testing.T) {
		cfg := &guardrails.GuardrailsConfig{
			BlockedKeywords: []string{"a", "b"},
		}

		got := typesGuardrailsFromRuntime(cfg)
		got.BlockedKeywords[0] = "modified"

		if cfg.BlockedKeywords[0] != "a" {
			t.Error("original slice was mutated")
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

func TestBaseAgent_Guardrails(t *testing.T) {
	logger := zap.NewNop()

	t.Run("initGuardrails initializes validators", func(t *testing.T) {
		cfg := types.AgentConfig{
			Core: types.AgentCoreConfig{
				ID:   "test-agent",
				Name: "Test Agent",
			},
			ExecutionOptions_: types.AgentExecutionOptions{
				Model: types.ModelConfig{
					Model: "gpt-4",
				},
			},
		}
		gateway := &mockGateway{}
		agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

		guardrailsCfg := &guardrails.GuardrailsConfig{
			MaxInputLength:     1000,
			BlockedKeywords:    []string{"bad"},
			PIIDetectionEnabled: true,
		}
		agent.initGuardrails(guardrailsCfg)

		if !agent.guardrailsEnabled {
			t.Error("guardrailsEnabled: expected true")
		}
		if agent.inputValidatorChain == nil {
			t.Error("inputValidatorChain should not be nil")
		}
		if agent.outputValidator == nil {
			t.Error("outputValidator should not be nil")
		}
	})

	t.Run("SetGuardrails enables guardrails", func(t *testing.T) {
		cfg := types.AgentConfig{
			Core: types.AgentCoreConfig{
				ID:   "test-agent-2",
				Name: "Test Agent 2",
			},
			ExecutionOptions_: types.AgentExecutionOptions{
				Model: types.ModelConfig{
					Model: "gpt-4",
				},
			},
		}
		gateway := &mockGateway{}
		agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

		guardrailsCfg := &guardrails.GuardrailsConfig{
			MaxInputLength:  500,
			BlockedKeywords: []string{"keyword"},
		}
		agent.SetGuardrails(guardrailsCfg)

		if !agent.GuardrailsEnabled() {
			t.Error("GuardrailsEnabled(): expected true")
		}
		if agent.runtimeGuardrailsCfg == nil {
			t.Error("runtimeGuardrailsCfg should not be nil")
		}
		if agent.config.Features.Guardrails == nil {
			t.Error("config.Features.Guardrails should not be nil")
		}
	})

	t.Run("SetGuardrails with nil disables", func(t *testing.T) {
		cfg := types.AgentConfig{
			Core: types.AgentCoreConfig{
				ID:   "test-agent-3",
				Name: "Test Agent 3",
			},
			ExecutionOptions_: types.AgentExecutionOptions{
				Model: types.ModelConfig{
					Model: "gpt-4",
				},
			},
		}
		gateway := &mockGateway{}
		agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

		// First enable
		agent.SetGuardrails(&guardrails.GuardrailsConfig{MaxInputLength: 100})
		if !agent.GuardrailsEnabled() {
			t.Fatal("expected guardrails to be enabled")
		}

		// Then disable
		agent.SetGuardrails(nil)
		if agent.GuardrailsEnabled() {
			t.Error("GuardrailsEnabled(): expected false after setting nil")
		}
		if agent.inputValidatorChain != nil {
			t.Error("inputValidatorChain should be nil after disable")
		}
		if agent.outputValidator != nil {
			t.Error("outputValidator should be nil after disable")
		}
	})

	t.Run("AddInputValidator adds to chain", func(t *testing.T) {
		cfg := types.AgentConfig{
			Core: types.AgentCoreConfig{
				ID:   "test-agent-4",
				Name: "Test Agent 4",
			},
			ExecutionOptions_: types.AgentExecutionOptions{
				Model: types.ModelConfig{
					Model: "gpt-4",
				},
			},
		}
		gateway := &mockGateway{}
		agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

		// Add validator when chain is nil
		mockValidator := &mockValidator{}
		agent.AddInputValidator(mockValidator)

		if !agent.GuardrailsEnabled() {
			t.Error("GuardrailsEnabled(): expected true after adding validator")
		}
		if agent.inputValidatorChain == nil {
			t.Error("inputValidatorChain should not be nil after adding validator")
		}
	})

	t.Run("AddOutputValidator adds to validator", func(t *testing.T) {
		cfg := types.AgentConfig{
			Core: types.AgentCoreConfig{
				ID:   "test-agent-5",
				Name: "Test Agent 5",
			},
			ExecutionOptions_: types.AgentExecutionOptions{
				Model: types.ModelConfig{
					Model: "gpt-4",
				},
			},
		}
		gateway := &mockGateway{}
		agent := BuildBaseAgent(cfg, gateway, nil, nil, nil, logger, nil)

		mockValidator := &mockValidator{}
		agent.AddOutputValidator(mockValidator)

		if !agent.GuardrailsEnabled() {
			t.Error("GuardrailsEnabled(): expected true")
		}
		if agent.outputValidator == nil {
			t.Error("outputValidator should not be nil")
		}
	})
}

// mockValidator implements guardrails.Validator for testing.
type mockValidator struct{}

func (m *mockValidator) Validate(ctx context.Context, input string) (*guardrails.ValidationResult, error) {
	return &guardrails.ValidationResult{Valid: true}, nil
}

func (m *mockValidator) Name() string { return "mock" }

// mockGateway implements llmcore.Gateway for testing.
type mockGateway struct{}

func (m *mockGateway) Chat(ctx context.Context, req *types.ChatRequest) (*types.ChatResponse, error) {
	return &types.ChatResponse{}, nil
}

func (m *mockGateway) ChatStream(ctx context.Context, req *types.ChatRequest) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockGateway) Stream(ctx context.Context, req *types.ChatRequest) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockGateway) Embed(ctx context.Context, req *types.EmbedRequest) (*types.EmbedResponse, error) {
	return &types.EmbedResponse{}, nil
}

func (m *mockGateway) Rerank(ctx context.Context, req *types.RerankRequest) (*types.RerankResponse, error) {
	return &types.RerankResponse{}, nil
}

func (m *mockGateway) GenerateImage(ctx context.Context, req *types.ImageGenerationRequest) (*types.ImageGenerationResponse, error) {
	return &types.ImageGenerationResponse{}, nil
}

func (m *mockGateway) GenerateSpeech(ctx context.Context, req *types.SpeechRequest) (*types.SpeechResponse, error) {
	return &types.SpeechResponse{}, nil
}

func (m *mockGateway) Transcribe(ctx context.Context, req *types.TranscriptionRequest) (*types.TranscriptionResponse, error) {
	return &types.TranscriptionResponse{}, nil
}

func (m *mockGateway) Moderate(ctx context.Context, req *types.ModerationRequest) (*types.ModerationResponse, error) {
	return &types.ModerationResponse{}, nil
}
