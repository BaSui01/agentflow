package context_test

import (
	stdctx "context"
	"strings"
	"testing"

	agentcontext "github.com/BaSui01/agentflow/agent/context"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func TestEngineer_Manage_NormalCompress_TruncatesToolContent(t *testing.T) {
	t.Parallel()

	e := agentcontext.New(agentcontext.Config{
		MaxContextTokens: 1000,
		ReserveForOutput: 0,
		SoftLimit:        0.0,
		WarnLimit:        0.85,
		HardLimit:        0.95,
		TargetUsage:      0.5,
		Strategy:         agentcontext.StrategyAdaptive,
	}, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleTool, Content: strings.Repeat("x", 3000)},
	}

	out, err := e.Manage(stdctx.Background(), msgs, "")
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if !strings.Contains(out[0].Content, "...[truncated]") {
		t.Fatalf("expected tool content truncated, got %q", out[0].Content)
	}
}

func TestEngineer_Manage_EmergencyCompress_AddsSummaryAndPreservesRecent(t *testing.T) {
	t.Parallel()

	e := agentcontext.New(agentcontext.Config{
		MaxContextTokens: 200,
		ReserveForOutput: 0,
		SoftLimit:        0.1,
		WarnLimit:        0.2,
		HardLimit:        0.3,
		TargetUsage:      0.5,
		Strategy:         agentcontext.StrategyAdaptive,
	}, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleSystem, Content: strings.Repeat("s", 2000)},
		{Role: types.RoleUser, Content: strings.Repeat("u1", 300)},
		{Role: types.RoleAssistant, Content: strings.Repeat("a1", 300)},
		{Role: types.RoleUser, Content: strings.Repeat("u2", 300)},
		{Role: types.RoleAssistant, Content: strings.Repeat("a2", 300)},
		{Role: types.RoleUser, Content: strings.Repeat("u3", 300)},
	}

	out, err := e.Manage(stdctx.Background(), msgs, "q")
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 messages (system + summary + 2 recent), got %d", len(out))
	}
	if out[0].Role != types.RoleSystem {
		t.Fatalf("expected first message role system, got %s", out[0].Role)
	}
	if out[1].Role != types.RoleSystem || !strings.HasPrefix(out[1].Content, "[Emergency Summary") {
		t.Fatalf("expected emergency summary message, got role=%s content=%q", out[1].Role, out[1].Content)
	}

	st := e.GetStats()
	if st.TotalCompressions == 0 || st.EmergencyCount != 1 {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

func TestEngineer_MustFit_ReturnsMessagesWithinBudget(t *testing.T) {
	t.Parallel()

	cfg := agentcontext.Config{
		MaxContextTokens: 160,
		ReserveForOutput: 0,
		SoftLimit:        0.1,
		WarnLimit:        0.2,
		HardLimit:        0.3,
		TargetUsage:      0.5,
		Strategy:         agentcontext.StrategyAdaptive,
	}
	e := agentcontext.New(cfg, zap.NewNop())

	msgs := []types.Message{
		{Role: types.RoleSystem, Content: strings.Repeat("s", 2000)},
		{Role: types.RoleUser, Content: strings.Repeat("u", 3000)},
		{Role: types.RoleAssistant, Content: strings.Repeat("a", 3000)},
		{Role: types.RoleUser, Content: strings.Repeat("u", 3000)},
	}

	out, err := e.MustFit(stdctx.Background(), msgs, "q")
	if err != nil {
		t.Fatalf("MustFit: %v", err)
	}

	maxTokens := cfg.MaxContextTokens - cfg.ReserveForOutput
	if got := types.NewEstimateTokenizer().CountMessagesTokens(out); got > maxTokens {
		t.Fatalf("expected tokens <= %d, got %d", maxTokens, got)
	}
}

func TestDefaultAgentContextConfig(t *testing.T) {
	t.Parallel()

	cfg := agentcontext.DefaultAgentContextConfig("gpt-4o")
	if cfg.MaxContextTokens != 128000 || cfg.ReserveForOutput != 4096 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
