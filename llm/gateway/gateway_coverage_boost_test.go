package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/moderation"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmtokenizer "github.com/BaSui01/agentflow/llm/tokenizer"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ═══ Stream: context cancel mid-stream ═══

func TestService_Stream_ContextCancelMidStream(t *testing.T) {
	slowProv := &boostSlowStreamProvider{chunkCount: 1000, delay: time.Millisecond}
	svc := New(Config{ChatProvider: slowProv, Logger: zap.NewNop()})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := svc.Stream(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)

	<-ch
	cancel()

	count := 0
	for range ch {
		count++
		if count > 2000 {
			t.Fatal("stream did not terminate after cancel")
		}
	}
}

// ═══ Stream: no usage in chunks ═══

func TestService_Stream_NoUsageChunks(t *testing.T) {
	noUsageProv := &boostNoUsageStreamProvider{}
	ledger := &recordingLedger{}
	svc := New(Config{ChatProvider: noUsageProv, Ledger: ledger, Logger: zap.NewNop()})

	ch, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)

	for range ch {
	}

	time.Sleep(50 * time.Millisecond)

	entries := ledger.Entries()
	assert.Empty(t, entries, "no ledger entry expected when stream has no usage")
}

// ═══ Stream: error chunk ═══

func TestService_Stream_ErrorChunk(t *testing.T) {
	errProv := &boostErrorChunkStreamProvider{}
	svc := New(Config{ChatProvider: errProv, Logger: zap.NewNop()})

	ch, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)

	chunk := <-ch
	require.NotNil(t, chunk.Err)
	assert.Contains(t, chunk.Err.Message, "upstream error")

	for range ch {
	}
}

// ═══ Stream: invalid payload ═══

func TestService_Stream_InvalidPayload(t *testing.T) {
	mockProv := &gatewayMockChatProvider{}
	svc := New(Config{ChatProvider: mockProv, Logger: zap.NewNop()})

	_, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    "not a ChatRequest",
	})
	require.Error(t, err)
}

// ═══ normalizeCost ═══

func TestNormalizeCost_NegativeAmount(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	cost := svc.normalizeCost(llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{AmountUSD: -5.0})
	assert.Equal(t, 0.0, cost.AmountUSD)
}

func TestNormalizeCost_EmptyCurrency(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	cost := svc.normalizeCost(llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{Currency: ""})
	assert.Equal(t, "USD", cost.Currency)
}

func TestNormalizeCost_NonUSDCurrency(t *testing.T) {
	calc := observability.NewCostCalculator()
	calc.SetPrice("prov", "model", 1.0, 2.0)
	svc := New(Config{CostCalculator: calc, Logger: zap.NewNop()})

	cost := svc.normalizeCost(
		llmcore.ProviderDecision{Provider: "prov", Model: "model"},
		llmcore.Usage{PromptTokens: 100, CompletionTokens: 50},
		llmcore.Cost{AmountUSD: 0, Currency: "CREDITS"},
	)
	assert.Equal(t, 0.0, cost.AmountUSD)
	assert.Equal(t, "CREDITS", cost.Currency)
}

func TestNormalizeCost_WhitespaceCurrency(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	cost := svc.normalizeCost(llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{Currency: "  usd  "})
	assert.Equal(t, "USD", cost.Currency)
}

func TestNormalizeCost_CalculatorFillsZeroUSD(t *testing.T) {
	calc := observability.NewCostCalculator()
	calc.SetPrice("prov", "model", 1.0, 2.0)
	svc := New(Config{CostCalculator: calc, Logger: zap.NewNop()})

	cost := svc.normalizeCost(
		llmcore.ProviderDecision{Provider: "prov", Model: "model"},
		llmcore.Usage{PromptTokens: 1000, CompletionTokens: 500},
		llmcore.Cost{AmountUSD: 0, Currency: "USD"},
	)
	assert.Greater(t, cost.AmountUSD, 0.0)
}

// ═══ normalizeUsage ═══

