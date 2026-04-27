package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/api"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func buildAPIChatRequestFromCompatCompletions(req openAICompatChatCompletionsRequest) (*api.ChatRequest, *types.Error) {
	messages, err := convertOpenAICompatInboundMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	tools, wsOptionsFromTools, err := convertOpenAICompatInboundTools(req.Tools)
	if err != nil {
		return nil, err
	}
	responseFormat, err := convertOpenAICompatResponseFormat(req.ResponseFormat)
	if err != nil {
		return nil, err
	}
	wsOptions := mergeOpenAICompatWebSearchOptions(req.WebSearchOptions, wsOptionsFromTools)

	apiReq := &api.ChatRequest{
		Model:                req.Model,
		Messages:             messages,
		MaxTokens:            req.MaxTokens,
		MaxCompletionTokens:  req.MaxCompletionTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		FrequencyPenalty:     req.FrequencyPenalty,
		PresencePenalty:      req.PresencePenalty,
		RepetitionPenalty:    req.RepetitionPenalty,
		N:                    req.N,
		LogProbs:             req.LogProbs,
		TopLogProbs:          req.TopLogProbs,
		Stop:                 req.Stop,
		Tools:                tools,
		ToolChoice:           req.ToolChoice,
		ResponseFormat:       responseFormat,
		StreamOptions:        req.StreamOptions,
		ParallelToolCalls:    req.ParallelToolCalls,
		ServiceTier:          req.ServiceTier,
		User:                 req.User,
		ReasoningEffort:      strings.TrimSpace(req.ReasoningEffort),
		ReasoningSummary:     "",
		Store:                req.Store,
		Modalities:           req.Modalities,
		PromptCacheKey:       strings.TrimSpace(req.PromptCacheKey),
		PromptCacheRetention: strings.TrimSpace(req.PromptCacheRetention),
		PreviousResponseID:   strings.TrimSpace(req.PreviousResponseID),
		ConversationID:       strings.TrimSpace(req.ConversationID),
		Include:              req.Include,
		Truncation:           strings.TrimSpace(req.Truncation),
		Provider:             req.Provider,
		RoutePolicy:          req.RoutePolicy,
		EndpointMode:         req.EndpointMode,
		Timeout:              req.Timeout,
		Metadata:             req.Metadata,
		Tags:                 req.Tags,
	}
	applyWebSearchOptionsToChatRequest(apiReq, wsOptions)
	return apiReq, nil
}

func buildAPIChatRequestFromCompatResponses(req openAICompatResponsesRequest) (*api.ChatRequest, *types.Error) {
	messages, err := convertOpenAICompatResponsesInput(req.Input)
	if err != nil {
		return nil, err
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		messages = append([]api.Message{{Role: "system", Content: instructions}}, messages...)
	}
	tools, wsOptionsFromTools, err := convertOpenAICompatInboundTools(req.Tools)
	if err != nil {
		return nil, err
	}

	var responseFormat *api.ResponseFormat
	if req.Text != nil {
		responseFormat, err = convertOpenAICompatResponseFormat(req.Text.Format)
		if err != nil {
			return nil, err
		}
	}

	maxTokens := 0
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		maxTokens = *req.MaxOutputTokens
	}
	temp := float32(0)
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	topP := float32(0)
	if req.TopP != nil {
		topP = *req.TopP
	}
	endpointMode := strings.TrimSpace(req.EndpointMode)
	if endpointMode == "" {
		endpointMode = "responses"
	}

	reasoningEffort := ""
	reasoningSummary := ""
	if req.Reasoning != nil {
		reasoningEffort = strings.TrimSpace(req.Reasoning.Effort)
		reasoningSummary = strings.TrimSpace(req.Reasoning.Summary)
	}
	wsOptions := mergeOpenAICompatWebSearchOptions(req.WebSearchOptions, wsOptionsFromTools)
	conversationID := strings.TrimSpace(req.Conversation)
	if conversationID == "" {
		conversationID = strings.TrimSpace(req.ConversationID)
	}

	apiReq := &api.ChatRequest{
		Model:                req.Model,
		Messages:             messages,
		MaxTokens:            maxTokens,
		MaxCompletionTokens:  req.MaxOutputTokens,
		Temperature:          temp,
		TopP:                 topP,
		Tools:                tools,
		ToolChoice:           req.ToolChoice,
		ResponseFormat:       responseFormat,
		ParallelToolCalls:    req.ParallelToolCalls,
		ServiceTier:          req.ServiceTier,
		User:                 req.User,
		ReasoningEffort:      reasoningEffort,
		ReasoningSummary:     reasoningSummary,
		Store:                req.Store,
		PromptCacheKey:       strings.TrimSpace(req.PromptCacheKey),
		PromptCacheRetention: strings.TrimSpace(req.PromptCacheRetention),
		PreviousResponseID:   strings.TrimSpace(req.PreviousResponseID),
		ConversationID:       conversationID,
		Include:              req.Include,
		Truncation:           strings.TrimSpace(req.Truncation),
		Provider:             req.Provider,
		RoutePolicy:          req.RoutePolicy,
		EndpointMode:         endpointMode,
		Timeout:              req.Timeout,
		Metadata:             req.Metadata,
		Tags:                 req.Tags,
	}
	applyWebSearchOptionsToChatRequest(apiReq, wsOptions)
	return apiReq, nil
}

