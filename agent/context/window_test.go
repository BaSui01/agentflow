package context

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockTokenCounter struct{}

func (m *mockTokenCounter) CountTokens(text string) int {
	return len(text) / 4
}

type mockSummarizer struct {
	result string
	err    error
}

func (m *mockSummarizer) Summarize(_ context.Context, msgs []types.Message) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

// --- helpers ---

func msg(role types.Role, content string) types.Message {
	return types.Message{Role: role, Content: content}
}

func userMsg(content string) types.Message   { return msg(types.RoleUser, content) }
func assistMsg(content string) types.Message { return msg(types.RoleAssistant, content) }
func sysMsg(content string) types.Message    { return msg(types.RoleSystem, content) }

func TestWindowManager_SlidingWindow_KeepsLastN(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:    StrategySlidingWindow,
		MaxMessages: 3,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("m1"), assistMsg("m2"), userMsg("m3"),
		assistMsg("m4"), userMsg("m5"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 3)
	assert.Equal(t, "m3", out[0].Content)
	assert.Equal(t, "m4", out[1].Content)
	assert.Equal(t, "m5", out[2].Content)
}

func TestWindowManager_SlidingWindow_KeepsSystemMsg(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategySlidingWindow,
		MaxMessages:   2,
		KeepSystemMsg: true,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		sysMsg("system prompt"),
		userMsg("m1"), assistMsg("m2"), userMsg("m3"), assistMsg("m4"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 3) // system + last 2
	assert.Equal(t, types.RoleSystem, out[0].Role)
	assert.Equal(t, "m3", out[1].Content)
	assert.Equal(t, "m4", out[2].Content)
}