func TestNormalizeUsage_AllNegative(t *testing.T) {
	usage := normalizeUsage(llmcore.Usage{
		PromptTokens:     -1,
		CompletionTokens: -2,
		TotalTokens:      -3,
		InputUnits:       -4,
		OutputUnits:      -5,
		TotalUnits:       -6,
	})
	assert.Equal(t, 0, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
	assert.Equal(t, 0, usage.TotalTokens)
	assert.Equal(t, 0, usage.InputUnits)
	assert.Equal(t, 0, usage.OutputUnits)
	assert.Equal(t, 0, usage.TotalUnits)
}

func TestNormalizeUsage_TotalUnitsFromInputUnits(t *testing.T) {
	usage := normalizeUsage(llmcore.Usage{
		InputUnits:  5,
		OutputUnits: 0,
		TotalUnits:  0,
	})
	assert.Equal(t, 5, usage.TotalUnits)
}

func TestNormalizeUsage_OutputUnitsFromTotalUnits(t *testing.T) {
	usage := normalizeUsage(llmcore.Usage{
		InputUnits:  0,
		OutputUnits: 0,
		TotalUnits:  7,
	})
	assert.Equal(t, 7, usage.OutputUnits)
}

func TestNormalizeUsage_ZeroAll(t *testing.T) {
	usage := normalizeUsage(llmcore.Usage{})
	assert.Equal(t, 0, usage.TotalTokens)
	assert.Equal(t, 0, usage.TotalUnits)
}

// ═══ cloneTags ═══

func TestCloneTags_NonEmpty(t *testing.T) {
	src := []string{"a", "b", "c"}
	dst := cloneTags(src)
	require.Equal(t, src, dst)
	dst[0] = "x"
	assert.Equal(t, "a", src[0])
}

func TestCloneTags_Empty(t *testing.T) {
	assert.Nil(t, cloneTags(nil))
	assert.Nil(t, cloneTags([]string{}))
}

// ═══ parseFloat ═══

func TestParseFloat_Valid(t *testing.T) {
	assert.Equal(t, 3.14, parseFloat("3.14"))
	assert.Equal(t, 0.0, parseFloat("0"))
	assert.Equal(t, 100.0, parseFloat("100"))
}

func TestParseFloat_Empty(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat(""))
}

func TestParseFloat_Negative(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat("-1.5"))
}

func TestParseFloat_Invalid(t *testing.T) {
	assert.Equal(t, 0.0, parseFloat("abc"))
	assert.Equal(t, 0.0, parseFloat("1.2.3"))
}

// ═══ parseInt ═══

func TestParseInt_Valid(t *testing.T) {
	assert.Equal(t, 42, parseInt("42"))
	assert.Equal(t, 0, parseInt("0"))
}

func TestParseInt_Negative(t *testing.T) {
	assert.Equal(t, 0, parseInt("-5"))
}

func TestParseInt_Invalid(t *testing.T) {
	assert.Equal(t, 0, parseInt("abc"))
	assert.Equal(t, 0, parseInt("3.14"))
}

// ═══ costAmount ═══

func TestCostAmount_Nil(t *testing.T) {
	assert.Equal(t, 0.0, costAmount(nil))
}

func TestCostAmount_NonUSD(t *testing.T) {
	assert.Equal(t, 0.0, costAmount(&llmcore.Cost{AmountUSD: 5.0, Currency: "CREDITS"}))
}

func TestCostAmount_Negative(t *testing.T) {
	assert.Equal(t, 0.0, costAmount(&llmcore.Cost{AmountUSD: -1.0, Currency: "USD"}))
}

func TestCostAmount_ValidUSD(t *testing.T) {
	assert.Equal(t, 1.5, costAmount(&llmcore.Cost{AmountUSD: 1.5, Currency: "USD"}))
}

func TestCostAmount_EmptyCurrency(t *testing.T) {
	assert.Equal(t, 2.0, costAmount(&llmcore.Cost{AmountUSD: 2.0, Currency: ""}))
}

// ═══ messageContents ═══

func TestMessageContents(t *testing.T) {
	msgs := []types.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	contents := messageContents(msgs)
	assert.Equal(t, []string{"hello", "world"}, contents)
}

func TestMessageContents_Empty(t *testing.T) {
	contents := messageContents(nil)
	assert.Empty(t, contents)
}

