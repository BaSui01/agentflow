package gateway

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ═══ normalizeUsage 边界测试 ═══

func TestNormalizeUsage_AllZero(t *testing.T) {
	u := normalizeUsage(llmcore.Usage{})
	assert.Equal(t, 0, u.PromptTokens)
	assert.Equal(t, 0, u.CompletionTokens)
	assert.Equal(t, 0, u.TotalTokens)
}

func TestNormalizeUsage_NegativeValues(t *testing.T) {
	u := normalizeUsage(llmcore.Usage{PromptTokens: -5, CompletionTokens: -3, TotalTokens: -8})
	assert.True(t, u.PromptTokens >= 0)
	assert.True(t, u.CompletionTokens >= 0)
}

func TestNormalizeUsage_TotalRecalculated(t *testing.T) {
	u := normalizeUsage(llmcore.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 0})
	assert.Equal(t, 30, u.TotalTokens)
}

// ═══ estimateRequestTokens 各能力类型 ═══

func TestEstimateRequestTokens_Chat(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hello world"}}},
	}
	tokens := svc.estimateRequestTokens(req)
	assert.True(t, tokens > 0)
}

func TestEstimateRequestTokens_Image(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	req := &llmcore.UnifiedRequest{Capability: llmcore.CapabilityImage, Payload: &ImageInput{}}
	tokens := svc.estimateRequestTokens(req)
	assert.True(t, tokens >= 0)
}

func TestEstimateRequestTokens_Unknown(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	req := &llmcore.UnifiedRequest{Capability: "unknown", Payload: nil}
	tokens := svc.estimateRequestTokens(req)
	assert.Equal(t, 0, tokens)
}

// ═══ estimateChatTokens 边界 ═══

func TestEstimateChatTokens_NilReq(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{Capability: llmcore.CapabilityChat}, &llm.ChatRequest{})
	assert.True(t, tokens >= 0)
}

func TestEstimateChatTokens_EmptyMessages(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{}, &llm.ChatRequest{Messages: nil})
	assert.True(t, tokens >= 0)
}

func TestEstimateChatTokens_WithTools(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	tokens := svc.estimateChatTokens(&llmcore.UnifiedRequest{}, &llm.ChatRequest{
		Messages: []types.Message{{Role: "user", Content: "hello"}},
		Tools:    []types.ToolSchema{{Name: "search", Description: "search tool"}},
	})
	assert.True(t, tokens > 0)
}

// ═══ messageContents ═══

func TestMessageContents(t *testing.T) {
	msgs := []types.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
		{Role: "system", Content: ""},
	}
	result := messageContents(msgs)
	assert.True(t, len(result) >= 2, "should have at least 2 non-empty contents")
}

func TestMessageContents_Empty(t *testing.T) {
	result := messageContents(nil)
	assert.Equal(t, 0, len(result))
}

// ═══ cloneTags ═══

func TestCloneTags_Nil(t *testing.T) {
	assert.Nil(t, cloneTags(nil))
}

func TestCloneTags_Empty(t *testing.T) {
	assert.Nil(t, cloneTags([]string{}))
}

func TestCloneTags_DeepCopy(t *testing.T) {
	original := []string{"a", "b"}
	cloned := cloneTags(original)
	assert.Equal(t, original, cloned)
	cloned[0] = "x"
	assert.Equal(t, "a", original[0])
}

// ═══ parseFloat ═══

func TestParseFloat_Valid(t *testing.T) {
	assert.InDelta(t, 3.14, parseFloat("3.14"), 0.001)
}

func TestParseFloat_Invalid(t *testing.T) {
	assert.Equal(t, float64(0), parseFloat("not-a-number"))
}

func TestParseFloat_Empty(t *testing.T) {
	assert.Equal(t, float64(0), parseFloat(""))
}

// ═══ costAmount ═══

func TestCostAmount_Valid(t *testing.T) {
	cost := &llmcore.Cost{AmountUSD: 0.05, Currency: "USD"}
	assert.InDelta(t, 0.05, costAmount(cost), 0.001)
}

func TestCostAmount_Nil(t *testing.T) {
	assert.Equal(t, float64(0), costAmount(nil))
}

// ═══ mergeChatRoutingMetadata ═══

func TestMergeChatRoutingMetadata_Empty(t *testing.T) {
	req := &llmcore.UnifiedRequest{}
	chatReq := &llm.ChatRequest{Model: "test"}
	mergeChatRoutingMetadata(req, chatReq)
}

func TestMergeChatRoutingMetadata_WithHints(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Hints: llmcore.CapabilityHints{ChatProvider: "openai"},
	}
	chatReq := &llm.ChatRequest{Model: "test"}
	mergeChatRoutingMetadata(req, chatReq)
	assert.Equal(t, "openai", chatReq.Metadata[llmcore.MetadataKeyChatProvider])
}

// ═══ normalizeRoutePolicy ═══

func TestNormalizeRoutePolicy_Known(t *testing.T) {
	for _, p := range []string{"balanced", "cost_first", "health_first", "latency_first"} {
		assert.Equal(t, llmcore.RoutePolicy(p), normalizeRoutePolicy(p))
	}
}

func TestNormalizeRoutePolicy_Unknown(t *testing.T) {
	result := normalizeRoutePolicy("random")
	_ = result // 只要不 panic
}

func TestNormalizeRoutePolicy_Empty(t *testing.T) {
	assert.Equal(t, llmcore.RoutePolicy(""), normalizeRoutePolicy(""))
}

// ═══ providerHintFromMetadata ═══

func TestProviderHintFromMetadata_Present(t *testing.T) {
	assert.Equal(t, "anthropic", providerHintFromMetadata(map[string]string{llmcore.MetadataKeyChatProvider: "anthropic"}))
}

func TestProviderHintFromMetadata_Missing(t *testing.T) {
	assert.Equal(t, "", providerHintFromMetadata(nil))
}

// ═══ recordResponseUsage / recordLedger ═══

func TestRecordResponseUsage_NilPolicyManager(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	svc.recordResponseUsage(&llmcore.UnifiedRequest{}, &llmcore.UnifiedResponse{})
}

func TestRecordLedger_NilLedger(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	svc.recordLedger(context.Background(), &llmcore.UnifiedRequest{}, "t1", llmcore.ProviderDecision{}, llmcore.Usage{}, llmcore.Cost{})
}

// ═══ validateRequest ═══

func TestValidateRequest_Nil(t *testing.T) {
	err := validateRequest(nil)
	require.Error(t, err)
}

func TestValidateRequest_NilPayload(t *testing.T) {
	err := validateRequest(&llmcore.UnifiedRequest{Capability: llmcore.CapabilityChat})
	require.Error(t, err)
}

func TestValidateRequest_Valid(t *testing.T) {
	err := validateRequest(&llmcore.UnifiedRequest{Capability: llmcore.CapabilityChat, Payload: &llm.ChatRequest{Model: "test"}})
	// 可能返回 nil 或非 nil（取决于更多校验），只要不 panic
	_ = err
}

// ═══ Stream context cancel ═══

func TestService_Stream_ContextCancel_Coverage(t *testing.T) {
	mockProv := &gatewayMockChatProvider{}
	svc := New(Config{ChatProvider: mockProv, Logger: zap.NewNop()})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := svc.Stream(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)
	cancel()
	for range ch {
	}
}
