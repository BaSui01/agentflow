// MockProvider 的 LLM 提供商测试模拟实现。
//
// 支持固定响应、流式输出与错误注入场景。
package mocks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// --- MockProvider 结构 ---

// GenerateRequest 是 E2E 测试使用的简化请求类型。
// 它将被内部转换为 llm.ChatRequest。
type GenerateRequest struct {
	Messages []types.Message    `json:"messages"`
	Model    string             `json:"model,omitempty"`
	Tools    []types.ToolSchema `json:"tools,omitempty"`
	Stream   bool               `json:"stream,omitempty"`
}

// GenerateResponse 是 E2E 测试使用的简化响应类型。
// 它从 llm.ChatResponse 中提取关键字段。
type GenerateResponse struct {
	Content   string           `json:"content"`
	ToolCalls []types.ToolCall `json:"tool_calls,omitempty"`
}

// GenerateStreamChunk 是 E2E 测试使用的简化流式块类型。
type GenerateStreamChunk struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// MockProvider 是 LLM Provider 的模拟实现
type MockProvider struct {
	mu sync.RWMutex

	// 响应配置
	response     string
	streamChunks []string
	toolCalls    []types.ToolCall
	err          error

	// Token 使用统计
	promptTokens     int
	completionTokens int

	// 调用记录
	calls           []MockProviderCall
	completionFunc  func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFunc      func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
	generateFunc    func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// 行为控制
	delay        int // 模拟延迟（毫秒）
	failAfter    int // 在第 N 次调用后失败
	callCount    int
	shouldStream bool
}

// MockProviderCall 记录单次调用
type MockProviderCall struct {
	Request  *llm.ChatRequest
	Response *llm.ChatResponse
	Error    error
}

// --- 构造函数和 Builder 方法 ---

// NewMockProvider 创建新的 MockProvider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		response:         "Mock response",
		streamChunks:     []string{},
		toolCalls:        []types.ToolCall{},
		calls:            []MockProviderCall{},
		promptTokens:     10,
		completionTokens: 20,
	}
}

// WithResponse 设置固定响应内容
func (m *MockProvider) WithResponse(response string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.response = response
	return m
}

// WithError 设置返回错误
func (m *MockProvider) WithError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
	return m
}

// WithStreamChunks 设置流式响应块
func (m *MockProvider) WithStreamChunks(chunks []string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamChunks = chunks
	m.shouldStream = true
	return m
}

// WithToolCalls 设置工具调用响应
func (m *MockProvider) WithToolCalls(toolCalls []types.ToolCall) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls = toolCalls
	return m
}

// WithTokenUsage 设置 Token 使用量
func (m *MockProvider) WithTokenUsage(prompt, completion int) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promptTokens = prompt
	m.completionTokens = completion
	return m
}

// WithDelay 设置响应延迟（毫秒）
func (m *MockProvider) WithDelay(ms int) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = ms
	return m
}

// WithFailAfter 设置在第 N 次调用后失败
func (m *MockProvider) WithFailAfter(n int) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failAfter = n
	return m
}

// WithCompletionFunc 设置自定义 Completion 函数
func (m *MockProvider) WithCompletionFunc(fn func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completionFunc = fn
	return m
}

// WithStreamFunc 设置自定义 Stream 函数
func (m *MockProvider) WithStreamFunc(fn func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamFunc = fn
	return m
}

// WithGenerateFunc 设置自定义 Generate 函数（E2E 测试用简化接口）
func (m *MockProvider) WithGenerateFunc(fn func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generateFunc = fn
	return m
}

// --- Provider 接口实现 ---

// Name 返回 Provider 名称
func (m *MockProvider) Name() string {
	return "mock"
}

// SupportsNativeFunctionCalling 返回是否支持原生函数调用
func (m *MockProvider) SupportsNativeFunctionCalling() bool {
	return true
}

// ListModels 返回可用模型列表
func (m *MockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	_ = ctx
	return []llm.Model{
		{
			ID:      "mock-model",
			Object:  "model",
			OwnedBy: "mock",
		},
	}, nil
}

// HealthCheck 执行健康检查
func (m *MockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{
		Healthy:   true,
		Latency:   10 * time.Millisecond,
		ErrorRate: 0,
	}, nil
}

// Endpoints 返回 Provider 使用的 API 端点信息
func (m *MockProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{
		Completion: "http://mock-provider/v1/chat/completions",
		Stream:     "http://mock-provider/v1/chat/completions",
		Models:     "http://mock-provider/v1/models",
		Health:     "http://mock-provider/v1/health",
		BaseURL:    "http://mock-provider",
	}
}

// Completion 生成响应
func (m *MockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	// 检查是否应该失败
	if m.failAfter > 0 && m.callCount > m.failAfter {
		err := errors.New("mock provider: configured to fail after N calls")
		m.calls = append(m.calls, MockProviderCall{Request: req, Error: err})
		return nil, err
	}

	// 检查是否有预设错误
	if m.err != nil {
		m.calls = append(m.calls, MockProviderCall{Request: req, Error: m.err})
		return nil, m.err
	}

	// 使用自定义函数
	if m.completionFunc != nil {
		resp, err := m.completionFunc(ctx, req)
		m.calls = append(m.calls, MockProviderCall{Request: req, Response: resp, Error: err})
		return resp, err
	}

	// 构建默认响应
	msg := types.Message{
		Role:      types.RoleAssistant,
		Content:   m.response,
		ToolCalls: m.toolCalls,
	}

	resp := &llm.ChatResponse{
		ID:       "mock-response-id",
		Provider: "mock",
		Model:    req.Model,
		Choices: []llm.ChatChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      msg,
			},
		},
		Usage: llm.ChatUsage{
			PromptTokens:     m.promptTokens,
			CompletionTokens: m.completionTokens,
			TotalTokens:      m.promptTokens + m.completionTokens,
		},
		CreatedAt: time.Now(),
	}

	if len(m.toolCalls) > 0 {
		resp.Choices[0].FinishReason = "tool_calls"
	}

	m.calls = append(m.calls, MockProviderCall{Request: req, Response: resp})
	return resp, nil
}