// ═══ mergeChatRoutingMetadata ═══

func TestMergeChatRoutingMetadata_NilInputs(t *testing.T) {
	mergeChatRoutingMetadata(nil, nil)
	mergeChatRoutingMetadata(nil, &llm.ChatRequest{})
	mergeChatRoutingMetadata(&llmcore.UnifiedRequest{}, nil)
}

func TestMergeChatRoutingMetadata_TagsCopied(t *testing.T) {
	req := &llmcore.UnifiedRequest{Tags: []string{"tag1", "tag2"}}
	chatReq := &llm.ChatRequest{}
	mergeChatRoutingMetadata(req, chatReq)
	assert.Equal(t, []string{"tag1", "tag2"}, chatReq.Tags)
}

func TestMergeChatRoutingMetadata_ExistingTagsNotOverwritten(t *testing.T) {
	req := &llmcore.UnifiedRequest{Tags: []string{"new-tag"}}
	chatReq := &llm.ChatRequest{Tags: []string{"existing-tag"}}
	mergeChatRoutingMetadata(req, chatReq)
	assert.Equal(t, []string{"existing-tag"}, chatReq.Tags)
}

// ═══ normalizeRoutePolicy ═══

func TestNormalizeRoutePolicy_AllVariants(t *testing.T) {
	tests := []struct {
		input string
		want  llmcore.RoutePolicy
	}{
		{"cost", llmcore.RoutePolicyCostFirst},
		{"cost_first", llmcore.RoutePolicyCostFirst},
		{"COST", llmcore.RoutePolicyCostFirst},
		{"health", llmcore.RoutePolicyHealthFirst},
		{"health_first", llmcore.RoutePolicyHealthFirst},
		{"latency", llmcore.RoutePolicyLatencyFirst},
		{"latency_first", llmcore.RoutePolicyLatencyFirst},
		{"balanced", llmcore.RoutePolicyBalanced},
		{"BALANCED", llmcore.RoutePolicyBalanced},
		{"unknown", ""},
		{"", ""},
		{"  cost  ", llmcore.RoutePolicyCostFirst},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeRoutePolicy(tt.input))
		})
	}
}

// ═══ providerHintFromMetadata ═══

func TestProviderHintFromMetadata_Empty(t *testing.T) {
	assert.Equal(t, "", providerHintFromMetadata(nil))
	assert.Equal(t, "", providerHintFromMetadata(map[string]string{}))
}

func TestProviderHintFromMetadata_ChatProvider(t *testing.T) {
	meta := map[string]string{llmcore.MetadataKeyChatProvider: "openai"}
	assert.Equal(t, "openai", providerHintFromMetadata(meta))
}

func TestProviderHintFromMetadata_FallbackKeys(t *testing.T) {
	meta := map[string]string{"provider_hint": "anthropic"}
	assert.Equal(t, "anthropic", providerHintFromMetadata(meta))
}

// ═══ recordResponseUsage ═══

func TestRecordResponseUsage_NilPolicyManager(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	svc.recordResponseUsage(&llmcore.UnifiedRequest{}, &llmcore.UnifiedResponse{})
}

func TestRecordResponseUsage_NilResp(t *testing.T) {
	budgetCfg := llmpolicy.DefaultBudgetConfig()
	budget := llmpolicy.NewTokenBudgetManager(budgetCfg, zap.NewNop())
	manager := llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budget})
	svc := New(Config{PolicyManager: manager, Logger: zap.NewNop()})
	svc.recordResponseUsage(&llmcore.UnifiedRequest{}, nil)
}

// ═══ recordLedger ═══

func TestRecordLedger_NilReq(t *testing.T) {
	ledger := &recordingLedger{}
	svc := New(Config{Ledger: ledger, Logger: zap.NewNop()})
	svc.recordLedger(context.Background(), nil, "trace", llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{})
	assert.Empty(t, ledger.Entries())
}

func TestRecordLedger_ErrorFromLedger(t *testing.T) {
	errLedger := &boostErrorLedger{}
	svc := New(Config{Ledger: errLedger, Logger: zap.NewNop()})
	svc.recordLedger(context.Background(), &llmcore.UnifiedRequest{Capability: llmcore.CapabilityChat}, "trace", llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{})
}

