package minimax

import (
	"github.com/BaSui01/agentflow/types"
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
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "MiniMax-Text-01",
			Timeout:       cfg.Timeout,
		}, logger),
	}
}

// Stream 覆写父类方法，对 SSE 流做后处理以解析 MiniMax 特有的 XML tool call 格式.
// MiniMax 将 tool calls 放在 content 字段中用 <tool_calls> XML 标签包裹.
// 新模型（MiniMax-Text-01, M1, M2, M2.5 等）已支持标准 JSON tool calling，
// 仅对旧模型（abab 系列）执行 XML 解析.
func (p *MiniMaxProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	upstream, err := p.Provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	// New models use standard JSON tool calling; skip XML parsing.
	if !isLegacyModel(req.Model) && !isLegacyModel(p.Cfg.DefaultModel) && !isLegacyModel(p.Cfg.FallbackModel) {
		return upstream, nil
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

// isLegacyModel returns true for old MiniMax models (abab series) that use XML tool call format.
func isLegacyModel(model string) bool {
	return strings.HasPrefix(model, "abab")
}

// xmlToolCallPayload 用于反序列化 <tool_calls> 标签内的 JSON.
type xmlToolCallPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// parseXMLToolCall 从 MiniMax 的 XML 格式 content 中提取 tool call.
// 格式: <tool_calls>\n{"name":"...","arguments":...}\n</tool_calls>
func parseXMLToolCall(content string) (types.ToolCall, bool) {
	const openTag = "<tool_calls>"
	const closeTag = "</tool_calls>"

	startIdx := strings.Index(content, openTag)
	endIdx := strings.Index(content, closeTag)
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return types.ToolCall{}, false
	}

	jsonStr := strings.TrimSpace(content[startIdx+len(openTag) : endIdx])
	if jsonStr == "" {
		return types.ToolCall{}, false
	}

	var payload xmlToolCallPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return types.ToolCall{}, false
	}

	return types.ToolCall{
		ID:        fmt.Sprintf("minimax_tc_%s", payload.Name),
		Name:      payload.Name,
		Arguments: payload.Arguments,
	}, true
}


