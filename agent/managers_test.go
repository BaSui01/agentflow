package agent

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

func TestMemoryCoordinator_Basic(t *testing.T) {
	logger := zap.NewNop()
	mockMemory := &mockMemoryManager{}
	mc := NewMemoryCoordinator("test-agent", mockMemory, logger)

	if !mc.HasMemory() {
		t.Error("expected HasMemory to return true")
	}

	if mc.GetMemoryManager() == nil {
		t.Error("expected GetMemoryManager to return non-nil")
	}
}

func TestMemoryCoordinator_NoMemory(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMemoryCoordinator("test-agent", nil, logger)

	if mc.HasMemory() {
		t.Error("expected HasMemory to return false")
	}

	// Should not error when memory is nil
	err := mc.Save(context.Background(), "test", MemoryShortTerm, nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	records, err := mc.Search(context.Background(), "test", 10)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if records != nil {
		t.Error("expected nil records")
	}
}

func TestMemoryCoordinator_RecentMemory(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMemoryCoordinator("test-agent", nil, logger)

	// Initially empty
	recent := mc.GetRecentMemory()
	if len(recent) != 0 {
		t.Errorf("expected empty recent memory, got %d", len(recent))
	}

	// Clear should not panic
	mc.ClearRecentMemory()
}

func TestGuardrailsCoordinator_Disabled(t *testing.T) {
	logger := zap.NewNop()
	gc := NewGuardrailsCoordinator(nil, logger)

	if gc.Enabled() {
		t.Error("expected guardrails to be disabled")
	}

	// Should pass through when disabled
	result, err := gc.ValidateInput(context.Background(), "test input")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !result.Valid {
		t.Error("expected valid result when disabled")
	}

	output, result, err := gc.ValidateOutput(context.Background(), "test output")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if output != "test output" {
		t.Errorf("expected output to pass through, got %s", output)
	}
	if !result.Valid {
		t.Error("expected valid result when disabled")
	}
}

func TestGuardrailsCoordinator_Enabled(t *testing.T) {
	logger := zap.NewNop()
	config := &guardrails.GuardrailsConfig{
		MaxInputLength:  100,
		BlockedKeywords: []string{"blocked"},
	}
	gc := NewGuardrailsCoordinator(config, logger)

	if !gc.Enabled() {
		t.Error("expected guardrails to be enabled")
	}

	if gc.InputValidatorCount() == 0 {
		t.Error("expected at least one input validator")
	}
}

func TestGuardrailsCoordinator_AddValidators(t *testing.T) {
	logger := zap.NewNop()
	gc := NewGuardrailsCoordinator(nil, logger)

	// Initially disabled
	if gc.Enabled() {
		t.Error("expected guardrails to be disabled initially")
	}

	// Add input validator should enable
	gc.AddInputValidator(guardrails.NewLengthValidator(&guardrails.LengthValidatorConfig{
		MaxLength: 100,
	}))

	if !gc.Enabled() {
		t.Error("expected guardrails to be enabled after adding validator")
	}
}

func TestGuardrailsCoordinator_BuildFeedbackMessage(t *testing.T) {
	logger := zap.NewNop()
	gc := NewGuardrailsCoordinator(nil, logger)

	result := &guardrails.ValidationResult{
		Valid: false,
		Errors: []guardrails.ValidationError{
			{Code: "LENGTH", Message: "Input too long"},
			{Code: "KEYWORD", Message: "Blocked keyword found"},
		},
	}

	msg := gc.BuildValidationFeedbackMessage(result)

	if msg == "" {
		t.Error("expected non-empty feedback message")
	}

	if !contains(msg, "LENGTH") || !contains(msg, "KEYWORD") {
		t.Error("expected feedback message to contain error codes")
	}
}

func TestFeatureManager_Basic(t *testing.T) {
	logger := zap.NewNop()
	fm := NewFeatureManager(logger)

	// Initially all disabled
	features := fm.EnabledFeatures()
	if len(features) != 0 {
		t.Errorf("expected no enabled features, got %v", features)
	}
}

func TestFeatureManager_EnableDisable(t *testing.T) {
	logger := zap.NewNop()
	fm := NewFeatureManager(logger)

	// Enable reflection
	fm.EnableReflection("mock-executor")
	if !fm.IsReflectionEnabled() {
		t.Error("expected reflection to be enabled")
	}
	if fm.GetReflection() != "mock-executor" {
		t.Error("expected to get mock executor")
	}

	// Disable reflection
	fm.DisableReflection()
	if fm.IsReflectionEnabled() {
		t.Error("expected reflection to be disabled")
	}
	if fm.GetReflection() != nil {
		t.Error("expected nil after disable")
	}
}

func TestFeatureManager_AllFeatures(t *testing.T) {
	logger := zap.NewNop()
	fm := NewFeatureManager(logger)

	// Enable all features
	fm.EnableReflection("reflection")
	fm.EnableToolSelection("tool-selector")
	fm.EnablePromptEnhancer("prompt-enhancer")
	fm.EnableSkills("skill-manager")
	fm.EnableMCP("mcp-server")
	fm.EnableEnhancedMemory("enhanced-memory")
	fm.EnableObservability("observability")

	features := fm.EnabledFeatures()
	if len(features) != 7 {
		t.Errorf("expected 7 enabled features, got %d: %v", len(features), features)
	}

	// Disable all
	fm.DisableAll()
	features = fm.EnabledFeatures()
	if len(features) != 0 {
		t.Errorf("expected no enabled features after DisableAll, got %v", features)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock memory manager for testing
type mockMemoryManager struct{}

func (m *mockMemoryManager) Save(ctx context.Context, record MemoryRecord) error {
	return nil
}

func (m *mockMemoryManager) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockMemoryManager) Clear(ctx context.Context, agentID string, kind MemoryKind) error {
	return nil
}

func (m *mockMemoryManager) Search(ctx context.Context, agentID, query string, topK int) ([]MemoryRecord, error) {
	return []MemoryRecord{}, nil
}

func (m *mockMemoryManager) LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
	return []MemoryRecord{
		{AgentID: agentID, Kind: kind, Content: "test", CreatedAt: time.Now()},
	}, nil
}

func (m *mockMemoryManager) Get(ctx context.Context, id string) (*MemoryRecord, error) {
	return nil, nil
}
