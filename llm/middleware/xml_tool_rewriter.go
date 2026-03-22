package middleware

import (
	"context"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// XMLToolRewriter 在 XML 工具调用模式下，将工具定义注入 system prompt 并清除 req.Tools。
// 当 req.ToolCallMode != XML 或无工具时为 no-op。
type XMLToolRewriter struct{}

// NewXMLToolRewriter 创建 XML 工具改写器
func NewXMLToolRewriter() *XMLToolRewriter {
	return &XMLToolRewriter{}
}

// Name 返回改写器名称
func (r *XMLToolRewriter) Name() string {
	return "xml_tool_rewriter"
}

// Rewrite 执行改写：将工具定义注入 system prompt，清除 Tools 和 ToolChoice。
// Fix 6: 浅拷贝 ChatRequest + 深拷贝 Messages slice，不修改原始输入。
func (r *XMLToolRewriter) Rewrite(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatRequest, error) {
	if req == nil {
		return req, nil
	}

	// 仅在 XML 模式且有工具时生效
	if req.ToolCallMode != llmpkg.ToolCallModeXML || len(req.Tools) == 0 {
		return req, nil
	}

	// 浅拷贝 ChatRequest，避免修改调用方持有的原始对象
	copied := *req
	// 深拷贝 Messages slice，避免修改原始消息内容
	copied.Messages = make([]llmpkg.Message, len(req.Messages))
	copy(copied.Messages, req.Messages)

	// 生成工具描述文本
	toolsXML := FormatToolsAsXML(req.Tools)

	// 注入到 system prompt —— 找到第一条 system 消息追加，没有则新建
	injected := false
	for i, msg := range copied.Messages {
		if msg.Role == llmpkg.RoleSystem {
			copied.Messages[i].Content += toolsXML
			injected = true
			break
		}
	}
	if !injected {
		// 在消息列表开头插入 system 消息
		systemMsg := types.NewMessage(llmpkg.RoleSystem, toolsXML)
		copied.Messages = append([]llmpkg.Message{systemMsg}, copied.Messages...)
	}

	// 清除 Tools 和 ToolChoice —— 避免 Provider 报错
	copied.Tools = nil
	copied.ToolChoice = nil

	return &copied, nil
}
