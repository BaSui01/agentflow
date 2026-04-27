package providerbase

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func StreamSSE(ctx context.Context, body io.ReadCloser, providerName string) <-chan llm.StreamChunk {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case <-ctx.Done():
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: fmt.Sprintf("stream parse panic: %v", r),
					HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
			}
		}()
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: &types.Error{
						Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
					}}:
					}
				}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}

			var oaResp OpenAICompatResponse
			if err := json.Unmarshal([]byte(data), &oaResp); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: &types.Error{
					Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: providerName,
				}}:
				}
				return
			}

			if oaResp.Usage != nil {
				streamUsage := &llm.ChatUsage{
					PromptTokens:     oaResp.Usage.PromptTokens,
					CompletionTokens: oaResp.Usage.CompletionTokens,
					TotalTokens:      oaResp.Usage.TotalTokens,
				}
				if len(oaResp.Choices) == 0 {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{
						ID:       oaResp.ID,
						Provider: providerName,
						Model:    oaResp.Model,
						Usage:    streamUsage,
					}:
					}
					continue
				}
			}

			for _, choice := range oaResp.Choices {
				chunk := llm.StreamChunk{
					ID:           oaResp.ID,
					Provider:     providerName,
					Model:        oaResp.Model,
					Index:        choice.Index,
					FinishReason: choice.FinishReason,
					Delta: types.Message{
						Role: llm.RoleAssistant,
					},
				}
				if choice.Delta != nil {
					chunk.Delta.Content = choice.Delta.Content
					chunk.Delta.Refusal = choice.Delta.Refusal
					chunk.Delta.ReasoningContent = choice.Delta.ReasoningContent
					if len(choice.Delta.ToolCalls) > 0 {
						chunk.Delta.ToolCalls = make([]types.ToolCall, 0, len(choice.Delta.ToolCalls))
						for _, tc := range choice.Delta.ToolCalls {
							chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, types.ToolCall{
								Index:     tc.Index,
								ID:        tc.ID,
								Name:      tc.Function.Name,
								Arguments: UnwrapStringifiedJSON(tc.Function.Arguments),
							})
						}
					}
				}
				if oaResp.Usage != nil {
					chunk.Usage = &llm.ChatUsage{
						PromptTokens:     oaResp.Usage.PromptTokens,
						CompletionTokens: oaResp.Usage.CompletionTokens,
						TotalTokens:      oaResp.Usage.TotalTokens,
					}
				}
				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}
			}
		}
	}()
	return ch
}

func NormalizeFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "STOP", "COMPLETED", "CANCELLED":
		return "stop"
	case "MAX_TOKENS", "INCOMPLETE", "LENGTH":
		return "length"
	case "TOOL_CALLS":
		return "tool_calls"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "LANGUAGE":
		return "content_filter"
	case "FAILED":
		return "error"
	case "":
		return ""
	default:
		return strings.ToLower(reason)
	}
}

func MapSDKError(err error, providerName string, extractAPIError func(err error) (statusCode int, message string, ok bool)) error {
	if err == nil {
		return nil
	}
	if statusCode, message, ok := extractAPIError(err); ok {
		return MapHTTPError(statusCode, message, providerName)
	}
	return &types.Error{
		Code:       llm.ErrUpstreamError,
		Message:    err.Error(),
		Cause:      err,
		HTTPStatus: http.StatusBadGateway,
		Retryable:  true,
		Provider:   providerName,
	}
}
