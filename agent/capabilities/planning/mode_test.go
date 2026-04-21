package deliberation

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles (function callback pattern, §30) ---

type mockReasoner struct {
	thinkFn func(ctx context.Context, prompt string) (string, float64, error)
}

func (m *mockReasoner) Think(ctx context.Context, prompt string) (content string, confidence float64, err error) {
	if m.thinkFn != nil {
		return m.thinkFn(ctx, prompt)
	}
	return "", 0, nil
}

// --- Immediate mode ---

func TestDeliberate_ImmediateMode(t *testing.T) {
	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeImmediate

	engine := NewEngine(cfg, &mockReasoner{}, nil)

	task := Task{
		ID:            "task-1",
		Description:   "simple task",
		SuggestedTool: "search",
		Parameters:    map[string]any{"q": "hello"},
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Equal(t, "task-1", result.TaskID)
	assert.Equal(t, "search", result.Decision.Tool)
	assert.Equal(t, 1.0, result.Decision.Confidence)
	assert.Equal(t, 0, result.Iterations)
	assert.Empty(t, result.Thoughts)
}

// --- Deliberate mode full cycle ---

func TestDeliberate_FullCycle(t *testing.T) {
	var callCount int
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			callCount++
			if strings.Contains(prompt, "Plan action") {
				return "Use search tool.\nTOOL: search\nCONFIDENCE: 0.9", 0.9, nil
			}
			return "analysis complete\nCONFIDENCE: 0.8", 0.8, nil
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate
	cfg.MinConfidence = 0.7

	engine := NewEngine(cfg, reasoner, nil)

	task := Task{
		ID:             "task-2",
		Description:    "complex task",
		Goal:           "find information",
		AvailableTools: []string{"search", "browse", "calculate"},
		SuggestedTool:  "browse",
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Equal(t, 1, result.Iterations)
	assert.Equal(t, 3, callCount) // understand, evaluate, plan
	assert.Len(t, result.Thoughts, 3)
	assert.Equal(t, "understand", result.Thoughts[0].Type)
	assert.Equal(t, "evaluate", result.Thoughts[1].Type)
	assert.Equal(t, "plan", result.Thoughts[2].Type)
	assert.Equal(t, "search", result.Decision.Tool) // parsed from TOOL: line
	assert.Equal(t, 0.9, result.FinalConfidence)
}

// APPEND_MARKER_2

// --- Adaptive mode selects correct mode ---

func TestDeliberate_AdaptiveSelectsImmediate(t *testing.T) {
	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeAdaptive

	engine := NewEngine(cfg, &mockReasoner{}, nil)

	// Simple task: 1 tool, short goal, no context -> complexity < 3 -> immediate
	task := Task{
		ID:             "task-simple",
		Description:    "quick lookup",
		Goal:           "find x",
		AvailableTools: []string{"search"},
		SuggestedTool:  "search",
		Parameters:     map[string]any{"q": "test"},
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Equal(t, 0, result.Iterations)
	assert.Equal(t, 1.0, result.Decision.Confidence)
}

func TestDeliberate_AdaptiveSelectsDeliberate(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "reasoning\nCONFIDENCE: 0.85", 0.85, nil
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeAdaptive
	cfg.MinConfidence = 0.7

	engine := NewEngine(cfg, reasoner, nil)

	// Complex task: 4 tools + long goal -> complexity >= 3 -> deliberate
	task := Task{
		ID:          "task-complex",
		Description: "multi-step research",
		Goal:        strings.Repeat("analyze deeply ", 10), // > 100 chars
		AvailableTools: []string{
			"search", "browse", "calculate", "summarize",
		},
		SuggestedTool: "search",
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Greater(t, result.Iterations, 0) // ran deliberation loop
}

// APPEND_MARKER_3

// --- Timeout cancellation between steps ---

func TestDeliberate_TimeoutBetweenSteps(t *testing.T) {
	var calls int32
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			atomic.AddInt32(&calls, 1)
			return "thought\nCONFIDENCE: 0.5", 0.5, nil
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate
	cfg.MaxThinkingTime = 50 * time.Millisecond
	cfg.MinConfidence = 0.99 // force multiple iterations
	cfg.MaxIterations = 10

	engine := NewEngine(cfg, reasoner, nil)

	// Pre-cancel the context to test immediate cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	task := Task{
		ID:          "task-timeout",
		Description: "will timeout",
		Goal:        "test",
	}

	_, err := engine.Deliberate(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
}

// --- Self-critique loop when confidence is low ---

func TestDeliberate_SelfCritiqueLoop(t *testing.T) {
	iteration := 0
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			if strings.Contains(prompt, "Plan action") {
				iteration++
				if iteration >= 2 {
					// Second plan iteration: high confidence
					return "final plan\nTOOL: search\nCONFIDENCE: 0.95", 0.95, nil
				}
				// First plan: low confidence triggers critique
				return "tentative plan\nTOOL: search\nCONFIDENCE: 0.3", 0.3, nil
			}
			if strings.Contains(prompt, "Critique") {
				return "needs improvement\nCONFIDENCE: 0.4", 0.4, nil
			}
			return "thought\nCONFIDENCE: 0.6", 0.6, nil
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate
	cfg.MinConfidence = 0.7
	cfg.EnableSelfCritique = true
	cfg.MaxIterations = 5

	engine := NewEngine(cfg, reasoner, nil)

	task := Task{
		ID:             "task-critique",
		Description:    "needs refinement",
		Goal:           "test self-critique",
		AvailableTools: []string{"search"},
		SuggestedTool:  "search",
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Equal(t, 2, result.Iterations) // first iteration critiqued, second succeeded
	assert.GreaterOrEqual(t, result.FinalConfidence, 0.7)

	// Should have critique thought from first iteration
	hasCritique := false
	for _, th := range result.Thoughts {
		if th.Type == "critique" {
			hasCritique = true
			break
		}
	}
	assert.True(t, hasCritique, "expected a critique thought")
}

// APPEND_MARKER_4

// --- Event hooks ---

func TestDeliberate_EventHooks(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "ok\nCONFIDENCE: 0.9", 0.9, nil
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate
	cfg.MinConfidence = 0.7

	engine := NewEngine(cfg, reasoner, nil)

	var thoughts []ThoughtProcess
	var decisions []Decision
	engine.OnThought = func(tp ThoughtProcess) {
		thoughts = append(thoughts, tp)
	}
	engine.OnDecision = func(d Decision) {
		decisions = append(decisions, d)
	}

	task := Task{
		ID:          "task-hooks",
		Description: "test hooks",
		Goal:        "verify event emission",
	}

	result, err := engine.Deliberate(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Len(t, thoughts, 3)  // understand, evaluate, plan
	assert.Len(t, decisions, 1) // final decision
}

// --- parseToolSelection ---

func TestParseToolSelection(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		available []string
		suggested string
		want      string
	}{
		{
			name:      "parses TOOL line",
			content:   "reasoning\nTOOL: search\nmore text",
			available: []string{"search", "browse"},
			suggested: "browse",
			want:      "search",
		},
		{
			name:      "case insensitive match",
			content:   "tool: Search",
			available: []string{"search", "browse"},
			suggested: "browse",
			want:      "search",
		},
		{
			name:      "falls back to suggested when tool not in list",
			content:   "TOOL: unknown_tool",
			available: []string{"search", "browse"},
			suggested: "browse",
			want:      "browse",
		},
		{
			name:      "falls back to suggested when no TOOL line",
			content:   "just some reasoning without tool selection",
			available: []string{"search"},
			suggested: "search",
			want:      "search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToolSelection(tt.content, tt.available, tt.suggested)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Reasoner error propagation ---

func TestDeliberate_ReasonerError(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "", 0, fmt.Errorf("provider unavailable")
		},
	}

	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate

	engine := NewEngine(cfg, reasoner, nil)

	task := Task{ID: "task-err", Description: "will fail"}

	_, err := engine.Deliberate(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider unavailable")
}

// --- selectAdaptiveMode unit test ---

func TestSelectAdaptiveMode(t *testing.T) {
	engine := NewEngine(DefaultDeliberationConfig(), &mockReasoner{}, nil)

	t.Run("simple task returns immediate", func(t *testing.T) {
		task := Task{
			AvailableTools: []string{"search"},
			Goal:           "short",
		}
		assert.Equal(t, ModeImmediate, engine.selectAdaptiveMode(task))
	})

	t.Run("complex task returns deliberate", func(t *testing.T) {
		task := Task{
			AvailableTools: []string{"a", "b", "c"},
			Goal:           "short",
		}
		assert.Equal(t, ModeDeliberate, engine.selectAdaptiveMode(task))
	})

	t.Run("long goal adds complexity", func(t *testing.T) {
		task := Task{
			AvailableTools: []string{"a"},
			Goal:           strings.Repeat("x", 101),
		}
		assert.Equal(t, ModeDeliberate, engine.selectAdaptiveMode(task))
	})
}