// Stream 流式生成响应
func (m *MockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	// 检查是否有预设错误
	if m.err != nil {
		return nil, m.err
	}

	// 使用自定义函数
	if m.streamFunc != nil {
		return m.streamFunc(ctx, req)
	}

	// 创建流式响应通道
	ch := make(chan llm.StreamChunk, len(m.streamChunks)+1)

	go func() {
		defer close(ch)

		// 如果没有设置流式块，使用完整响应
		if len(m.streamChunks) == 0 {
			ch <- llm.StreamChunk{
				ID:       "mock-chunk-id",
				Provider: "mock",
				Model:    req.Model,
				Delta: types.Message{
					Role:    types.RoleAssistant,
					Content: m.response,
				},
				FinishReason: "stop",
			}
			return
		}

		// 发送流式块
		for i, chunk := range m.streamChunks {
			select {
			case <-ctx.Done():
				return
			case ch <- llm.StreamChunk{
				ID:       "mock-chunk-id",
				Provider: "mock",
				Model:    req.Model,
				Index:    i,
				Delta: types.Message{
					Role:    types.RoleAssistant,
					Content: chunk,
				},
				FinishReason: func() string {
					if i == len(m.streamChunks)-1 {
						return "stop"
					}
					return ""
				}(),
			}:
			}
		}
	}()

	return ch, nil
}

// --- E2E 测试简化接口 ---

// Generate 是 E2E 测试使用的简化生成接口。
// 内部将 GenerateRequest 转换为 llm.ChatRequest 并调用 Completion。
func (m *MockProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	m.mu.Lock()

	// 如果设置了自定义 generateFunc，使用它
	if m.generateFunc != nil {
		fn := m.generateFunc
		m.mu.Unlock()
		return fn(ctx, req)
	}
	m.mu.Unlock()

	// 转换为 ChatRequest 并调用 Completion
	chatReq := &llm.ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
	}

	chatResp, err := m.Completion(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// 从 ChatResponse 提取简化响应
	resp := &GenerateResponse{}
	if len(chatResp.Choices) > 0 {
		resp.Content = chatResp.Choices[0].Message.Content
		resp.ToolCalls = chatResp.Choices[0].Message.ToolCalls
	}
	return resp, nil
}

// StreamGenerate 是 E2E 测试使用的简化流式接口。
// 内部将 GenerateRequest 转换为 llm.ChatRequest 并调用 Stream，
// 返回简化的 GenerateStreamChunk channel。
func (m *MockProvider) StreamGenerate(ctx context.Context, req *GenerateRequest) (<-chan GenerateStreamChunk, error) {
	chatReq := &llm.ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
	}

	llmCh, err := m.Stream(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan GenerateStreamChunk, cap(llmCh))
	go func() {
		defer close(ch)
		for chunk := range llmCh {
			select {
			case <-ctx.Done():
				return
			case ch <- GenerateStreamChunk{
				Content:      chunk.Delta.Content,
				FinishReason: chunk.FinishReason,
			}:
			}
		}
	}()

	return ch, nil
}

// --- 查询方法 ---

// GetCalls 获取所有调用记录
func (m *MockProvider) GetCalls() []MockProviderCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]MockProviderCall{}, m.calls...)
}

// GetCallCount 获取调用次数
func (m *MockProvider) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

// GetLastCall 获取最后一次调用
func (m *MockProvider) GetLastCall() *MockProviderCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.calls) == 0 {
		return nil
	}
	call := m.calls[len(m.calls)-1]
	return &call
}

// Reset 重置所有状态
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = []MockProviderCall{}
	m.callCount = 0
	m.err = nil
}

// --- 预设 Provider 工厂 ---

// NewSuccessProvider 创建总是成功的 Provider
func NewSuccessProvider(response string) *MockProvider {
	return NewMockProvider().WithResponse(response)
}

// NewErrorProvider 创建总是失败的 Provider
func NewErrorProvider(err error) *MockProvider {
	return NewMockProvider().WithError(err)
}

// NewToolCallProvider 创建返回工具调用的 Provider
func NewToolCallProvider(toolCalls []types.ToolCall) *MockProvider {
	return NewMockProvider().WithToolCalls(toolCalls)
}

// NewStreamProvider 创建流式响应的 Provider
func NewStreamProvider(chunks []string) *MockProvider {
	return NewMockProvider().WithStreamChunks(chunks)
}

// NewFlakeyProvider 创建不稳定的 Provider（间歇性失败）
func NewFlakeyProvider(failAfter int, response string) *MockProvider {
	return NewMockProvider().
		WithResponse(response).
		WithFailAfter(failAfter)
}
