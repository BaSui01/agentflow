package middleware

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ParseXMLToolCalls (同步解析) ---

func TestParseXMLToolCalls_NoTags(t *testing.T) {
	calls, cleaned, found := ParseXMLToolCalls("Hello, how can I help?")
	assert.False(t, found)
	assert.Nil(t, calls)
	assert.Equal(t, "Hello, how can I help?", cleaned)
}

func TestParseXMLToolCalls_SingleCall(t *testing.T) {
	content := `Let me check the weather.
<tool_calls>
{"name":"get_weather","arguments":{"city":"Beijing"}}
</tool_calls>`

	calls, cleaned, found := ParseXMLToolCalls(content)
	require.True(t, found)
	require.Len(t, calls, 1)
	assert.Equal(t, "get_weather", calls[0].Name)
	assert.NotEmpty(t, calls[0].ID)

	var args map[string]string
	json.Unmarshal(calls[0].Arguments, &args)
	assert.Equal(t, "Beijing", args["city"])

	assert.Equal(t, "Let me check the weather.", cleaned)
}

func TestParseXMLToolCalls_MultipleCallsInBlock(t *testing.T) {
	content := `<tool_calls>
{"name":"search","arguments":{"query":"golang"}}
{"name":"calc","arguments":{"expr":"1+1"}}
</tool_calls>`

	calls, cleaned, found := ParseXMLToolCalls(content)
	require.True(t, found)
	require.Len(t, calls, 2)
	assert.Equal(t, "search", calls[0].Name)
	assert.Equal(t, "calc", calls[1].Name)
	assert.Empty(t, cleaned)
}

func TestParseXMLToolCalls_MultipleBlocks(t *testing.T) {
	content := `First result.
<tool_calls>
{"name":"tool1","arguments":{}}
</tool_calls>
Then more text.
<tool_calls>
{"name":"tool2","arguments":{}}
</tool_calls>`

	calls, cleaned, found := ParseXMLToolCalls(content)
	require.True(t, found)
	require.Len(t, calls, 2)
	assert.Equal(t, "tool1", calls[0].Name)
	assert.Equal(t, "tool2", calls[1].Name)
	assert.Contains(t, cleaned, "First result.")
	assert.Contains(t, cleaned, "Then more text.")
}

func TestParseXMLToolCalls_EmptyBlock(t *testing.T) {
	content := "<tool_calls>\n\n</tool_calls>"
	calls, _, found := ParseXMLToolCalls(content)
	assert.False(t, found)
	assert.Nil(t, calls)
}

func TestParseXMLToolCalls_InvalidJSON(t *testing.T) {
	content := "<tool_calls>\nnot-json\n</tool_calls>"
	calls, _, found := ParseXMLToolCalls(content)
	assert.False(t, found)
	assert.Nil(t, calls)
}

func TestParseXMLToolCalls_MissingCloseTag(t *testing.T) {
	content := `<tool_calls>{"name":"test","arguments":{}}`
	calls, cleaned, found := ParseXMLToolCalls(content)
	assert.False(t, found)
	assert.Nil(t, calls)
	assert.Equal(t, content, cleaned)
}

func TestParseXMLToolCalls_NoName(t *testing.T) {
	content := `<tool_calls>
{"arguments":{"key":"val"}}
</tool_calls>`
	calls, _, found := ParseXMLToolCalls(content)
	assert.False(t, found)
	assert.Nil(t, calls)
}

// --- XMLToolCallStreamParser (流式解析) ---

func TestStreamParser_SimpleComplete(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 一次性接收完整块
	pass, calls := p.Feed(`<tool_calls>
{"name":"search","arguments":{"q":"test"}}
</tool_calls>`)

	assert.Empty(t, pass)
	require.Len(t, calls, 1)
	assert.Equal(t, "search", calls[0].Name)
}

func TestStreamParser_SplitAcrossChunks(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// chunk 1: 开始标签被拆分
	pass1, calls1 := p.Feed("Hello! <tool_")
	// 部分标签缓冲，"Hello! " 应该输出
	assert.Equal(t, "Hello! ", pass1)
	assert.Nil(t, calls1)

	// chunk 2: 完成标签 + 部分内容
	pass2, calls2 := p.Feed(`calls>
{"name":"get_weather","arguments":{"city":"NYC"}}
</tool_calls>`)

	assert.Empty(t, pass2)
	require.Len(t, calls2, 1)
	assert.Equal(t, "get_weather", calls2[0].Name)
}

func TestStreamParser_TextBeforeAndAfter(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 前面有普通文本
	pass1, calls1 := p.Feed("Let me search. ")
	assert.Equal(t, "Let me search. ", pass1)
	assert.Nil(t, calls1)

	// 工具调用块
	pass2, calls2 := p.Feed(`<tool_calls>
{"name":"search","arguments":{"q":"test"}}
</tool_calls> Done!`)

	assert.Equal(t, " Done!", pass2)
	require.Len(t, calls2, 1)
}

