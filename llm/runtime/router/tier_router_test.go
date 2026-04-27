package router

import (
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func newTestTierRouter(enabled bool) *TierRouter {
	cfg := DefaultTierConfig()
	cfg.Enabled = enabled
	return NewTierRouter(cfg, zap.NewNop())
}

func TestScoreComplexity_SimpleRequest(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	req := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}
	score := tr.ScoreComplexity(req)

	// 1 message (<=2 → 5) + short content <500 (→ 5) + 0 tools (→ 0) + no system prompt (→ 0) = 10
	if score != 10 {
		t.Fatalf("expected score 10, got %d", score)
	}
}

func TestScoreComplexity_ComplexRequest(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	msgs := make([]Message, 25)
	msgs[0] = Message{Role: "system", Content: strings.Repeat("x", 5000)}
	for i := 1; i < 25; i++ {
		msgs[i] = Message{Role: "user", Content: strings.Repeat("a", 500)}
	}
	tools := make([]types.ToolSchema, 10)
	for i := range tools {
		tools[i] = types.ToolSchema{Name: "tool"}
	}

	req := &ChatRequest{
		Model:    "gpt-4o",
		Messages: msgs,
		Tools:    tools,
	}
	score := tr.ScoreComplexity(req)

	// 25 messages (>20 → 25) + total ~17000 chars (>10000 → 25) + 10 tools (>8 → 25) + system 5000 (>=3000 → 25) = 100
	if score != 100 {
		t.Fatalf("expected score 100, got %d", score)
	}
}

func TestScoreComplexity_NilRequest(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	score := tr.ScoreComplexity(nil)
	if score != 50 {
		t.Fatalf("expected score 50 for nil request, got %d", score)
	}
}