func applyWebSearchOptionsToChatRequest(req *api.ChatRequest, opts *llmcore.WebSearchOptions) {
	if req == nil || opts == nil {
		return
	}
	req.WebSearchOptions = toAPIWebSearchOptions(mergeLLMWebSearchOptions(convertAPIWebSearchOptions(req.WebSearchOptions), opts))

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	merged := convertAPIWebSearchOptions(req.WebSearchOptions)
	if merged == nil {
		return
	}
	if v := strings.TrimSpace(merged.SearchContextSize); v != "" {
		req.Metadata["web_search_context_size"] = v
	}
	if merged.UserLocation != nil {
		if v := strings.TrimSpace(merged.UserLocation.Country); v != "" {
			req.Metadata["web_search_country"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.Region); v != "" {
			req.Metadata["web_search_region"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.City); v != "" {
			req.Metadata["web_search_city"] = v
		}
		if v := strings.TrimSpace(merged.UserLocation.Timezone); v != "" {
			req.Metadata["web_search_timezone"] = v
		}
	}
	if domains := normalizeAllowedDomains(merged.AllowedDomains); len(domains) > 0 {
		req.Metadata["web_search_allowed_domains"] = strings.Join(domains, ",")
	}
	if domains := normalizeAllowedDomains(merged.BlockedDomains); len(domains) > 0 {
		req.Metadata["web_search_blocked_domains"] = strings.Join(domains, ",")
	}
	if merged.MaxUses > 0 {
		req.Metadata["web_search_max_uses"] = fmt.Sprintf("%d", merged.MaxUses)
	}
}

func convertOpenAICompatResponseFormat(raw any) (*api.ResponseFormat, *types.Error) {
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		return &api.ResponseFormat{Type: s}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, types.NewInvalidRequestError("invalid response_format")
	}
	var out api.ResponseFormat
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, types.NewInvalidRequestError("invalid response_format")
	}
	out.Type = strings.TrimSpace(out.Type)
	if out.Type == "" {
		return nil, types.NewInvalidRequestError("response_format.type is required")
	}
	return &out, nil
}