func TestStreamParser_NoToolCalls(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	pass, calls := p.Feed("Just regular text, no tools here.")
	assert.Equal(t, "Just regular text, no tools here.", pass)
	assert.Nil(t, calls)
}

func TestStreamParser_Flush(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 喂入不完整的标签
	p.Feed("<tool_")
	remaining := p.Flush()
	assert.Equal(t, "<tool_", remaining)
}

func TestStreamParser_MultipleCallsStreamed(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 两个工具调用在同一个块中逐步到达
	p.Feed("<tool_calls>\n")
	pass, calls := p.Feed(`{"name":"a","arguments":{}}
{"name":"b","arguments":{}}
</tool_calls>`)

	assert.Empty(t, pass)
	require.Len(t, calls, 2)
	assert.Equal(t, "a", calls[0].Name)
	assert.Equal(t, "b", calls[1].Name)
}

// --- findPartialPrefix ---

func TestFindPartialPrefix(t *testing.T) {
	tests := []struct {
		text     string
		tag      string
		expected int
	}{
		{"hello<tool_", "<tool_calls>", 5},   // "<tool_" 是 <tool_calls> 的前缀
		{"hello<tool_calls>", "<tool_calls>", -1}, // 完整标签，不是部分前缀
		{"hello", "<tool_calls>", -1},         // 无部分前缀
		{"<", "<tool_calls>", 0},              // 只有 "<"
		{"no match", "<tool_calls>", -1},
	}

	for _, tt := range tests {
		result := findPartialPrefix(tt.text, tt.tag)
		assert.Equal(t, tt.expected, result, "text=%q tag=%q", tt.text, tt.tag)
	}
}

// --- Fix 7c: 同步解析器跨 block ID 唯一性 ---

func TestParseXMLToolCalls_CrossBlockIDUniqueness(t *testing.T) {
	// 两个 block 中有同名工具 "search"，ID 不应碰撞
	content := `<tool_calls>
{"name":"search","arguments":{"q":"a"}}
</tool_calls>
middle text
<tool_calls>
{"name":"search","arguments":{"q":"b"}}
</tool_calls>`

	calls, _, found := ParseXMLToolCalls(content)
	require.True(t, found)
	require.Len(t, calls, 2)

	// 两个同名工具的 ID 必须不同
	assert.NotEqual(t, calls[0].ID, calls[1].ID,
		"跨 block 同名工具 ID 不应碰撞: %s vs %s", calls[0].ID, calls[1].ID)
}

// --- Fix 7d: 流式缓冲区溢出保护 ---

func TestStreamParser_BufferOverflowProtection(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 发送 <tool_calls> 开始标签
	pass1, calls1 := p.Feed("<tool_calls>")
	assert.Empty(t, pass1)
	assert.Nil(t, calls1)

	// 持续喂入大量数据但不关闭标签，模拟 LLM 不关闭 </tool_calls> 的场景
	bigChunk := strings.Repeat("x", maxXMLToolBufferSize+1)
	pass2, calls2 := p.Feed(bigChunk)

	// 超限后应 flush 为 passthrough，不应死等结束标签
	assert.Nil(t, calls2, "超限后不应产生工具调用")
	assert.NotEmpty(t, pass2, "超限后缓冲内容应作为 passthrough 输出")
	assert.Contains(t, pass2, "<tool_calls>", "flush 的内容应包含原始开始标签")

	// 解析器状态应已重置，后续正常内容可以通过
	pass3, calls3 := p.Feed("normal text after overflow")
	assert.Equal(t, "normal text after overflow", pass3)
	assert.Nil(t, calls3)
}

// --- Fix 7e: 流式解析器跨 Feed 调用 ID 唯一性 ---

func TestStreamParser_CrossFeedIDUniqueness(t *testing.T) {
	p := NewXMLToolCallStreamParser()

	// 第一次 Feed：完整的工具调用块
	_, calls1 := p.Feed(`<tool_calls>
{"name":"search","arguments":{"q":"first"}}
</tool_calls>`)
	require.Len(t, calls1, 1)

	// 第二次 Feed：又一个同名工具调用块
	_, calls2 := p.Feed(`<tool_calls>
{"name":"search","arguments":{"q":"second"}}
</tool_calls>`)
	require.Len(t, calls2, 1)

	// 跨 Feed 的同名工具 ID 必须不同
	assert.NotEqual(t, calls1[0].ID, calls2[0].ID,
		"跨 Feed 同名工具 ID 不应碰撞: %s vs %s", calls1[0].ID, calls2[0].ID)
}
