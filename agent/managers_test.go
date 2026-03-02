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

	// 当内存为零时不应出错
	err := mc.Save(context.Background(), "test", MemoryShortTerm, nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	records, err := mc.Search(context.Background(), "test", 10)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected empty records, got %d", len(records))
	}
}

func TestMemoryCoordinator_RecentMemory(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMemoryCoordinator("test-agent", nil, logger)

	// 最初为空
	recent := mc.GetRecentMemory()
	if len(recent) != 0 {
		t.Errorf("expected empty recent memory, got %d", len(recent))
	}

	// 清空不应该慌张
	mc.ClearRecentMemory()
}

func TestGuardrailsCoordinator_Disabled(t *testing.T) {
	logger := zap.NewNop()
	gc := NewGuardrailsCoordinator(nil, logger)

	if gc.Enabled() {
		t.Error("expected guardrails to be disabled")
	}

	// 残疾时应该通过
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

	// 最初已禁用
	if gc.Enabled() {
		t.Error("expected guardrails to be disabled initially")
	}

	// 添加输入验证器应启用
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

// contains checks if s contains substr.
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

// mockMemoryManager is a test double for MemoryManager.
type mockMemoryManager struct{}

func (m *mockMemoryManager) Save(_ context.Context, _ MemoryRecord) error {
	return nil
}

func (m *mockMemoryManager) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockMemoryManager) Clear(_ context.Context, _ string, _ MemoryKind) error {
	return nil
}

func (m *mockMemoryManager) Search(_ context.Context, _, _ string, _ int) ([]MemoryRecord, error) {
	return []MemoryRecord{}, nil
}

func (m *mockMemoryManager) LoadRecent(_ context.Context, agentID string, kind MemoryKind, _ int) ([]MemoryRecord, error) {
	return []MemoryRecord{
		{AgentID: agentID, Kind: kind, Content: "test", CreatedAt: time.Now()},
	}, nil
}

func (m *mockMemoryManager) Get(_ context.Context, _ string) (*MemoryRecord, error) {
	return nil, nil
}

// TestMemoryCoordinator_SaveWriteThroughCache verifies that Save() appends
// the new record to the in-process recentMemory cache.
func TestMemoryCoordinator_SaveWriteThroughCache(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMemoryCoordinator("test-agent", &mockMemoryManager{}, logger)

	ctx := context.Background()

	// Load initial (empty) cache.
	_ = mc.LoadRecent(ctx, MemoryShortTerm, 10)
	if len(mc.GetRecentMemory()) != 1 {
		// mockMemoryManager.LoadRecent returns 1 record
		t.Fatalf("expected 1 record after LoadRecent, got %d", len(mc.GetRecentMemory()))
	}

	// Save a new record — cache should grow.
	err := mc.Save(ctx, "new-content", MemoryShortTerm, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recent := mc.GetRecentMemory()
	if len(recent) != 2 {
		t.Errorf("expected 2 records after Save, got %d", len(recent))
	}
	if recent[1].Content != "new-content" {
		t.Errorf("expected last record content to be 'new-content', got %q", recent[1].Content)
	}
}

// TestMemoryCoordinator_SaveCacheEviction verifies that the cache is bounded.
func TestMemoryCoordinator_SaveCacheEviction(t *testing.T) {
	logger := zap.NewNop()
	mc := NewMemoryCoordinator("test-agent", &mockMemoryManager{}, logger)

	ctx := context.Background()

	for i := 0; i < defaultMaxRecentMemory+5; i++ {
		_ = mc.Save(ctx, "msg", MemoryShortTerm, nil)
	}

	recent := mc.GetRecentMemory()
	if len(recent) != defaultMaxRecentMemory {
		t.Errorf("expected cache size %d, got %d", defaultMaxRecentMemory, len(recent))
	}
}