// ═══ estimateRequestTokens ═══

func TestEstimateRequestTokens_NilPayload(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	assert.Equal(t, 0, svc.estimateRequestTokens(nil))
	assert.Equal(t, 0, svc.estimateRequestTokens(&llmcore.UnifiedRequest{}))
}

func TestEstimateRequestTokens_UnknownCapability(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	assert.Equal(t, 0, svc.estimateRequestTokens(&llmcore.UnifiedRequest{
		Capability: "unknown",
		Payload:    "something",
	}))
}

func TestEstimateRequestTokens_ToolsCapability(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 3, messageTokens: 0}
		},
		Logger: zap.NewNop(),
	})

	tokens := svc.estimateRequestTokens(&llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityTools,
		Payload: &ToolsInput{
			Calls: []types.ToolCall{
				{Name: "search", Arguments: []byte(`{"q":"test"}`)},
			},
		},
	})
	assert.Greater(t, tokens, 0)
}

func TestEstimateRequestTokens_ModerationCapability(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 5, messageTokens: 0}
		},
		Logger: zap.NewNop(),
	})

	tokens := svc.estimateRequestTokens(&llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityModeration,
		Payload: &ModerationInput{
			Request: &moderation.ModerationRequest{
				Input: []string{"hello", "world"},
			},
		},
	})
	assert.Greater(t, tokens, 0)
}

func TestEstimateRequestTokens_RerankCapability(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 4, messageTokens: 0}
		},
		Logger: zap.NewNop(),
	})

	tokens := svc.estimateRequestTokens(&llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityRerank,
		Payload: &RerankInput{
			Request: &rerank.RerankRequest{
				Query: "hello",
				Documents: []rerank.Document{
					{Title: "t", Text: "doc text"},
				},
			},
		},
	})
	assert.Greater(t, tokens, 0)
}

// ═══ estimateChatTokens ═══

func TestEstimateChatTokens_WithMaxCompletionTokens(t *testing.T) {
	maxTokens := 500
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 2, messageTokens: 10}
		},
		Logger: zap.NewNop(),
	})

	chatReq := &llm.ChatRequest{
		Model:               "test",
		Messages:            []types.Message{{Role: "user", Content: "hi"}},
		MaxCompletionTokens: &maxTokens,
	}
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{ModelHint: "test"}, chatReq)
	assert.Equal(t, 510, tokens)
}

func TestEstimateChatTokens_NilTokenizer(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return nil
		},
		Logger: zap.NewNop(),
	})

	chatReq := &llm.ChatRequest{
		Model:    "test",
		Messages: []types.Message{{Role: "user", Content: "hi"}},
	}
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{ModelHint: "test"}, chatReq)
	assert.Equal(t, 0, tokens)
}

func TestEstimateChatTokens_WithMaxTokensFallback(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 2, messageTokens: 10}
		},
		Logger: zap.NewNop(),
	})

	chatReq := &llm.ChatRequest{
		Model:     "test",
		Messages:  []types.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 200,
	}
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{ModelHint: "test"}, chatReq)
	assert.Equal(t, 210, tokens)
}

// ═══ countTextsTokens ═══

func TestCountTextsTokens_EmptyTexts(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	assert.Equal(t, 0, svc.countTextsTokens("model", nil))
	assert.Equal(t, 0, svc.countTextsTokens("model", []string{}))
}

func TestCountTextsTokens_WhitespaceOnly(t *testing.T) {
	svc := New(Config{
		TokenizerResolver: func(model string) llmtokenizer.Tokenizer {
			return &stubTokenizer{tokenCountPerText: 5}
		},
		Logger: zap.NewNop(),
	})
	assert.Equal(t, 0, svc.countTextsTokens("model", []string{"  ", "\t", "\n"}))
}

// ═══ SupportsStructuredOutput ═══

