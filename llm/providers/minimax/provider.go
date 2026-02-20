package minimax

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// MiniMaxProvider 实现 MiniMax LLM 提供者.
// MiniMax 使用 OpenAI 兼容的 API 格式.
type MiniMaxProvider struct {
	*openaicompat.Provider
}

// NewMiniMaxProvider 创建新的 MiniMax 提供者实例.
func NewMiniMaxProvider(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.io"
	}

	return &MiniMaxProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "minimax",
			APIKey:        cfg.APIKey,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "abab6.5s-chat",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}

// Stream 覆写父类方法，对 SSE 流做后处理以解析 MiniMax 特有的 XML tool call 格式.
// MiniMax 将 tool calls 放在 content 字段中用 <tool_calls> XML 标签包裹.
func (p *MiniMaxProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	upstream, err := p.Provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan llm.StreamChunk)
	go func() {
		defer close(out)
		for chunk := range upstream {
			// 检测 content 中是否包含 MiniMax XML tool call 格式
			if tc, ok := parseXMLToolCall(chunk.Delta.Content); ok {
				chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, tc)
				chunk.Delta.Content = ""
			}
			out <- chunk
		}
	}()
	return out, nil
}

// xmlToolCallPayload 用于反序列化 <tool_calls> 标签内的 JSON.
type xmlToolCallPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// parseXMLToolCall 从 MiniMax 的 XML 格式 content 中提取 tool call.
// 格式: <tool_calls>\n{"name":"...","arguments":...}\n</tool_calls>
func parseXMLToolCall(content string) (llm.ToolCall, bool) {
	const openTag = "<tool_calls>"
	const closeTag = "</tool_calls>"

	startIdx := strings.Index(content, openTag)
	endIdx := strings.Index(content, closeTag)
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return llm.ToolCall{}, false
	}

	jsonStr := strings.TrimSpace(content[startIdx+len(openTag) : endIdx])
	if jsonStr == "" {
		return llm.ToolCall{}, false
	}

	var payload xmlToolCallPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return llm.ToolCall{}, false
	}

	return llm.ToolCall{
		ID:        fmt.Sprintf("minimax_tc_%s", payload.Name),
		Name:      payload.Name,
		Arguments: payload.Arguments,
	}, true
}
