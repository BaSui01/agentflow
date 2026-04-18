package middleware

import (
	"context"
	"encoding/json"
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXMLToolRewriter_NilRequest(t *testing.T) {
	rw := NewXMLToolRewriter()
	result, err := rw.Rewrite(context.Background(), nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestXMLToolRewriter_NativeMode_NoOp(t *testing.T) {
	rw := NewXMLToolRewriter()
	tools := []types.ToolSchema{{Name: "search", Description: "search tool"}}

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeNative,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "hi"}},
		Tools:        tools,
	}

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, result.Tools, 1, "Native 模式下工具不应被清除")
}

func TestXMLToolRewriter_NoTools_NoOp(t *testing.T) {
	rw := NewXMLToolRewriter()

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages:     []types.Message{{Role: types.RoleUser, Content: "hi"}},
		Tools:        nil,
	}

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, result.Tools, "无工具时应为 no-op")
}

func TestXMLToolRewriter_XMLMode_InjectsSystemPrompt(t *testing.T) {
	rw := NewXMLToolRewriter()
	tools := []types.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages: []types.Message{
			{Role: types.RoleSystem, Content: "You are a helpful assistant."},
			{Role: types.RoleUser, Content: "What is the weather?"},
		},
		Tools:      tools,
		ToolChoice: &types.ToolChoice{Mode: types.ToolChoiceModeAuto},
	}

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)

	// 工具定义应被注入到 system prompt
	assert.Contains(t, result.Messages[0].Content, "get_weather")
	assert.Contains(t, result.Messages[0].Content, "<tool_calls>")
	assert.Contains(t, result.Messages[0].Content, "You are a helpful assistant.")

	// Tools 和 ToolChoice 应被清除
	assert.Nil(t, result.Tools)
	assert.Nil(t, result.ToolChoice)
}

func TestXMLToolRewriter_XMLMode_NoSystemMessage_CreatesOne(t *testing.T) {
	rw := NewXMLToolRewriter()
	tools := []types.ToolSchema{
		{Name: "calc", Description: "Calculator"},
	}

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages: []types.Message{
			{Role: types.RoleUser, Content: "1+1=?"},
		},
		Tools: tools,
	}

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)

	// 应在消息列表开头创建 system 消息
	require.Len(t, result.Messages, 2)
	assert.Equal(t, types.RoleSystem, result.Messages[0].Role)
	assert.Contains(t, result.Messages[0].Content, "calc")
	assert.Equal(t, "1+1=?", result.Messages[1].Content)
}

func TestXMLToolRewriter_Name(t *testing.T) {
	rw := NewXMLToolRewriter()
	assert.Equal(t, "xml_tool_rewriter", rw.Name())
}

// --- Fix 7f: Rewriter 不修改原始输入 ---

func TestXMLToolRewriter_InputImmutability(t *testing.T) {
	rw := NewXMLToolRewriter()
	tools := []types.ToolSchema{
		{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
	}

	originalSystemContent := "You are a helpful assistant."
	originalUserContent := "What is the weather?"

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages: []types.Message{
			{Role: types.RoleSystem, Content: originalSystemContent},
			{Role: types.RoleUser, Content: originalUserContent},
		},
		Tools:      tools,
		ToolChoice: &types.ToolChoice{Mode: types.ToolChoiceModeAuto},
	}

	// 保存原始 Messages slice 的长度和内容
	origMsgLen := len(req.Messages)
	origToolsLen := len(req.Tools)

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)

	// 返回的应该是不同的指针
	assert.NotSame(t, req, result, "Rewrite 应返回新的 ChatRequest 指针")

	// 原始 req 不应被修改
	assert.Len(t, req.Messages, origMsgLen, "原始 Messages 长度不应改变")
	assert.Len(t, req.Tools, origToolsLen, "原始 Tools 不应被清除")
	require.NotNil(t, req.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeAuto, req.ToolChoice.Mode, "原始 ToolChoice 不应被清除")
	assert.Equal(t, originalSystemContent, req.Messages[0].Content,
		"原始 system message 内容不应被修改")
	assert.Equal(t, originalUserContent, req.Messages[1].Content,
		"原始 user message 内容不应被修改")

	// 结果应该被正确改写
	assert.Contains(t, result.Messages[0].Content, "get_weather")
	assert.Nil(t, result.Tools)
	assert.Nil(t, result.ToolChoice)
}

func TestXMLToolRewriter_InputImmutability_NoSystemMessage(t *testing.T) {
	rw := NewXMLToolRewriter()
	tools := []types.ToolSchema{
		{Name: "calc", Description: "Calculator"},
	}

	req := &llmpkg.ChatRequest{
		ToolCallMode: llmpkg.ToolCallModeXML,
		Messages: []types.Message{
			{Role: types.RoleUser, Content: "1+1=?"},
		},
		Tools: tools,
	}

	origMsgLen := len(req.Messages)

	result, err := rw.Rewrite(context.Background(), req)
	require.NoError(t, err)

	// 原始 req 的 Messages 长度不应改变（不应被 prepend system 消息）
	assert.Len(t, req.Messages, origMsgLen, "原始 Messages 不应被 prepend")
	assert.Equal(t, "1+1=?", req.Messages[0].Content)

	// 结果应有 2 条消息（新增的 system + 原始 user）
	require.Len(t, result.Messages, 2)
	assert.Equal(t, types.RoleSystem, result.Messages[0].Role)
}