func TestSelectTier(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	tests := []struct {
		score int
		want  ModelTier
	}{
		{0, TierNano},
		{10, TierNano},
		{30, TierNano},
		{31, TierStandard},
		{50, TierStandard},
		{69, TierStandard},
		{70, TierFrontier},
		{85, TierFrontier},
		{100, TierFrontier},
	}
	for _, tt := range tests {
		got := tr.SelectTier(tt.score)
		if got != tt.want {
			t.Errorf("SelectTier(%d) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

func TestSelectModel_FamilyMatching(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	got := tr.SelectModel(TierNano, "gpt-4o")
	if got != "gpt-5.4-nano" {
		t.Fatalf("expected gpt-5.4-nano, got %s", got)
	}

	got = tr.SelectModel(TierFrontier, "claude-sonnet-4-6")
	if got != "claude-opus-4-7" {
		t.Fatalf("expected claude-opus-4-7, got %s", got)
	}

	got = tr.SelectModel(TierStandard, "gemini-flash")
	if got != "gemini-2.5-pro" {
		t.Fatalf("expected gemini-2.5-pro, got %s", got)
	}
}

func TestSelectModel_UnknownFamily(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	got := tr.SelectModel(TierNano, "some-custom-model")
	if got != "gpt-5.4-nano" {
		t.Fatalf("expected first candidate gpt-5.4-nano, got %s", got)
	}
}

func TestSelectModel_EmptyCandidates(t *testing.T) {
	t.Parallel()
	cfg := TierConfig{Enabled: true, NanoThreshold: 30, FrontierThreshold: 70}
	tr := NewTierRouter(cfg, zap.NewNop())

	got := tr.SelectModel(TierNano, "gpt-4o")
	if got != "gpt-4o" {
		t.Fatalf("expected original model gpt-4o, got %s", got)
	}
}

func TestResolveModel_Disabled(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(false)

	req := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	}
	got := tr.ResolveModel(req)
	if got != "gpt-4o" {
		t.Fatalf("disabled tier routing should return original model, got %s", got)
	}
}

func TestResolveModel_Enabled(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	simple := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	}
	got := tr.ResolveModel(simple)
	if got != "gpt-5.4-nano" {
		t.Fatalf("expected gpt-5.4-nano for simple request, got %s", got)
	}

	msgs := make([]Message, 25)
	msgs[0] = Message{Role: "system", Content: strings.Repeat("x", 5000)}
	for i := 1; i < 25; i++ {
		msgs[i] = Message{Role: "user", Content: strings.Repeat("a", 500)}
	}
	tools := make([]types.ToolSchema, 10)
	for i := range tools {
		tools[i] = types.ToolSchema{Name: "tool"}
	}
	complex := &ChatRequest{
		Model:    "gpt-4o",
		Messages: msgs,
		Tools:    tools,
	}
	got = tr.ResolveModel(complex)
	if got != "gpt-5.5" {
		t.Fatalf("expected gpt-5.5 for complex request, got %s", got)
	}
}

func TestResolveModel_NilRequest(t *testing.T) {
	t.Parallel()
	tr := newTestTierRouter(true)

	got := tr.ResolveModel(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil request, got %s", got)
	}
}

func TestNewTierRouter_DefaultThresholds(t *testing.T) {
	t.Parallel()
	cfg := TierConfig{Enabled: true}
	tr := NewTierRouter(cfg, nil)

	if tr.config.NanoThreshold != 30 {
		t.Fatalf("expected default nano threshold 30, got %d", tr.config.NanoThreshold)
	}
	if tr.config.FrontierThreshold != 70 {
		t.Fatalf("expected default frontier threshold 70, got %d", tr.config.FrontierThreshold)
	}
}

func TestExtractFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  string
	}{
		{"gpt-4o", "gpt"},
		{"GPT-4o-mini", "gpt"},
		{"claude-sonnet-3.5", "claude"},
		{"gemini-pro", "gemini"},
		{"deepseek-v3", "deepseek"},
		{"qwen-72b", "qwen"},
		{"unknown-model", ""},
	}
	for _, tt := range tests {
		got := extractFamily(tt.model)
		if got != tt.want {
			t.Errorf("extractFamily(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

// --- Custom weight tests ---

func TestScoreComplexity_CustomWeights(t *testing.T) {
	t.Parallel()

	cfg := DefaultTierConfig()
	cfg.Enabled = true
	cfg.Weights = ScoringWeights{
		MessageCount:  50,
		ContentLength: 0,
		ToolCount:     0,
		SystemPrompt:  0,
	}
	tr := NewTierRouter(cfg, zap.NewNop())

	req := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}
	score := tr.ScoreComplexity(req)

	// msgScore=5 (<=2 msgs), applyWeight(5, 50) = 5*50/25 = 10
	// All other weights are 0
	if score != 10 {
		t.Fatalf("expected score 10 with MessageCount=50 only, got %d", score)
	}
}

func TestScoreComplexity_ToolHeavyWeighting(t *testing.T) {
	t.Parallel()

	cfg := DefaultTierConfig()
	cfg.Enabled = true
	cfg.Weights = ScoringWeights{
		MessageCount:  10,
		ContentLength: 10,
		ToolCount:     50,
		SystemPrompt:  10,
	}
	tr := NewTierRouter(cfg, zap.NewNop())

	tools := make([]types.ToolSchema, 10)
	for i := range tools {
		tools[i] = types.ToolSchema{Name: "tool"}
	}
	req := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
		Tools: tools,
	}
	score := tr.ScoreComplexity(req)

	// msg: applyWeight(5, 10) = 5*10/25 = 2
	// content: applyWeight(5, 10) = 2
	// tools (>8): applyWeight(25, 50) = 25*50/25 = 50
	// system: applyWeight(0, 10) = 0
	// total = 54
	if score != 54 {
		t.Fatalf("expected score 54 with tool-heavy weights, got %d", score)
	}
}

func TestScoreComplexity_ZeroWeightsDefault(t *testing.T) {
	t.Parallel()

	cfg := TierConfig{
		Enabled:           true,
		NanoThreshold:     30,
		FrontierThreshold: 70,
	}
	tr := NewTierRouter(cfg, zap.NewNop())

	req := &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}
	score := tr.ScoreComplexity(req)

	// Zero weights → defaults applied → same as default: 5+5+0+0 = 10
	if score != 10 {
		t.Fatalf("expected score 10 with default weights, got %d", score)
	}
}

func TestApplyWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw, weight, want int
	}{
		{5, 25, 5},
		{10, 25, 10},
		{15, 25, 15},
		{20, 25, 20},
		{25, 25, 25},
		{5, 50, 10},
		{25, 50, 50},
		{5, 10, 2},
		{25, 0, 0},
		{0, 25, 0},
	}
	for _, tt := range tests {
		got := applyWeight(tt.raw, tt.weight)
		if got != tt.want {
			t.Errorf("applyWeight(%d, %d) = %d, want %d", tt.raw, tt.weight, got, tt.want)
		}
	}
}
