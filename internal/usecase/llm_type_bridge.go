package usecase

import (
	"strings"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// LLMTypeBridge - LLM Layer Type Bridge Service
// =============================================================================
// This service encapsulates type conversion logic between handlers layer and
// LLM layer, allowing handlers to avoid direct imports from llm/core.
//
// Architecture compliance:
// - handlers layer -> usecase layer (via LLMTypeBridge interface)
// - usecase layer -> llm/core layer (implementation)
// =============================================================================

// LLMTypeBridge provides type conversion between handlers and LLM layers.
// Handlers should use this interface instead of directly importing llm/core types.
type LLMTypeBridge interface {
	// WebSearchOptions conversions
	ToLLMWebSearchOptions(opts *WebSearchOptions) *llmcore.WebSearchOptions
	FromLLMWebSearchOptions(opts *llmcore.WebSearchOptions) *WebSearchOptions
	MergeLLMWebSearchOptions(base, override *llmcore.WebSearchOptions) *llmcore.WebSearchOptions

	// API Key conversions
	ToLLMProviderAPIKey(providerID uint, input CreateAPIKeyInput) llmcore.LLMProviderAPIKey
	FromLLMProviderAPIKey(key llmcore.LLMProviderAPIKey) APIKeyView
	FromLLMProviderAPIKeys(keys []llmcore.LLMProviderAPIKey) []APIKeyView
	FromLLMProviderAPIKeyStats(keys []llmcore.LLMProviderAPIKey) []APIKeyStatsView

	// Provider conversions
	FromLLMProviders(providers []llmcore.LLMProvider) []ProviderView

	// Chat request/response conversions (delegated to ChatConverter)
	// These methods are provided for handlers that need direct LLM type access
	ToLLMChatRequest(req *ChatRequest) *types.ChatRequest
	FromLLMChatResponse(resp *types.ChatResponse) *ChatResponse
}

// ProviderView is the DTO for LLM provider information.
type ProviderView struct {
	ID          uint   `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DefaultLLMTypeBridge is the default implementation of LLMTypeBridge.
type DefaultLLMTypeBridge struct {
	chatConverter ChatConverter
}

// NewLLMTypeBridge creates a new LLMTypeBridge instance.
func NewLLMTypeBridge(chatConverter ChatConverter) LLMTypeBridge {
	return &DefaultLLMTypeBridge{
		chatConverter: chatConverter,
	}
}

// =============================================================================
// WebSearchOptions Conversions
// =============================================================================

// ToLLMWebSearchOptions converts usecase WebSearchOptions to llm/core WebSearchOptions.
func (b *DefaultLLMTypeBridge) ToLLMWebSearchOptions(opts *WebSearchOptions) *llmcore.WebSearchOptions {
	if opts == nil {
		return nil
	}
	out := &llmcore.WebSearchOptions{
		SearchContextSize: strings.TrimSpace(opts.SearchContextSize),
		AllowedDomains:    normalizeWebSearchDomains(opts.AllowedDomains),
		BlockedDomains:    normalizeWebSearchDomains(opts.BlockedDomains),
		MaxUses:           opts.MaxUses,
	}
	if opts.UserLocation != nil {
		out.UserLocation = &llmcore.WebSearchLocation{
			Type:     strings.TrimSpace(opts.UserLocation.Type),
			Country:  strings.TrimSpace(opts.UserLocation.Country),
			Region:   strings.TrimSpace(opts.UserLocation.Region),
			City:     strings.TrimSpace(opts.UserLocation.City),
			Timezone: strings.TrimSpace(opts.UserLocation.Timezone),
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

// FromLLMWebSearchOptions converts llm/core WebSearchOptions to usecase WebSearchOptions.
func (b *DefaultLLMTypeBridge) FromLLMWebSearchOptions(opts *llmcore.WebSearchOptions) *WebSearchOptions {
	if opts == nil {
		return nil
	}
	out := &WebSearchOptions{
		SearchContextSize: strings.TrimSpace(opts.SearchContextSize),
		AllowedDomains:    normalizeWebSearchDomains(opts.AllowedDomains),
		BlockedDomains:    normalizeWebSearchDomains(opts.BlockedDomains),
		MaxUses:           opts.MaxUses,
	}
	if opts.UserLocation != nil {
		out.UserLocation = &WebSearchLocation{
			Type:     strings.TrimSpace(opts.UserLocation.Type),
			Country:  strings.TrimSpace(opts.UserLocation.Country),
			Region:   strings.TrimSpace(opts.UserLocation.Region),
			City:     strings.TrimSpace(opts.UserLocation.City),
			Timezone: strings.TrimSpace(opts.UserLocation.Timezone),
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

// MergeLLMWebSearchOptions merges two llm/core WebSearchOptions, with override taking precedence.
func (b *DefaultLLMTypeBridge) MergeLLMWebSearchOptions(base, override *llmcore.WebSearchOptions) *llmcore.WebSearchOptions {
	if base == nil && override == nil {
		return nil
	}
	out := &llmcore.WebSearchOptions{}
	if base != nil {
		out.SearchContextSize = strings.TrimSpace(base.SearchContextSize)
		out.AllowedDomains = normalizeWebSearchDomains(base.AllowedDomains)
		out.BlockedDomains = normalizeWebSearchDomains(base.BlockedDomains)
		out.MaxUses = base.MaxUses
		if base.UserLocation != nil {
			out.UserLocation = &llmcore.WebSearchLocation{
				Type:     strings.TrimSpace(base.UserLocation.Type),
				Country:  strings.TrimSpace(base.UserLocation.Country),
				Region:   strings.TrimSpace(base.UserLocation.Region),
				City:     strings.TrimSpace(base.UserLocation.City),
				Timezone: strings.TrimSpace(base.UserLocation.Timezone),
			}
		}
	}
	if override != nil {
		if v := strings.TrimSpace(override.SearchContextSize); v != "" {
			out.SearchContextSize = v
		}
		if domains := normalizeWebSearchDomains(override.AllowedDomains); len(domains) > 0 {
			out.AllowedDomains = domains
		}
		if domains := normalizeWebSearchDomains(override.BlockedDomains); len(domains) > 0 {
			out.BlockedDomains = domains
		}
		if override.MaxUses > 0 {
			out.MaxUses = override.MaxUses
		}
		if override.UserLocation != nil {
			if out.UserLocation == nil {
				out.UserLocation = &llmcore.WebSearchLocation{}
			}
			if v := strings.TrimSpace(override.UserLocation.Type); v != "" {
				out.UserLocation.Type = v
			}
			if v := strings.TrimSpace(override.UserLocation.Country); v != "" {
				out.UserLocation.Country = v
			}
			if v := strings.TrimSpace(override.UserLocation.Region); v != "" {
				out.UserLocation.Region = v
			}
			if v := strings.TrimSpace(override.UserLocation.City); v != "" {
				out.UserLocation.City = v
			}
			if v := strings.TrimSpace(override.UserLocation.Timezone); v != "" {
				out.UserLocation.Timezone = v
			}
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

// =============================================================================
// API Key Conversions
// =============================================================================

// ToLLMProviderAPIKey converts CreateAPIKeyInput to llm/core LLMProviderAPIKey.
func (b *DefaultLLMTypeBridge) ToLLMProviderAPIKey(providerID uint, input CreateAPIKeyInput) llmcore.LLMProviderAPIKey {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	priority := input.Priority
	if priority == 0 {
		priority = 100
	}
	weight := input.Weight
	if weight == 0 {
		weight = 100
	}
	return llmcore.LLMProviderAPIKey{
		ProviderID:   providerID,
		APIKey:       input.APIKey,
		BaseURL:      input.BaseURL,
		Label:        input.Label,
		Priority:     priority,
		Weight:       weight,
		Enabled:      enabled,
		RateLimitRPM: input.RateLimitRPM,
		RateLimitRPD: input.RateLimitRPD,
	}
}

// FromLLMProviderAPIKey converts llm/core LLMProviderAPIKey to APIKeyView.
func (b *DefaultLLMTypeBridge) FromLLMProviderAPIKey(key llmcore.LLMProviderAPIKey) APIKeyView {
	return APIKeyView{
		ID:             key.ID,
		ProviderID:     key.ProviderID,
		APIKeyMasked:   maskAPIKey(key.APIKey),
		BaseURL:        key.BaseURL,
		Label:          key.Label,
		Priority:       key.Priority,
		Weight:         key.Weight,
		Enabled:        key.Enabled,
		TotalRequests:  key.TotalRequests,
		FailedRequests: key.FailedRequests,
		RateLimitRPM:   key.RateLimitRPM,
		RateLimitRPD:   key.RateLimitRPD,
	}
}

// FromLLMProviderAPIKeys converts a slice of llm/core LLMProviderAPIKey to APIKeyView slice.
func (b *DefaultLLMTypeBridge) FromLLMProviderAPIKeys(keys []llmcore.LLMProviderAPIKey) []APIKeyView {
	if len(keys) == 0 {
		return nil
	}
	result := make([]APIKeyView, 0, len(keys))
	for _, k := range keys {
		result = append(result, b.FromLLMProviderAPIKey(k))
	}
	return result
}

// FromLLMProviderAPIKeyStats converts llm/core LLMProviderAPIKey slice to APIKeyStatsView slice.
func (b *DefaultLLMTypeBridge) FromLLMProviderAPIKeyStats(keys []llmcore.LLMProviderAPIKey) []APIKeyStatsView {
	if len(keys) == 0 {
		return nil
	}
	stats := make([]APIKeyStatsView, 0, len(keys))
	for _, k := range keys {
		successRate := 1.0
		if k.TotalRequests > 0 {
			successRate = float64(k.TotalRequests-k.FailedRequests) / float64(k.TotalRequests)
		}
		stats = append(stats, APIKeyStatsView{
			KeyID:          k.ID,
			Label:          k.Label,
			BaseURL:        k.BaseURL,
			Enabled:        k.Enabled,
			IsHealthy:      k.IsHealthy(),
			TotalRequests:  k.TotalRequests,
			FailedRequests: k.FailedRequests,
			SuccessRate:    successRate,
			CurrentRPM:     k.CurrentRPM,
			CurrentRPD:     k.CurrentRPD,
			LastUsedAt:     k.LastUsedAt,
			LastErrorAt:    k.LastErrorAt,
			LastError:      k.LastError,
		})
	}
	return stats
}

// =============================================================================
// Provider Conversions
// =============================================================================

// FromLLMProviders converts llm/core LLMProvider slice to ProviderView slice.
func (b *DefaultLLMTypeBridge) FromLLMProviders(providers []llmcore.LLMProvider) []ProviderView {
	if len(providers) == 0 {
		return nil
	}
	result := make([]ProviderView, 0, len(providers))
	for _, p := range providers {
		result = append(result, ProviderView{
			ID:          p.ID,
			Code:        p.Code,
			Name:        p.Name,
			Description: p.Description,
			Status:      p.Status.String(),
			CreatedAt:   p.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
		})
	}
	return result
}

// =============================================================================
// Chat Request/Response Conversions
// =============================================================================

// ToLLMChatRequest converts usecase ChatRequest to types ChatRequest.
// This method delegates to ChatConverter if available, otherwise performs direct conversion.
func (b *DefaultLLMTypeBridge) ToLLMChatRequest(req *ChatRequest) *types.ChatRequest {
	if req == nil {
		return nil
	}
	if b.chatConverter != nil {
		return b.chatConverter.ToLLMRequest(req)
	}
	// Fallback direct conversion (should not be used in production)
	return directChatRequestConversion(req)
}

// FromLLMChatResponse converts types ChatResponse to usecase ChatResponse.
// This method delegates to ChatConverter if available, otherwise performs direct conversion.
func (b *DefaultLLMTypeBridge) FromLLMChatResponse(resp *types.ChatResponse) *ChatResponse {
	if resp == nil {
		return nil
	}
	if b.chatConverter != nil {
		return b.chatConverter.ToChatResponse(resp)
	}
	// Fallback direct conversion (should not be used in production)
	return directChatResponseConversion(resp)
}

// =============================================================================
// Helper Functions
// =============================================================================

// normalizeWebSearchDomains deduplicates and trims domain list.
func normalizeWebSearchDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		v := strings.TrimSpace(d)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// directChatRequestConversion provides a fallback conversion without ChatConverter.
func directChatRequestConversion(req *ChatRequest) *types.ChatRequest {
	messages := make([]types.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = types.Message{
			Role:             types.Role(msg.Role),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			ReasoningSummaries: msg.ReasoningSummaries,
			OpaqueReasoning:    msg.OpaqueReasoning,
			ThinkingBlocks:     msg.ThinkingBlocks,
			Refusal:            msg.Refusal,
			Name:               msg.Name,
			ToolCalls:          msg.ToolCalls,
			ToolCallID:         msg.ToolCallID,
			IsToolError:        msg.IsToolError,
			Videos:             msg.Videos,
			Annotations:        msg.Annotations,
			Timestamp:          msg.Timestamp,
		}
		if len(msg.Images) > 0 {
			messages[i].Images = make([]types.ImageContent, len(msg.Images))
			for j, img := range msg.Images {
				messages[i].Images[j] = types.ImageContent{
					Type: img.Type,
					URL:  img.URL,
					Data: img.Data,
				}
			}
		}
	}

	tools := make([]types.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = types.ToolSchema{
			Type:        tool.Type,
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Strict:      tool.Strict,
			Version:     tool.Version,
		}
		if tool.Format != nil {
			tools[i].Format = &types.ToolFormat{
				Type:       tool.Format.Type,
				Syntax:     tool.Format.Syntax,
				Definition: tool.Format.Definition,
			}
		}
	}

	out := &types.ChatRequest{
		TraceID:                          req.TraceID,
		TenantID:                         req.TenantID,
		UserID:                           req.UserID,
		Model:                            req.Model,
		Messages:                         messages,
		MaxTokens:                        req.MaxTokens,
		Temperature:                      req.Temperature,
		TopP:                             req.TopP,
		FrequencyPenalty:                 req.FrequencyPenalty,
		PresencePenalty:                  req.PresencePenalty,
		RepetitionPenalty:                req.RepetitionPenalty,
		N:                                req.N,
		LogProbs:                         req.LogProbs,
		TopLogProbs:                      req.TopLogProbs,
		Stop:                             append([]string(nil), req.Stop...),
		Tools:                            tools,
		ToolChoice:                       parseToolChoiceValue(req.ToolChoice),
		ParallelToolCalls:                req.ParallelToolCalls,
		ServiceTier:                      req.ServiceTier,
		User:                             req.User,
		MaxCompletionTokens:              req.MaxCompletionTokens,
		ReasoningEffort:                  req.ReasoningEffort,
		ReasoningSummary:                 req.ReasoningSummary,
		ReasoningDisplay:                 req.ReasoningDisplay,
		InferenceSpeed:                   req.InferenceSpeed,
		Store:                            req.Store,
		Modalities:                       append([]string(nil), req.Modalities...),
		PromptCacheKey:                   req.PromptCacheKey,
		PromptCacheRetention:             req.PromptCacheRetention,
		CachedContent:                    req.CachedContent,
		IncludeServerSideToolInvocations: req.IncludeServerSideToolInvocations,
		PreviousResponseID:               req.PreviousResponseID,
		ConversationID:                   req.ConversationID,
		Include:                          append([]string(nil), req.Include...),
		Truncation:                       req.Truncation,
		Metadata:                         cloneStringMap(req.Metadata),
		Tags:                             append([]string(nil), req.Tags...),
	}

	if req.ResponseFormat != nil {
		out.ResponseFormat = &types.ResponseFormat{
			Type: types.ResponseFormatType(req.ResponseFormat.Type),
		}
		if req.ResponseFormat.JSONSchema != nil {
			out.ResponseFormat.JSONSchema = &types.JSONSchemaParam{
				Name:        req.ResponseFormat.JSONSchema.Name,
				Description: req.ResponseFormat.JSONSchema.Description,
				Schema:      req.ResponseFormat.JSONSchema.Schema,
				Strict:      req.ResponseFormat.JSONSchema.Strict,
			}
		}
	}

	if req.StreamOptions != nil {
		out.StreamOptions = &types.StreamOptions{
			IncludeUsage:      req.StreamOptions.IncludeUsage,
			ChunkIncludeUsage: req.StreamOptions.ChunkIncludeUsage,
		}
	}

	if req.CacheControl != nil {
		out.CacheControl = &types.CacheControl{
			Type: req.CacheControl.Type,
			TTL:  req.CacheControl.TTL,
		}
	}

	return out
}

// directChatResponseConversion provides a fallback conversion without ChatConverter.
func directChatResponseConversion(resp *types.ChatResponse) *ChatResponse {
	if resp == nil {
		return nil
	}
	choices := make([]ChatChoice, len(resp.Choices))
	for i, c := range resp.Choices {
		choices[i] = ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      convertTypesMessageToUsecase(c.Message),
		}
	}
	return &ChatResponse{
		ID:        resp.ID,
		Provider:  resp.Provider,
		Model:     resp.Model,
		Choices:   choices,
		Usage:     convertTypesUsageToUsecase(resp.Usage),
		CreatedAt: resp.CreatedAt,
	}
}

// convertTypesMessageToUsecase converts types.Message to usecase Message.
func convertTypesMessageToUsecase(msg types.Message) Message {
	return Message{
		Role:               string(msg.Role),
		Content:            msg.Content,
		ReasoningContent:   msg.ReasoningContent,
		ReasoningSummaries: msg.ReasoningSummaries,
		OpaqueReasoning:    msg.OpaqueReasoning,
		ThinkingBlocks:     msg.ThinkingBlocks,
		Refusal:            msg.Refusal,
		Name:               msg.Name,
		ToolCalls:          msg.ToolCalls,
		ToolCallID:         msg.ToolCallID,
		IsToolError:        msg.IsToolError,
		Videos:             msg.Videos,
		Annotations:        msg.Annotations,
		Metadata:           msg.Metadata,
		Timestamp:          msg.Timestamp,
	}
}

// convertTypesUsageToUsecase converts types.ChatUsage to usecase ChatUsage.
func convertTypesUsageToUsecase(usage types.ChatUsage) ChatUsage {
	out := ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if usage.PromptTokensDetails != nil {
		out.PromptTokensDetails = &PromptTokensDetails{
			CachedTokens:        usage.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: usage.PromptTokensDetails.CacheCreationTokens,
			AudioTokens:         usage.PromptTokensDetails.AudioTokens,
		}
	}
	if usage.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &CompletionTokensDetails{
			ReasoningTokens:          usage.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              usage.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: usage.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: usage.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}
	return out
}

// parseToolChoiceValue converts any tool choice value to *types.ToolChoice.
func parseToolChoiceValue(v any) *types.ToolChoice {
	if v == nil {
		return nil
	}
	switch tc := v.(type) {
	case string:
		return types.ParseToolChoiceString(tc)
	case map[string]any:
		if fn, ok := tc["function"].(map[string]any); ok {
			name, _ := fn["name"].(string)
			if name == "" {
				return nil
			}
			return &types.ToolChoice{
				Mode:     types.ToolChoiceModeSpecific,
				ToolName: name,
			}
		}
		return nil
	default:
		return nil
	}
}

// cloneStringMap creates a shallow copy of a string map.
func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
