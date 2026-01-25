package middleware

import (
	"context"
	"testing"

	llmpkg "github.com/BaSui01/agentflow/llm"

	"github.com/stretchr/testify/assert"
)

func TestEmptyToolsCleaner_Rewrite(t *testing.T) {
	cleaner := NewEmptyToolsCleaner()

	tests := []struct {
		name           string
		req            *llmpkg.ChatRequest
		expectedChoice string
		description    string
	}{
		{
			name: "空工具数组应清除 tool_choice",
			req: &llmpkg.ChatRequest{
				Tools:      []llmpkg.ToolSchema{},
				ToolChoice: "auto",
			},
			expectedChoice: "",
			description:    "当 Tools 为空数组时，ToolChoice 应被清空",
		},
		{
			name: "nil 工具列表应清除 tool_choice",
			req: &llmpkg.ChatRequest{
				Tools:      nil,
				ToolChoice: "auto",
			},
			expectedChoice: "",
			description:    "当 Tools 为 nil 时，ToolChoice 应被清空",
		},
		{
			name: "非空工具列表不应清除 tool_choice",
			req: &llmpkg.ChatRequest{
				Tools: []llmpkg.ToolSchema{
					{Name: "test_tool", Description: "Test tool"},
				},
				ToolChoice: "auto",
			},
			expectedChoice: "auto",
			description:    "当 Tools 非空时，ToolChoice 应保持不变",
		},
		{
			name: "空 tool_choice 应保持不变",
			req: &llmpkg.ChatRequest{
				Tools:      []llmpkg.ToolSchema{},
				ToolChoice: "",
			},
			expectedChoice: "",
			description:    "当 ToolChoice 本身为空时，应保持不变",
		},
		{
			name:           "nil 请求应返回 nil",
			req:            nil,
			expectedChoice: "",
			description:    "当请求为 nil 时，应安全返回 nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cleaner.Rewrite(context.Background(), tt.req)

			assert.NoError(t, err, "Rewrite 不应返回错误")

			if tt.req != nil {
				assert.Equal(t, tt.expectedChoice, result.ToolChoice, tt.description)
			} else {
				assert.Nil(t, result, "nil 请求应返回 nil")
			}
		})
	}
}

func TestEmptyToolsCleaner_Name(t *testing.T) {
	cleaner := NewEmptyToolsCleaner()
	assert.Equal(t, "empty_tools_cleaner", cleaner.Name())
}

func TestRewriterChain_Execute(t *testing.T) {
	tests := []struct {
		name        string
		rewriters   []RequestRewriter
		req         *llmpkg.ChatRequest
		expectedErr bool
		description string
	}{
		{
			name:      "空链应直接返回原请求",
			rewriters: []RequestRewriter{},
			req: &llmpkg.ChatRequest{
				Tools:      []llmpkg.ToolSchema{},
				ToolChoice: "auto",
			},
			expectedErr: false,
			description: "空改写器链应直接返回原请求",
		},
		{
			name: "单个改写器应正常执行",
			rewriters: []RequestRewriter{
				NewEmptyToolsCleaner(),
			},
			req: &llmpkg.ChatRequest{
				Tools:      []llmpkg.ToolSchema{},
				ToolChoice: "auto",
			},
			expectedErr: false,
			description: "单个改写器应正常执行",
		},
		{
			name: "多个改写器应按顺序执行",
			rewriters: []RequestRewriter{
				NewEmptyToolsCleaner(),
				NewEmptyToolsCleaner(), // 重复执行应幂等
			},
			req: &llmpkg.ChatRequest{
				Tools:      []llmpkg.ToolSchema{},
				ToolChoice: "auto",
			},
			expectedErr: false,
			description: "多个改写器应按顺序执行",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewRewriterChain(tt.rewriters...)
			result, err := chain.Execute(context.Background(), tt.req)

			if tt.expectedErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, result, "结果不应为 nil")
			}
		})
	}
}

func TestRewriterChain_AddRewriter(t *testing.T) {
	chain := NewRewriterChain()
	assert.Equal(t, 0, len(chain.GetRewriters()), "初始链应为空")

	chain.AddRewriter(NewEmptyToolsCleaner())
	assert.Equal(t, 1, len(chain.GetRewriters()), "添加后链长度应为 1")

	chain.AddRewriter(NewEmptyToolsCleaner())
	assert.Equal(t, 2, len(chain.GetRewriters()), "再次添加后链长度应为 2")
}
