package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ToolSchema 工具定义（本地定义避免循环依赖）
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatRequest 请求（本地定义避免循环依赖）
type ChatRequest struct {
	Messages  []Message    `json:"messages"`
	Tools     []ToolSchema `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
}

// Tokenizer 定义 token 计数接口。
type Tokenizer interface {
	// CountTokens 计算文本的 token 数量
	CountTokens(text string) int

	// CountMessageTokens 计算消息的 token 数量（包含角色、名称等元数据开销）
	CountMessageTokens(msg Message) int

	// CountMessagesTokens 计算消息列表的总 token 数
	CountMessagesTokens(msgs []Message) int

	// EstimateToolTokens 估算工具调用的 token 数
	EstimateToolTokens(tools []ToolSchema) int
}

// ====== 实现：EstimateTokenizer ======
// 基于字符数的简单估算，适用于所有模型（不依赖外部库）

const (
	// 平均 1 个 token ≈ 4 个字符（英文），中文约 1.5 个字符
	englishCharsPerToken = 4.0
	chineseCharsPerToken = 1.5

	// 消息元数据开销（角色、格式等）
	messageOverhead = 4 // 每条消息的固定开销

	// Tool Schema 开销（JSON 结构）
	toolSchemaOverhead = 10 // 每个工具的固定开销
)

type EstimateTokenizer struct {
	mu              sync.RWMutex
	modelMultiplier map[string]float64 // 不同模型的调整系数
}

// NewEstimateTokenizer 创建基于估算的 Tokenizer。
func NewEstimateTokenizer() *EstimateTokenizer {
	return &EstimateTokenizer{
		modelMultiplier: map[string]float64{
			"gpt-4":         1.0,
			"gpt-3.5-turbo": 1.0,
			"claude":        1.1,  // Claude 稍微更细粒度
			"gemini":        0.95, // Gemini 稍微更粗粒度
			"qwen":          1.0,
			"ernie":         1.0,
			"glm":           1.0,
		},
	}
}

func (t *EstimateTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	// 统计中文和英文字符
	var chineseCount, englishCount int
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FA5 { // 中文 Unicode 范围
			chineseCount++
		} else {
			englishCount++
		}
	}

	// 估算 tokens
	tokens := float64(chineseCount)/chineseCharsPerToken + float64(englishCount)/englishCharsPerToken
	return int(tokens) + 1 // 至少 1 个 token
}

func (t *EstimateTokenizer) CountMessageTokens(msg Message) int {
	tokens := messageOverhead

	// 计算 Content
	tokens += t.CountTokens(msg.Content)

	// 计算 Name
	if msg.Name != "" {
		tokens += t.CountTokens(msg.Name)
	}

	// 计算 ToolCalls
	for _, tc := range msg.ToolCalls {
		tokens += t.CountTokens(tc.Name)
		tokens += len(tc.Arguments) / 4 // 粗略估算 JSON 参数
	}

	// 计算 ToolCallID
	if msg.ToolCallID != "" {
		tokens += 1
	}

	return tokens
}

func (t *EstimateTokenizer) CountMessagesTokens(msgs []Message) int {
	total := 0
	for _, msg := range msgs {
		total += t.CountMessageTokens(msg)
	}
	return total
}

func (t *EstimateTokenizer) EstimateToolTokens(tools []ToolSchema) int {
	if len(tools) == 0 {
		return 0
	}

	total := 0
	for _, tool := range tools {
		// 工具名称
		total += t.CountTokens(tool.Name)

		// 工具描述
		total += t.CountTokens(tool.Description)

		// 参数 Schema（JSON）
		total += len(tool.Parameters) / 4

		// 固定开销
		total += toolSchemaOverhead
	}

	return total
}

// SetModelMultiplier 设置特定模型的调整系数。
func (t *EstimateTokenizer) SetModelMultiplier(model string, multiplier float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.modelMultiplier[model] = multiplier
}

// GetModelMultiplier 获取特定模型的调整系数。
func (t *EstimateTokenizer) GetModelMultiplier(model string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// 尝试精确匹配
	if mult, ok := t.modelMultiplier[model]; ok {
		return mult
	}

	// 尝试前缀匹配（例如 gpt-4-turbo -> gpt-4）
	for prefix, mult := range t.modelMultiplier {
		if strings.HasPrefix(model, prefix) {
			return mult
		}
	}

	return 1.0 // 默认系数
}

// ====== 实现：TikTokenizer（可选，需要引入 tiktoken-go）======
// 如果需要精确的 token 计数，可以使用 tiktoken-go 库
// 这里提供接口定义，实际实现需要引入第三方库

// TikTokenizer 基于 tiktoken 的精确 Tokenizer（需要 tiktoken-go 库）
// type TikTokenizer struct {
//     encoder tiktoken.Encoder
// }
//
// func NewTikTokenizer(model string) (*TikTokenizer, error) {
//     encoder, err := tiktoken.EncodingForModel(model)
//     if err != nil {
//         return nil, err
//     }
//     return &TikTokenizer{encoder: encoder}, nil
// }
//
// func (t *TikTokenizer) CountTokens(text string) int {
//     tokens := t.encoder.Encode(text, nil, nil)
//     return len(tokens)
// }

// ====== 工具函数 ======

// CountRequestTokens 计算完整请求的 token 数（消息 + 工具）。
func CountRequestTokens(req *ChatRequest, tokenizer Tokenizer) int {
	total := 0

	// 消息 tokens
	total += tokenizer.CountMessagesTokens(req.Messages)

	// 工具 tokens
	if len(req.Tools) > 0 {
		total += tokenizer.EstimateToolTokens(req.Tools)
	}

	return total
}

// EstimateResponseTokens 估算响应的 token 数（基于 MaxTokens）。
func EstimateResponseTokens(req *ChatRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 1000 // 默认估算
}

// TotalRequestTokens 计算请求的总 token 预算（输入 + 输出）。
func TotalRequestTokens(req *ChatRequest, tokenizer Tokenizer) int {
	inputTokens := CountRequestTokens(req, tokenizer)
	outputTokens := EstimateResponseTokens(req)
	return inputTokens + outputTokens
}

// FormatToolResultAsMessage 将 ToolResult 格式化为 Message（工具函数）。
func FormatToolResultAsMessage(toolCallID, toolName string, result json.RawMessage, err error) Message {
	msg := Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Name:       toolName,
	}

	if err != nil {
		msg.Content = fmt.Sprintf("Error: %s", err.Error())
	} else {
		msg.Content = string(result)
	}

	return msg
}
