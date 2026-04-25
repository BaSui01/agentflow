package adapters

import (
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultChatRequestAdapter_BuildMapsFormalModelFields(t *testing.T) {
	maxCompletionTokens := 512
	frequencyPenalty := float32(0.2)
	presencePenalty := float32(0.3)
	repetitionPenalty := float32(1.1)
	n := 2
	logProbs := true
	topLogProbs := 3
	serviceTier := "priority"
	store := true
	includeServerSide := true
	thinkingBudget := int32(-1)
	includeThoughts := true
	adapter := NewDefaultChatRequestAdapter()

	req, err := adapter.Build(types.ExecutionOptions{
		Model: types.ModelOptions{
			Provider:             "openai",
			Model:                "gpt-5.4",
			RoutePolicy:          "latency_first",
			MaxTokens:            1024,
			MaxCompletionTokens:  &maxCompletionTokens,
			Temperature:          0.2,
			TopP:                 0.9,
			Stop:                 []string{"STOP"},
			FrequencyPenalty:     &frequencyPenalty,
			PresencePenalty:      &presencePenalty,
			RepetitionPenalty:    &repetitionPenalty,
			N:                    &n,
			LogProbs:             &logProbs,
			TopLogProbs:          &topLogProbs,
			User:                 " user-1 ",
			StreamOptions:        &types.StreamOptions{IncludeUsage: true, ChunkIncludeUsage: true},
			ServiceTier:          &serviceTier,
			ReasoningEffort:      "medium",
			ReasoningSummary:     "auto",
			ReasoningDisplay:     "redacted",
			ReasoningMode:        "thinking",
			ThinkingType:         " adaptive ",
			ThinkingLevel:        " high ",
			ThinkingBudget:       &thinkingBudget,
			IncludeThoughts:      &includeThoughts,
			MediaResolution:      " media_resolution_high ",
			InferenceSpeed:       "fast",
			Store:                &store,
			Modalities:           []string{"text", "audio"},
			PromptCacheKey:       " cache-key ",
			PromptCacheRetention: " 24h ",
			CacheControl:         &types.CacheControl{Type: "ephemeral", TTL: "5m"},
			CachedContent:        " cachedContents/session-1 ",
			Include:              []string{"reasoning.encrypted_content"},
			Truncation:           " auto ",
			PreviousResponseID:   " resp_prev_1 ",
			ConversationID:       " conv_1 ",
			ThoughtSignatures:    []string{"sig-1"},
			Verbosity:            " low ",
			Phase:                " commentary ",
			WebSearchOptions:     &types.WebSearchOptions{SearchContextSize: "medium", AllowedDomains: []string{"example.com"}},
		},
		Control: types.AgentControlOptions{Timeout: 5 * time.Second},
		Tools: types.ToolProtocolOptions{
			ToolChoice: &types.ToolChoice{
				Mode:                             types.ToolChoiceModeSpecific,
				ToolName:                         "search",
				IncludeServerSideToolInvocations: &includeServerSide,
			},
			ParallelToolCalls: &store,
			ToolCallMode:      types.ToolCallModeNative,
		},
		Metadata: map[string]string{"request": "r1"},
		Tags:     []string{"tag-a"},
	}, []types.Message{{Role: types.RoleUser, Content: "hello"}})

	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Equal(t, "gpt-5.4", req.Model)
	assert.Equal(t, "latency_first", req.RoutePolicy)
	assert.Equal(t, 1024, req.MaxTokens)
	require.NotNil(t, req.MaxCompletionTokens)
	assert.Equal(t, 512, *req.MaxCompletionTokens)
	require.NotNil(t, req.FrequencyPenalty)
	assert.Equal(t, float32(0.2), *req.FrequencyPenalty)
	require.NotNil(t, req.PresencePenalty)
	assert.Equal(t, float32(0.3), *req.PresencePenalty)
	require.NotNil(t, req.RepetitionPenalty)
	assert.Equal(t, float32(1.1), *req.RepetitionPenalty)
	require.NotNil(t, req.N)
	assert.Equal(t, 2, *req.N)
	require.NotNil(t, req.LogProbs)
	assert.True(t, *req.LogProbs)
	require.NotNil(t, req.TopLogProbs)
	assert.Equal(t, 3, *req.TopLogProbs)
	assert.Equal(t, "user-1", req.User)
	require.NotNil(t, req.StreamOptions)
	assert.True(t, req.StreamOptions.IncludeUsage)
	assert.True(t, req.StreamOptions.ChunkIncludeUsage)
	require.NotNil(t, req.ServiceTier)
	assert.Equal(t, "priority", *req.ServiceTier)
	assert.Equal(t, "medium", req.ReasoningEffort)
	assert.Equal(t, "auto", req.ReasoningSummary)
	assert.Equal(t, "redacted", req.ReasoningDisplay)
	assert.Equal(t, "thinking", req.ReasoningMode)
	assert.Equal(t, "adaptive", req.ThinkingType)
	assert.Equal(t, "high", req.ThinkingLevel)
	require.NotNil(t, req.ThinkingBudget)
	assert.Equal(t, int32(-1), *req.ThinkingBudget)
	require.NotNil(t, req.IncludeThoughts)
	assert.True(t, *req.IncludeThoughts)
	assert.Equal(t, "media_resolution_high", req.MediaResolution)
	assert.Equal(t, "fast", req.InferenceSpeed)
	require.NotNil(t, req.Store)
	assert.True(t, *req.Store)
	assert.Equal(t, []string{"text", "audio"}, req.Modalities)
	assert.Equal(t, "cache-key", req.PromptCacheKey)
	assert.Equal(t, "24h", req.PromptCacheRetention)
	require.NotNil(t, req.CacheControl)
	assert.Equal(t, "ephemeral", req.CacheControl.Type)
	assert.Equal(t, "cachedContents/session-1", req.CachedContent)
	assert.Equal(t, []string{"reasoning.encrypted_content"}, req.Include)
	assert.Equal(t, "auto", req.Truncation)
	assert.Equal(t, "resp_prev_1", req.PreviousResponseID)
	assert.Equal(t, "conv_1", req.ConversationID)
	assert.Equal(t, []string{"sig-1"}, req.ThoughtSignatures)
	assert.Equal(t, "low", req.Verbosity)
	assert.Equal(t, "commentary", req.Phase)
	require.NotNil(t, req.WebSearchOptions)
	assert.Equal(t, []string{"example.com"}, req.WebSearchOptions.AllowedDomains)
	assert.Equal(t, "openai", req.Metadata[llmcore.MetadataKeyChatProvider])
	assert.Equal(t, "r1", req.Metadata["request"])
	assert.Equal(t, []string{"tag-a"}, req.Tags)
	require.NotNil(t, req.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeSpecific, req.ToolChoice.Mode)
	assert.Equal(t, "search", req.ToolChoice.ToolName)
	require.NotNil(t, req.ToolChoice.IncludeServerSideToolInvocations)
	assert.True(t, *req.ToolChoice.IncludeServerSideToolInvocations)
	require.NotNil(t, req.ParallelToolCalls)
	assert.True(t, *req.ParallelToolCalls)
	assert.Equal(t, types.ToolCallModeNative, req.ToolCallMode)
}
