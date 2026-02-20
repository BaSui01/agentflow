// fixtures 中的 LLM 响应测试数据工厂。
//
// 提供常见响应、流式片段与错误场景样例。
package fixtures

import (
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// --- ChatResponse 工厂 ---

// SimpleResponse 返回简单的文本响应
func SimpleResponse(content string) *llm.ChatResponse {
	return &llm.ChatResponse{
		ID:       "resp-001",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: content,
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		CreatedAt: time.Now(),
	}
}

// ResponseWithUsage 返回带自定义 Token 使用量的响应
func ResponseWithUsage(content string, promptTokens, completionTokens int) *llm.ChatResponse {
	resp := SimpleResponse(content)
	resp.Usage = llm.ChatUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	return resp
}

// ResponseWithToolCalls 返回带工具调用的响应
func ResponseWithToolCalls(content string, toolCalls []types.ToolCall) *llm.ChatResponse {
	return &llm.ChatResponse{
		ID:       "resp-tool-001",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "tool_calls",
				Message: types.Message{
					Role:      types.RoleAssistant,
					Content:   content,
					ToolCalls: toolCalls,
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     50,
			CompletionTokens: 100,
			TotalTokens:      150,
		},
		CreatedAt: time.Now(),
	}
}

// ResponseWithSingleToolCall 返回带单个工具调用的响应
func ResponseWithSingleToolCall(content, toolName, toolID string, args []byte) *llm.ChatResponse {
	return ResponseWithToolCalls(content, []types.ToolCall{
		{
			ID:        toolID,
			Name:      toolName,
			Arguments: args,
		},
	})
}

// TruncatedResponse 返回因长度限制而截断的响应
func TruncatedResponse(content string) *llm.ChatResponse {
	resp := SimpleResponse(content)
	resp.Choices[0].FinishReason = "length"
	resp.Usage = llm.ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 4096,
		TotalTokens:      4196,
	}
	return resp
}

// ContentFilteredResponse 返回被内容过滤的响应
func ContentFilteredResponse() *llm.ChatResponse {
	return &llm.ChatResponse{
		ID:       "resp-filtered-001",
		Provider: "mock",
		Model:    "gpt-4",
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "content_filter",
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: "",
				},
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     50,
			CompletionTokens: 0,
			TotalTokens:      50,
		},
		CreatedAt: time.Now(),
	}
}

// --- StreamChunk 工厂 ---

// TextChunk 创建文本流式块
func TextChunk(content string, finishReason string) llm.StreamChunk {
	return llm.StreamChunk{
		ID:       "chunk-001",
		Provider: "mock",
		Model:    "gpt-4",
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: content,
		},
		FinishReason: finishReason,
	}
}

// ToolCallChunk 创建工具调用流式块
func ToolCallChunk(toolCall types.ToolCall, finishReason string) llm.StreamChunk {
	return llm.StreamChunk{
		ID:       "chunk-tool-001",
		Provider: "mock",
		Model:    "gpt-4",
		Delta: types.Message{
			Role:      types.RoleAssistant,
			ToolCalls: []types.ToolCall{toolCall},
		},
		FinishReason: finishReason,
	}
}

// ErrorChunk 创建错误流式块
func ErrorChunk(err *types.Error) llm.StreamChunk {
	return llm.StreamChunk{
		ID:           "chunk-error-001",
		Provider:     "mock",
		Model:        "gpt-4",
		FinishReason: "error",
		Err:          err,
	}
}

// SimpleStreamChunks 返回简单的流式块序列
func SimpleStreamChunks(content string, chunkSize int) []llm.StreamChunk {
	var chunks []llm.StreamChunk

	for i := 0; i < len(content); i += chunkSize {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := content[i:end]
		finishReason := ""
		if end >= len(content) {
			finishReason = "stop"
		}

		chunks = append(chunks, TextChunk(chunk, finishReason))
	}

	// 确保至少有一个块
	if len(chunks) == 0 {
		chunks = append(chunks, TextChunk("", "stop"))
	}

	return chunks
}

// WordByWordChunks 返回逐词的流式块序列
func WordByWordChunks(words []string) []llm.StreamChunk {
	chunks := make([]llm.StreamChunk, len(words))
	for i, word := range words {
		content := word
		if i < len(words)-1 {
			content += " "
		}
		finishReason := ""
		if i == len(words)-1 {
			finishReason = "stop"
		}
		chunks[i] = TextChunk(content, finishReason)
	}
	return chunks
}

// --- Token 使用量工厂 ---

// SmallUsage 返回小量 Token 使用
func SmallUsage() llm.ChatUsage {
	return llm.ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}
}

// MediumUsage 返回中等 Token 使用
func MediumUsage() llm.ChatUsage {
	return llm.ChatUsage{
		PromptTokens:     500,
		CompletionTokens: 1000,
		TotalTokens:      1500,
	}
}

// LargeUsage 返回大量 Token 使用
func LargeUsage() llm.ChatUsage {
	return llm.ChatUsage{
		PromptTokens:     4000,
		CompletionTokens: 4096,
		TotalTokens:      8096,
	}
}

// CustomUsage 返回自定义 Token 使用
func CustomUsage(prompt, completion int) llm.ChatUsage {
	return llm.ChatUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
}

// --- 预设响应场景 ---

// GreetingResponse 返回问候响应
func GreetingResponse() *llm.ChatResponse {
	return SimpleResponse("Hello! How can I assist you today?")
}

// CalculationResponse 返回计算响应
func CalculationResponse(result string) *llm.ChatResponse {
	return SimpleResponse("The result is: " + result)
}

// SearchResultResponse 返回搜索结果响应
func SearchResultResponse(results []string) *llm.ChatResponse {
	content := "Here are the search results:\n"
	for i, r := range results {
		content += string(rune('1'+i)) + ". " + r + "\n"
	}
	return SimpleResponse(content)
}

// ErrorExplanationResponse 返回错误解释响应
func ErrorExplanationResponse(errorMsg string) *llm.ChatResponse {
	return SimpleResponse("I encountered an error: " + errorMsg + ". Let me try a different approach.")
}

// ThinkingResponse 返回思考过程响应
func ThinkingResponse(thinking, conclusion string) *llm.ChatResponse {
	return SimpleResponse("Let me think about this...\n\n" + thinking + "\n\nConclusion: " + conclusion)
}

// RefusalResponse 返回拒绝响应
func RefusalResponse(reason string) *llm.ChatResponse {
	return SimpleResponse("I'm sorry, but I can't help with that request. " + reason)
}

// ClarificationResponse 返回澄清请求响应
func ClarificationResponse(question string) *llm.ChatResponse {
	return SimpleResponse("I need some clarification before I can help. " + question)
}