func convertOpenAICompatWebSearchOptions(in *openAICompatWebSearchOptions) *llmcore.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &llmcore.WebSearchOptions{
		SearchContextSize: strings.TrimSpace(in.SearchContextSize),
		UserLocation:      parseOpenAICompatWebSearchLocation(in.UserLocation),
		AllowedDomains:    normalizeAllowedDomains(in.AllowedDomains),
		BlockedDomains:    normalizeAllowedDomains(in.BlockedDomains),
		MaxUses:           in.MaxUses,
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

func parseOpenAICompatWebSearchLocation(raw json.RawMessage) *llmcore.WebSearchLocation {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var payload struct {
		Type        string `json:"type,omitempty"`
		Country     string `json:"country,omitempty"`
		Region      string `json:"region,omitempty"`
		City        string `json:"city,omitempty"`
		Timezone    string `json:"timezone,omitempty"`
		Approximate *struct {
			Country  string `json:"country,omitempty"`
			Region   string `json:"region,omitempty"`
			City     string `json:"city,omitempty"`
			Timezone string `json:"timezone,omitempty"`
		} `json:"approximate,omitempty"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	out := &llmcore.WebSearchLocation{
		Type:     strings.TrimSpace(payload.Type),
		Country:  strings.TrimSpace(payload.Country),
		Region:   strings.TrimSpace(payload.Region),
		City:     strings.TrimSpace(payload.City),
		Timezone: strings.TrimSpace(payload.Timezone),
	}
	if payload.Approximate != nil {
		if out.Type == "" {
			out.Type = "approximate"
		}
		if out.Country == "" {
			out.Country = strings.TrimSpace(payload.Approximate.Country)
		}
		if out.Region == "" {
			out.Region = strings.TrimSpace(payload.Approximate.Region)
		}
		if out.City == "" {
			out.City = strings.TrimSpace(payload.Approximate.City)
		}
		if out.Timezone == "" {
			out.Timezone = strings.TrimSpace(payload.Approximate.Timezone)
		}
	}

	if out.Type == "" && out.Country == "" && out.Region == "" && out.City == "" && out.Timezone == "" {
		return nil
	}
	return out
}

func normalizeAllowedDomains(domains []string) []string {
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

func mergeOpenAICompatWebSearchOptions(topLevel *openAICompatWebSearchOptions, fromTools *llmcore.WebSearchOptions) *llmcore.WebSearchOptions {
	return mergeLLMWebSearchOptions(fromTools, convertOpenAICompatWebSearchOptions(topLevel))
}

func mergeLLMWebSearchOptions(base *llmcore.WebSearchOptions, override *llmcore.WebSearchOptions) *llmcore.WebSearchOptions {
	if base == nil && override == nil {
		return nil
	}
	out := &llmcore.WebSearchOptions{}
	if base != nil {
		out.SearchContextSize = strings.TrimSpace(base.SearchContextSize)
		out.AllowedDomains = normalizeAllowedDomains(base.AllowedDomains)
		out.BlockedDomains = normalizeAllowedDomains(base.BlockedDomains)
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
		if domains := normalizeAllowedDomains(override.AllowedDomains); len(domains) > 0 {
			out.AllowedDomains = domains
		}
		if domains := normalizeAllowedDomains(override.BlockedDomains); len(domains) > 0 {
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

func toAPIWebSearchOptions(in *llmcore.WebSearchOptions) *api.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &api.WebSearchOptions{
		SearchContextSize: strings.TrimSpace(in.SearchContextSize),
		AllowedDomains:    normalizeAllowedDomains(in.AllowedDomains),
		BlockedDomains:    normalizeAllowedDomains(in.BlockedDomains),
		MaxUses:           in.MaxUses,
	}
	if in.UserLocation != nil {
		out.UserLocation = &api.WebSearchLocation{
			Type:     strings.TrimSpace(in.UserLocation.Type),
			Country:  strings.TrimSpace(in.UserLocation.Country),
			Region:   strings.TrimSpace(in.UserLocation.Region),
			City:     strings.TrimSpace(in.UserLocation.City),
			Timezone: strings.TrimSpace(in.UserLocation.Timezone),
		}
	}
	if out.SearchContextSize == "" && out.UserLocation == nil && len(out.AllowedDomains) == 0 &&
		len(out.BlockedDomains) == 0 && out.MaxUses == 0 {
		return nil
	}
	return out
}

func convertOpenAICompatInboundMessages(in []openAICompatInboundMessage) ([]api.Message, *types.Error) {
	out := make([]api.Message, 0, len(in))
	for _, msg := range in {
		toolCalls := make([]types.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			callType := strings.ToLower(strings.TrimSpace(tc.Type))
			if callType == "" {
				callType = types.ToolTypeFunction
			}
			call := types.ToolCall{
				ID:   tc.ID,
				Type: callType,
			}
			switch callType {
			case types.ToolTypeCustom:
				call.Name = strings.TrimSpace(tc.Custom.Name)
				call.Input = tc.Custom.Input
			default:
				call.Type = types.ToolTypeFunction
				args := json.RawMessage(strings.TrimSpace(tc.Function.Arguments))
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				call.Name = tc.Function.Name
				call.Arguments = args
			}
			toolCalls = append(toolCalls, call)
		}
		out = append(out, api.Message{
			Role:       msg.Role,
			Content:    flattenOpenAICompatContent(msg.Content),
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}
	return out, nil
}

func convertOpenAICompatResponsesInput(input any) ([]api.Message, *types.Error) {
	switch v := input.(type) {
	case string:
		return []api.Message{{Role: "user", Content: v}}, nil
	case []any:
		out := make([]api.Message, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.TrimSpace(asString(m["type"]))
			if itemType == "function_call_output" {
				out = append(out, api.Message{
					Role:       "tool",
					ToolCallID: strings.TrimSpace(asString(m["call_id"])),
					Content:    flattenOpenAICompatContent(m["output"]),
				})
				continue
			}
			if itemType == "custom_tool_call_output" {
				out = append(out, api.Message{
					Role:       "tool",
					ToolCallID: strings.TrimSpace(asString(m["call_id"])),
					Content:    flattenOpenAICompatContent(m["output"]),
				})
				continue
			}
			if itemType == "function_call" {
				args := json.RawMessage(strings.TrimSpace(asString(m["arguments"])))
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				out = append(out, api.Message{
					Role: "assistant",
					ToolCalls: []types.ToolCall{{
						ID:        strings.TrimSpace(firstNonEmptyString(asString(m["call_id"]), asString(m["id"]))),
						Type:      types.ToolTypeFunction,
						Name:      strings.TrimSpace(asString(m["name"])),
						Arguments: args,
					}},
				})
				continue
			}
			if itemType == "custom_tool_call" {
				out = append(out, api.Message{
					Role: "assistant",
					ToolCalls: []types.ToolCall{{
						ID:    strings.TrimSpace(firstNonEmptyString(asString(m["call_id"]), asString(m["id"]))),
						Type:  types.ToolTypeCustom,
						Name:  strings.TrimSpace(asString(m["name"])),
						Input: asString(m["input"]),
					}},
				})
				continue
			}
			if itemType == "reasoning" {
				out = append(out, convertOpenAICompatReasoningInputItem(m))
				continue
			}
			role := strings.TrimSpace(asString(m["role"]))
			if role == "" {
				role = "user"
			}
			out = append(out, api.Message{
				Role:    role,
				Content: flattenOpenAICompatContent(m["content"]),
			})
		}
		if len(out) == 0 {
			return nil, types.NewInvalidRequestError("responses input cannot be empty")
		}
		return out, nil
	default:
		return nil, types.NewInvalidRequestError("responses input must be string or array")
	}
}

func convertOpenAICompatReasoningInputItem(item map[string]any) api.Message {
	msg := api.Message{Role: "assistant"}
	itemID := strings.TrimSpace(asString(item["id"]))
	if summaries := convertOpenAICompatReasoningSummaries(item["summary"], itemID); len(summaries) > 0 {
		msg.ReasoningSummaries = summaries
		joined := joinOpenAICompatSummaryTexts(summaries)
		if joined != "" {
			msg.ReasoningContent = &joined
		}
	}
	if content := convertOpenAICompatReasoningText(item["content"]); content != "" {
		msg.ReasoningContent = &content
	}
	if encrypted := strings.TrimSpace(asString(item["encrypted_content"])); encrypted != "" {
		msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{
			Provider: "openai",
			ID:       itemID,
			Kind:     "encrypted_content",
			State:    encrypted,
			Status:   strings.TrimSpace(asString(item["status"])),
		})
	}
	return msg
}

func convertOpenAICompatReasoningSummaries(raw any, itemID string) []types.ReasoningSummary {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]types.ReasoningSummary, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(asString(m["text"]))
		if text == "" {
			continue
		}
		kind := strings.TrimSpace(asString(m["type"]))
		if kind == "" {
			kind = "summary_text"
		}
		out = append(out, types.ReasoningSummary{
			Provider: "openai",
			ID:       itemID,
			Kind:     kind,
			Text:     text,
		})
	}
	return out
}

func joinOpenAICompatSummaryTexts(summaries []types.ReasoningSummary) string {
	parts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if strings.TrimSpace(summary.Text) != "" {
			parts = append(parts, summary.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func convertOpenAICompatReasoningText(raw any) string {
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(asString(m["text"]))
		if text == "" {
			continue
		}
		if kind := strings.TrimSpace(asString(m["type"])); kind != "" && kind != "reasoning_text" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

func convertOpenAICompatInboundTools(in []openAICompatInboundTool) ([]api.ToolSchema, *llmcore.WebSearchOptions, *types.Error) {
	if len(in) == 0 {
		return nil, nil, nil
	}
	tools := make([]api.ToolSchema, 0, len(in))
	var wsOpts *llmcore.WebSearchOptions

	for _, tool := range in {
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		switch toolType {
		case "web_search", "web_search_preview":
			tools = append(tools, api.ToolSchema{
				Name:       "web_search",
				Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			})
			candidate := &llmcore.WebSearchOptions{
				SearchContextSize: strings.TrimSpace(tool.SearchContextSize),
				UserLocation:      parseOpenAICompatWebSearchLocation(tool.UserLocation),
			}
			if tool.Filters != nil {
				candidate.AllowedDomains = normalizeAllowedDomains(tool.Filters.AllowedDomains)
			}
			wsOpts = mergeLLMWebSearchOptions(wsOpts, candidate)
		case "", "function":
			name := strings.TrimSpace(tool.Name)
			desc := strings.TrimSpace(tool.Description)
			params := tool.Parameters
			strict := tool.Strict
			if strings.TrimSpace(tool.Function.Name) != "" {
				name = strings.TrimSpace(tool.Function.Name)
				desc = strings.TrimSpace(tool.Function.Description)
				params = tool.Function.Parameters
				if tool.Function.Strict != nil {
					strict = tool.Function.Strict
				}
			}
			if name == "" {
				continue
			}
			paramJSON, err := json.Marshal(params)
			if err != nil {
				return nil, nil, types.NewInvalidRequestError("invalid tool parameters")
			}
			if string(paramJSON) == "null" {
				paramJSON = []byte(`{"type":"object","properties":{}}`)
			}
			tools = append(tools, api.ToolSchema{
				Type:        types.ToolTypeFunction,
				Name:        name,
				Description: desc,
				Parameters:  paramJSON,
				Strict:      strict,
			})
		case types.ToolTypeCustom:
			name := strings.TrimSpace(tool.Name)
			desc := strings.TrimSpace(tool.Description)
			format := tool.Format
			if strings.TrimSpace(tool.Custom.Name) != "" {
				name = strings.TrimSpace(tool.Custom.Name)
				desc = strings.TrimSpace(tool.Custom.Description)
				format = tool.Custom.Format
			}
			if name == "" {
				continue
			}
			tools = append(tools, api.ToolSchema{
				Type:        types.ToolTypeCustom,
				Name:        name,
				Description: desc,
				Parameters:  json.RawMessage(`{}`),
				Format:      parseOpenAICompatToolFormat(format),
			})
		default:
			continue
		}
	}
	return tools, wsOpts, nil
}

func flattenOpenAICompatContent(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, p := range v {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(asString(pm["text"]))
			if text == "" {
				text = strings.TrimSpace(asString(pm["content"]))
			}
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if t := strings.TrimSpace(asString(v["text"])); t != "" {
			return t
		}
		if t := strings.TrimSpace(asString(v["content"])); t != "" {
			return t
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseOpenAICompatToolFormat(raw any) *api.ToolFormat {
	if raw == nil {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out api.ToolFormat
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	if strings.TrimSpace(out.Type) == "" && strings.TrimSpace(out.Syntax) == "" && strings.TrimSpace(out.Definition) == "" {
		return nil
	}
	return &out
}
