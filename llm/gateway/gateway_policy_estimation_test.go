package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmtokenizer "github.com/BaSui01/agentflow/llm/tokenizer"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPreflightPolicy_EstimatesChatTokens(t *testing.T) {
	service := newPolicyTestService(t, 30, &stubTokenizer{
		tokenCountPerText: 2,
		messageTokens:     16,
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "hello world"},
			},
			MaxTokens: 20,
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "per-request")
	require.Equal(t, "36", req.Metadata["estimated_tokens"])
}

func TestPreflightPolicy_UsesMetadataEstimatedTokens(t *testing.T) {
	service := newPolicyTestService(t, 5, &stubTokenizer{
		tokenCountPerText: 50,
		messageTokens:     100,
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Metadata: map[string]string{
			"estimated_tokens": "1",
		},
		Payload: &llm.ChatRequest{
			Model: "test-model",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "very long message that should be ignored by override"},
			},
			MaxTokens: 100,
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "1", req.Metadata["estimated_tokens"])
}

func TestPreflightPolicy_EstimatesEmbeddingTokens(t *testing.T) {
	service := newPolicyTestService(t, 3, &stubTokenizer{
		tokenCountPerText: 2,
		messageTokens:     0,
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityEmbedding,
		Payload: &EmbeddingInput{
			Request: &embedding.EmbeddingRequest{
				Model: "test-embedding-model",
				Input: []string{"foo", "bar"},
			},
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, "4", req.Metadata["estimated_tokens"])
}

func newPolicyTestService(t *testing.T, maxTokensPerRequest int, tok llmtokenizer.Tokenizer) *Service {
	t.Helper()

	cfg := llmpolicy.DefaultBudgetConfig()
	cfg.MaxTokensPerRequest = maxTokensPerRequest
	cfg.MaxTokensPerMinute = 1_000_000
	cfg.MaxTokensPerHour = 1_000_000
	cfg.MaxTokensPerDay = 1_000_000
	cfg.MaxCostPerRequest = 1_000_000
	cfg.MaxCostPerDay = 1_000_000

	budget := llmpolicy.NewTokenBudgetManager(cfg, zap.NewNop())
	manager := llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budget})

	return New(Config{
		PolicyManager: manager,
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return tok
		},
	})
}

type stubTokenizer struct {
	tokenCountPerText int
	messageTokens     int
}

func (s *stubTokenizer) CountTokens(text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	return s.tokenCountPerText, nil
}

func (s *stubTokenizer) CountMessages(messages []llmtokenizer.Message) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	return s.messageTokens, nil
}

func (s *stubTokenizer) Encode(text string) ([]int, error) {
	return nil, nil
}

func (s *stubTokenizer) Decode(tokens []int) (string, error) {
	return "", nil
}

func (s *stubTokenizer) MaxTokens() int {
	return 128000
}

func (s *stubTokenizer) Name() string {
	return "stub-tokenizer"
}