func TestWindowManager_TokenBudget_TrimsByTokens(t *testing.T) {
	t.Parallel()
	// Each message: content tokens + 4 overhead.
	// "hello" = 5 chars / 4 = 1 token, + 4 overhead = 5 tokens per msg.
	wm := NewWindowManager(WindowConfig{
		Strategy:  StrategyTokenBudget,
		MaxTokens: 15, // room for ~3 messages
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("hello"), assistMsg("hello"), userMsg("hello"),
		assistMsg("hello"), userMsg("hello"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 3)
	// Should keep the last 3 messages
	assert.Equal(t, "hello", out[0].Content)
	assert.Equal(t, types.RoleAssistant, out[1].Role)
	assert.Equal(t, types.RoleUser, out[2].Role)
}

func TestWindowManager_TokenBudget_ReserveTokens(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategyTokenBudget,
		MaxTokens:     20,
		ReserveTokens: 10, // effective budget = 10
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("hello"), assistMsg("hello"), userMsg("hello"),
		assistMsg("hello"), userMsg("hello"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 2) // budget=10, each msg ~5 tokens
}

func TestWindowManager_TokenBudget_KeepsSystemMsg(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategyTokenBudget,
		MaxTokens:     15,
		KeepSystemMsg: true,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		sysMsg("sys"),
		userMsg("hello"), assistMsg("hello"), userMsg("hello"),
		assistMsg("hello"), userMsg("hello"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Equal(t, types.RoleSystem, out[0].Role)
	// System msg takes some budget, rest goes to recent messages
	assert.True(t, len(out) >= 2)
}

func TestWindowManager_Summarize_WithMockSummarizer(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategySummarize,
		MaxTokens:     20,
		KeepSystemMsg: true,
		KeepLastN:     2,
	}, &mockTokenCounter{}, &mockSummarizer{result: "summary of old messages"})

	messages := []types.Message{
		sysMsg("system"),
		userMsg(strings.Repeat("a", 100)),
		assistMsg(strings.Repeat("b", 100)),
		userMsg("recent1"),
		assistMsg("recent2"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	// system + summary + last 2
	require.Len(t, out, 4)
	assert.Equal(t, types.RoleSystem, out[0].Role)
	assert.Equal(t, "summary of old messages", out[1].Content)
	assert.Equal(t, types.RoleAssistant, out[1].Role)
	assert.Equal(t, "recent1", out[2].Content)
	assert.Equal(t, "recent2", out[3].Content)
}

func TestWindowManager_Summarize_NilSummarizer_FallsBackToTokenBudget(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:  StrategySummarize,
		MaxTokens: 15,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("hello"), assistMsg("hello"), userMsg("hello"),
		assistMsg("hello"), userMsg("hello"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	// Falls back to token budget behavior
	assert.True(t, len(out) <= 3)
}

func TestWindowManager_Summarize_ErrorFallsBackToTokenBudget(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategySummarize,
		MaxTokens:     20,
		KeepSystemMsg: true,
		KeepLastN:     2,
	}, &mockTokenCounter{}, &mockSummarizer{err: errors.New("llm unavailable")})

	messages := []types.Message{
		sysMsg("system"),
		userMsg(strings.Repeat("a", 100)),
		assistMsg(strings.Repeat("b", 100)),
		userMsg("recent1"),
		assistMsg("recent2"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	// Should not contain the summary, just token-budget trimmed
	for _, m := range out {
		assert.NotEqual(t, "summary of old messages", m.Content)
	}
}

func TestWindowManager_NoTrimWhenUnderLimit(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:    StrategySlidingWindow,
		MaxMessages: 10,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("m1"), assistMsg("m2"), userMsg("m3"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 3)
	assert.Equal(t, "m1", out[0].Content)
	assert.Equal(t, "m2", out[1].Content)
	assert.Equal(t, "m3", out[2].Content)
}

func TestWindowManager_EstimateTokens(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("hello world!"), // 12 chars / 4 = 3 tokens + 4 overhead = 7
		assistMsg("hi"),         // 2 chars / 4 = 0, min 0 + 4 overhead = 4
	}

	total := wm.EstimateTokens(messages)
	assert.Equal(t, 11, total)
}

func TestWindowManager_GetStatus(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		MaxTokens:     1000,
		ReserveTokens: 200,
	}, &mockTokenCounter{}, nil)

	messages := []types.Message{
		userMsg("hello"), assistMsg("world"),
	}

	status := wm.GetStatus(messages)
	ws, ok := status.(WindowStatus)
	require.True(t, ok)
	assert.Equal(t, 2, ws.MessageCount)
	assert.Equal(t, 800, ws.MaxTokens)
	assert.Equal(t, wm.EstimateTokens(messages), ws.TotalTokens)
	assert.False(t, ws.Trimmed)
}

func TestWindowManager_NilTokenCounter_UsesDefaultEstimation(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:  StrategyTokenBudget,
		MaxTokens: 100,
	}, nil, nil) // nil tokenCounter

	messages := []types.Message{
		userMsg("hello"), assistMsg("world"),
	}

	total := wm.EstimateTokens(messages)
	assert.Greater(t, total, 0)

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestWindowManager_EmptyMessages(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:  StrategySlidingWindow,
		MaxTokens: 100,
	}, &mockTokenCounter{}, nil)

	out, err := wm.PrepareMessages(context.Background(), nil, "")
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = wm.PrepareMessages(context.Background(), []types.Message{}, "")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestWindowManager_Summarize_WithinBudget_NoTrim(t *testing.T) {
	t.Parallel()
	wm := NewWindowManager(WindowConfig{
		Strategy:      StrategySummarize,
		MaxTokens:     10000,
		KeepSystemMsg: true,
		KeepLastN:     2,
	}, &mockTokenCounter{}, &mockSummarizer{result: "should not appear"})

	messages := []types.Message{
		sysMsg("system"),
		userMsg("hello"),
		assistMsg("world"),
	}

	out, err := wm.PrepareMessages(context.Background(), messages, "")
	require.NoError(t, err)
	assert.Len(t, out, 3)
	for _, m := range out {
		assert.NotEqual(t, "should not appear", m.Content)
	}
}