func TestChatProviderAdapter_SupportsStructuredOutput_WithFallback(t *testing.T) {
	fb := &boostStructuredOutputProvider{}
	adapter := NewChatProviderAdapter(nil, fb)
	assert.True(t, adapter.SupportsStructuredOutput())
}

// ═══ buildUnifiedChatRequest ═══

func TestBuildUnifiedChatRequest_NilReq(t *testing.T) {
	req := buildUnifiedChatRequest(nil)
	assert.Equal(t, llmcore.CapabilityChat, req.Capability)
}

func TestBuildUnifiedChatRequest_WithMetadata(t *testing.T) {
	chatReq := &llm.ChatRequest{
		Model:   "gpt-4",
		TraceID: "trace-123",
		Metadata: map[string]string{
			llmcore.MetadataKeyChatProvider: "openai",
			"route_policy":                 "cost",
		},
		Tags: []string{"prod"},
	}
	req := buildUnifiedChatRequest(chatReq)
	assert.Equal(t, "openai", req.ProviderHint)
	assert.Equal(t, "gpt-4", req.ModelHint)
	assert.Equal(t, llmcore.RoutePolicyCostFirst, req.RoutePolicy)
	assert.Equal(t, "trace-123", req.TraceID)
	assert.Equal(t, []string{"prod"}, req.Tags)
}

// ═══ Mock providers ═══

type boostSlowStreamProvider struct {
	chunkCount int
	delay      time.Duration
}

func (p *boostSlowStreamProvider) Name() string { return "slow-stream" }
func (p *boostSlowStreamProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}
func (p *boostSlowStreamProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 10)
	go func() {
		defer close(ch)
		for i := 0; i < p.chunkCount; i++ {
			time.Sleep(p.delay)
			ch <- llm.StreamChunk{Delta: types.Message{Content: "x"}}
		}
	}()
	return ch, nil
}
func (p *boostSlowStreamProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *boostSlowStreamProvider) SupportsNativeFunctionCalling() bool                { return false }
func (p *boostSlowStreamProvider) ListModels(_ context.Context) ([]llm.Model, error)  { return nil, nil }
func (p *boostSlowStreamProvider) Endpoints() llm.ProviderEndpoints                   { return llm.ProviderEndpoints{} }

type boostNoUsageStreamProvider struct{}

func (p *boostNoUsageStreamProvider) Name() string { return "no-usage-stream" }
func (p *boostNoUsageStreamProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}
func (p *boostNoUsageStreamProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 2)
	ch <- llm.StreamChunk{Delta: types.Message{Content: "hi"}}
	ch <- llm.StreamChunk{Delta: types.Message{Content: " there"}}
	close(ch)
	return ch, nil
}
func (p *boostNoUsageStreamProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *boostNoUsageStreamProvider) SupportsNativeFunctionCalling() bool                { return false }
func (p *boostNoUsageStreamProvider) ListModels(_ context.Context) ([]llm.Model, error)  { return nil, nil }
func (p *boostNoUsageStreamProvider) Endpoints() llm.ProviderEndpoints                   { return llm.ProviderEndpoints{} }

type boostErrorChunkStreamProvider struct{}

func (p *boostErrorChunkStreamProvider) Name() string { return "error-chunk-stream" }
func (p *boostErrorChunkStreamProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}
func (p *boostErrorChunkStreamProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Err: &types.Error{Code: types.ErrUpstreamError, Message: "upstream error"}}
	close(ch)
	return ch, nil
}
func (p *boostErrorChunkStreamProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *boostErrorChunkStreamProvider) SupportsNativeFunctionCalling() bool                { return false }
func (p *boostErrorChunkStreamProvider) ListModels(_ context.Context) ([]llm.Model, error)  { return nil, nil }
func (p *boostErrorChunkStreamProvider) Endpoints() llm.ProviderEndpoints                   { return llm.ProviderEndpoints{} }

type boostErrorLedger struct{}

func (l *boostErrorLedger) Record(_ context.Context, _ observability.LedgerEntry) error {
	return errors.New("ledger write failed")
}

type boostStructuredOutputProvider struct {
	mockFallbackProvider
}

func (p *boostStructuredOutputProvider) SupportsStructuredOutput() bool { return true }
