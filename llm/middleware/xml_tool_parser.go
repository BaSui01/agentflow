package middleware

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/BaSui01/agentflow/types"
)

const (
	xmlToolCallOpenTag  = "<tool_calls>"
	xmlToolCallCloseTag = "</tool_calls>"

	// maxXMLToolBufferSize 流式缓冲区上限（256KB）。
	// LLM 不关闭 </tool_calls> 标签时，防止 buffer 无限增长。
	maxXMLToolBufferSize = 256 * 1024
)

// xmlToolCallPayload 用于反序列化 <tool_calls> 标签内的 JSON
type xmlToolCallPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ParseXMLToolCalls 从文本中提取所有 <tool_calls> 块，解析为标准 ToolCall 切片。
// 返回解析出的工具调用、清理后的文本（移除了 <tool_calls> 块）、以及是否找到。
func ParseXMLToolCalls(content string) ([]types.ToolCall, string, bool) {
	if !strings.Contains(content, xmlToolCallOpenTag) {
		return nil, content, false
	}

	var toolCalls []types.ToolCall
	cleaned := content
	// Fix 2: 用贯穿整个调用的计数器替代 len(calls)，避免跨 block 同名工具 ID 碰撞
	idCounter := 0

	for {
		startIdx := strings.Index(cleaned, xmlToolCallOpenTag)
		if startIdx < 0 {
			break
		}
		endIdx := strings.Index(cleaned[startIdx:], xmlToolCallCloseTag)
		if endIdx < 0 {
			break
		}
		endIdx += startIdx // 转为绝对位置

		// 提取标签内内容
		inner := strings.TrimSpace(cleaned[startIdx+len(xmlToolCallOpenTag) : endIdx])

		// 解析内部的 JSON 行（支持多个工具调用）
		if inner != "" {
			calls := parseToolCallLines(inner, &idCounter)
			toolCalls = append(toolCalls, calls...)
		}

		// 从 cleaned 中移除这个 <tool_calls> 块
		cleaned = cleaned[:startIdx] + cleaned[endIdx+len(xmlToolCallCloseTag):]
	}

	cleaned = strings.TrimSpace(cleaned)

	return toolCalls, cleaned, len(toolCalls) > 0
}

// parseToolCallLines 解析 <tool_calls> 内的 JSON 行，每行一个工具调用。
// idCounter 是贯穿整个 ParseXMLToolCalls 调用的计数器，确保跨 block 的 ID 唯一性。
func parseToolCallLines(inner string, idCounter *int) []types.ToolCall {
	var calls []types.ToolCall

	lines := strings.Split(inner, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var payload xmlToolCallPayload
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if payload.Name == "" {
			continue
		}

		*idCounter++
		calls = append(calls, types.ToolCall{
			ID:        fmt.Sprintf("xmltc_%s_%d", payload.Name, *idCounter),
			Name:      payload.Name,
			Arguments: payload.Arguments,
		})
	}

	return calls
}

// XMLToolCallStreamParser 有状态的流式 XML 工具调用解析器。
// 支持跨 chunk 的部分标签缓冲，当检测到完整 <tool_calls>...</tool_calls> 块时
// 解析并返回工具调用。
type XMLToolCallStreamParser struct {
	buffer    strings.Builder
	inBlock   bool     // 当前是否在 <tool_calls> 块内
	idCounter atomic.Int64
}

// NewXMLToolCallStreamParser 创建流式解析器
func NewXMLToolCallStreamParser() *XMLToolCallStreamParser {
	return &XMLToolCallStreamParser{}
}

// Feed 接收一个 chunk 的文本内容，返回：
//   - passthrough: 应直接传递给下游的普通文本
//   - toolCalls:   本次 chunk 中解析出的完整工具调用
//   - 注意: passthrough 可能为空（内容全部被缓冲或属于 tool_calls 块内部）
func (p *XMLToolCallStreamParser) Feed(text string) (passthrough string, toolCalls []types.ToolCall) {
	p.buffer.WriteString(text)
	buf := p.buffer.String()

	var passThroughParts []string

	for {
		if !p.inBlock {
			// 查找 <tool_calls> 开始标签
			idx := strings.Index(buf, xmlToolCallOpenTag)
			if idx < 0 {
				// 检查是否有部分标签在缓冲末尾（如 "<tool_" ）
				partialIdx := findPartialPrefix(buf, xmlToolCallOpenTag)
				if partialIdx >= 0 && partialIdx < len(buf) {
					// 将非标签部分输出，保留部分标签在缓冲区
					passThroughParts = append(passThroughParts, buf[:partialIdx])
					p.buffer.Reset()
					p.buffer.WriteString(buf[partialIdx:])
					break
				}
				// 没有任何标签痕迹，全部输出
				passThroughParts = append(passThroughParts, buf)
				p.buffer.Reset()
				break
			}

			// 输出标签前的文本
			if idx > 0 {
				passThroughParts = append(passThroughParts, buf[:idx])
			}
			p.inBlock = true
			buf = buf[idx+len(xmlToolCallOpenTag):]
		}

		// 在 <tool_calls> 块内，查找结束标签
		endIdx := strings.Index(buf, xmlToolCallCloseTag)
		if endIdx < 0 {
			// Fix 3: 缓冲区溢出保护 —— 超过 maxXMLToolBufferSize 时，
			// 说明 LLM 没有正确关闭 </tool_calls> 标签，将已缓冲内容
			// 作为普通文本 flush 出去，重置状态继续处理后续内容。
			if len(buf) > maxXMLToolBufferSize {
				passThroughParts = append(passThroughParts, xmlToolCallOpenTag+buf)
				p.buffer.Reset()
				p.inBlock = false
				buf = ""
				break
			}
			// 结束标签还没到，继续缓冲
			p.buffer.Reset()
			p.buffer.WriteString(buf)
			break
		}

		// 提取块内容并解析
		inner := strings.TrimSpace(buf[:endIdx])
		if inner != "" {
			idCtr := int(p.idCounter.Load())
			calls := parseToolCallLines(inner, &idCtr)
			p.idCounter.Store(int64(idCtr))
			toolCalls = append(toolCalls, calls...)
		}

		p.inBlock = false
		buf = buf[endIdx+len(xmlToolCallCloseTag):]
	}

	passthrough = strings.Join(passThroughParts, "")
	return
}

// Flush 刷新缓冲区中剩余的内容。在流结束时调用。
// 返回未被识别为工具调用的残留文本。
func (p *XMLToolCallStreamParser) Flush() string {
	remaining := p.buffer.String()
	p.buffer.Reset()
	p.inBlock = false
	return remaining
}

// findPartialPrefix 查找 text 末尾是否包含 tag 的部分前缀。
// 返回部分前缀开始的位置，-1 表示没找到。
func findPartialPrefix(text, tag string) int {
	// 从最长可能的部分前缀开始检查
	maxLen := len(tag) - 1
	if maxLen > len(text) {
		maxLen = len(text)
	}
	for l := maxLen; l >= 1; l-- {
		if strings.HasSuffix(text, tag[:l]) {
			return len(text) - l
		}
	}
	return -1
}
