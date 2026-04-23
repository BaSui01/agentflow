package middleware

import (
	"context"
	"fmt"

	llmpkg "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// XMLToolCallProvider 包装内部 Provider，在 XML 工具调用模式下：
//   - 请求前：将工具定义注入 system prompt，清除 req.Tools（避免 Provider 报错）
//   - 响应后：从文本中解析 <tool_calls> 块，转换为标准 ToolCalls
//
// 对 ReAct 循环完全透明——它只看到标准的 ToolCalls。
type XMLToolCallProvider struct {
	inner    llmpkg.Provider
	rewriter *XMLToolRewriter
	logger   *zap.Logger // Fix 5: 日志可观测性
}

// NewXMLToolCallProvider 创建 XML 工具调用 Provider 包装器。
// logger 可为 nil，此时使用 zap.NewNop()。
func NewXMLToolCallProvider(inner llmpkg.Provider, logger *zap.Logger) *XMLToolCallProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &XMLToolCallProvider{
		inner:    inner,
		rewriter: NewXMLToolRewriter(),
		logger:   logger,
	}
}

// rewriteRequest 在 XML 模式下改写请求：注入工具定义到 system prompt，清除 req.Tools
func (p *XMLToolCallProvider) rewriteRequest(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatRequest, error) {
	if req.ToolCallMode != llmpkg.ToolCallModeXML {
		return req, nil
	}
	return p.rewriter.Rewrite(ctx, req)
}

// Completion 执行同步补全，在 XML 模式下先注入 prompt 再解析响应
func (p *XMLToolCallProvider) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	req, err := p.rewriteRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := p.inner.Completion(ctx, req)
	if err != nil {
		return nil, err
	}

	// 仅在 XML 模式下做后处理
	if req.ToolCallMode != llmpkg.ToolCallModeXML {
		return resp, nil
	}

	// 遍历所有 choices，解析 content 中的工具调用
	for i := range resp.Choices {
		content := resp.Choices[i].Message.Content
		if content == "" {
			continue
		}

		toolCalls, cleanedContent, found := ParseXMLToolCalls(content)
		if found {
			resp.Choices[i].Message.ToolCalls = append(resp.Choices[i].Message.ToolCalls, toolCalls...)
			resp.Choices[i].Message.Content = cleanedContent
			// Fix 4: 仅当 finish_reason 为 "stop" 或空（正常结束）时覆盖为 "tool_calls"，
			// 保留 "length"（token 超限）、"content_filter"（内容过滤）等异常原因，
			// 让上层能正确识别非正常终止场景。
			if resp.Choices[i].FinishReason == "stop" || resp.Choices[i].FinishReason == "" {
				resp.Choices[i].FinishReason = "tool_calls"
			}
			p.logger.Debug("parsed XML tool calls from completion",
				zap.Int("choice_index", i),
				zap.Int("tool_call_count", len(toolCalls)),
				zap.String("finish_reason", resp.Choices[i].FinishReason))
		}
	}

	return resp, nil
}

// Stream 执行流式补全，在 XML 模式下先注入 prompt 再用流式解析器处理
func (p *XMLToolCallProvider) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	req, err := p.rewriteRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	upstream, err := p.inner.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	// 非 XML 模式直接透传
	if req.ToolCallMode != llmpkg.ToolCallModeXML {
		return upstream, nil
	}

	out := make(chan llmpkg.StreamChunk)
	go func() {
		defer close(out)

		// Fix 1: panic 恢复 —— parser panic 时发送 error chunk 而非死锁
		defer func() {
			if r := recover(); r != nil {
				p.logger.Error("panic recovered in XML stream parser goroutine",
					zap.Any("panic", r))
				out <- llmpkg.StreamChunk{
					Err: types.NewError(types.ErrInternalError,
						fmt.Sprintf("xml stream parser panic: %v", r)),
				}
			}
		}()

		parser := NewXMLToolCallStreamParser()

		for chunk := range upstream {
			if chunk.Err != nil {
				out <- chunk
				continue
			}

			content := chunk.Delta.Content
			if content == "" {
				out <- chunk
				continue
			}

			passthrough, toolCalls := parser.Feed(content)

			// 有工具调用时注入
			if len(toolCalls) > 0 {
				chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, toolCalls...)
				chunk.Delta.Content = passthrough
				// Fix 4: 仅当 finish_reason 为 "stop" 或空（正常结束）时覆盖为 "tool_calls"，
				// 保留 "length"（token 超限）、"content_filter"（内容过滤）等异常原因。
				if chunk.FinishReason == "stop" || chunk.FinishReason == "" {
					chunk.FinishReason = "tool_calls"
				}
				p.logger.Debug("parsed XML tool calls from stream",
					zap.Int("tool_call_count", len(toolCalls)),
					zap.String("finish_reason", chunk.FinishReason))
				out <- chunk
				continue
			}

			// 有 passthrough 文本时输出
			if passthrough != "" {
				chunk.Delta.Content = passthrough
				out <- chunk
			}
			// passthrough 为空说明内容在缓冲中，不发送
		}

		// 流结束，刷新剩余缓冲
		remaining := parser.Flush()
		if remaining != "" {
			out <- llmpkg.StreamChunk{
				Delta: types.Message{Content: remaining},
			}
		}
	}()

	return out, nil
}

// --- 透传方法 ---

func (p *XMLToolCallProvider) Name() string { return p.inner.Name() }

func (p *XMLToolCallProvider) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

func (p *XMLToolCallProvider) SupportsNativeFunctionCalling() bool {
	return p.inner.SupportsNativeFunctionCalling()
}

func (p *XMLToolCallProvider) ListModels(ctx context.Context) ([]llmpkg.Model, error) {
	return p.inner.ListModels(ctx)
}

func (p *XMLToolCallProvider) Endpoints() llmpkg.ProviderEndpoints {
	return p.inner.Endpoints()
}
