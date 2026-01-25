package middleware

import (
	"context"

	llmpkg "github.com/BaSui01/agentflow/llm"
)

// EmptyToolsCleaner 空工具列表清理器
// 当请求的 Tools 为空时，清除 ToolChoice 字段
// 避免上游 API 返回 400 错误（OpenAI 不允许空 tools 数组时设置 tool_choice）
type EmptyToolsCleaner struct{}

// Name 返回改写器名称
func (r *EmptyToolsCleaner) Name() string {
	return "empty_tools_cleaner"
}

// Rewrite 执行改写
func (r *EmptyToolsCleaner) Rewrite(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatRequest, error) {
	if req == nil {
		return req, nil
	}

	// 如果 Tools 为空（nil 或空数组），清除 ToolChoice
	if len(req.Tools) == 0 {
		req.ToolChoice = ""
	}

	return req, nil
}

// NewEmptyToolsCleaner 创建空工具清理器
func NewEmptyToolsCleaner() *EmptyToolsCleaner {
	return &EmptyToolsCleaner{}
}
