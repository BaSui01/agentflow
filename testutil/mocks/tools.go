// =============================================================================
// 🔧 MockToolManager - 工具管理器模拟实现
// =============================================================================
// 用于测试的工具管理器模拟，支持工具注册和执行
//
// 使用方法:
//
//	tools := mocks.NewMockToolManager().
//	    WithTool("calculator", func(args map[string]any) (any, error) {
//	        return 42, nil
//	    })
// =============================================================================
package mocks

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// 🎯 MockToolManager 结构
// =============================================================================

// ToolFunc 工具执行函数类型
type ToolFunc func(ctx context.Context, args map[string]any) (any, error)

// MockToolManager 是工具管理器的模拟实现
type MockToolManager struct {
	mu sync.RWMutex

	// 工具注册表
	tools       map[string]types.ToolSchema
	toolFuncs   map[string]ToolFunc
	toolResults map[string]any
	toolErrors  map[string]error

	// 调用记录
	calls []ToolCall

	// 默认行为
	defaultResult any
	defaultError  error
}

// ToolCall 记录单次工具调用
type ToolCall struct {
	Name   string
	Args   map[string]any
	Result any
	Error  error
}

// =============================================================================
// 🔧 构造函数和 Builder 方法
// =============================================================================

// NewMockToolManager 创建新的 MockToolManager
func NewMockToolManager() *MockToolManager {
	return &MockToolManager{
		tools:       make(map[string]types.ToolSchema),
		toolFuncs:   make(map[string]ToolFunc),
		toolResults: make(map[string]any),
		toolErrors:  make(map[string]error),
		calls:       []ToolCall{},
	}
}

// WithTool 注册工具及其执行函数
func (m *MockToolManager) WithTool(name string, fn ToolFunc) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()

	params, _ := json.Marshal(map[string]any{"type": "object"})
	m.tools[name] = types.ToolSchema{
		Name:        name,
		Description: "Mock tool: " + name,
		Parameters:  params,
	}
	m.toolFuncs[name] = fn
	return m
}

// WithToolDefinition 注册工具定义
func (m *MockToolManager) WithToolDefinition(tool types.ToolSchema) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[tool.Name] = tool
	return m
}

// WithToolResult 设置工具的固定返回结果
func (m *MockToolManager) WithToolResult(name string, result any) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolResults[name] = result
	// 确保工具存在
	if _, ok := m.tools[name]; !ok {
		params, _ := json.Marshal(map[string]any{"type": "object"})
		m.tools[name] = types.ToolSchema{
			Name:        name,
			Description: "Mock tool: " + name,
			Parameters:  params,
		}
	}
	return m
}

// WithToolError 设置工具的固定返回错误
func (m *MockToolManager) WithToolError(name string, err error) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolErrors[name] = err
	return m
}

// WithDefaultResult 设置默认返回结果
func (m *MockToolManager) WithDefaultResult(result any) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultResult = result
	return m
}

// WithDefaultError 设置默认返回错误
func (m *MockToolManager) WithDefaultError(err error) *MockToolManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultError = err
	return m
}

// =============================================================================
// 🎯 ToolManager 接口实现
// =============================================================================

// Register 注册工具
func (m *MockToolManager) Register(tool types.ToolSchema) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[tool.Name] = tool
	return nil
}

// Unregister 注销工具
func (m *MockToolManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tools, name)
	delete(m.toolFuncs, name)
	delete(m.toolResults, name)
	delete(m.toolErrors, name)
	return nil
}

// Get 获取工具定义
func (m *MockToolManager) Get(name string) (types.ToolSchema, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tool, ok := m.tools[name]
	return tool, ok
}

// List 列出所有工具
func (m *MockToolManager) List() []types.ToolSchema {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]types.ToolSchema, 0, len(m.tools))
	for _, tool := range m.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Execute 执行工具
func (m *MockToolManager) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	call := ToolCall{
		Name: name,
		Args: args,
	}

	// 检查工具是否存在
	if _, ok := m.tools[name]; !ok {
		err := errors.New("tool not found: " + name)
		call.Error = err
		m.calls = append(m.calls, call)
		return nil, err
	}

	// 检查是否有预设错误
	if err, ok := m.toolErrors[name]; ok {
		call.Error = err
		m.calls = append(m.calls, call)
		return nil, err
	}

	// 检查是否有预设结果
	if result, ok := m.toolResults[name]; ok {
		call.Result = result
		m.calls = append(m.calls, call)
		return result, nil
	}

	// 检查是否有执行函数
	if fn, ok := m.toolFuncs[name]; ok {
		result, err := fn(ctx, args)
		call.Result = result
		call.Error = err
		m.calls = append(m.calls, call)
		return result, err
	}

	// 使用默认行为
	if m.defaultError != nil {
		call.Error = m.defaultError
		m.calls = append(m.calls, call)
		return nil, m.defaultError
	}

	call.Result = m.defaultResult
	m.calls = append(m.calls, call)
	return m.defaultResult, nil
}

// ExecuteToolCall 执行 ToolCall 结构
func (m *MockToolManager) ExecuteToolCall(ctx context.Context, tc types.ToolCall) (any, error) {
	var args map[string]any
	if len(tc.Arguments) > 0 {
		json.Unmarshal(tc.Arguments, &args)
	}
	return m.Execute(ctx, tc.Name, args)
}

// =============================================================================
// 🔍 查询方法
// =============================================================================

// GetCalls 获取所有调用记录
func (m *MockToolManager) GetCalls() []ToolCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]ToolCall{}, m.calls...)
}

// GetCallCount 获取调用次数
func (m *MockToolManager) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.calls)
}

// GetCallsForTool 获取特定工具的调用记录
func (m *MockToolManager) GetCallsForTool(name string) []ToolCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var calls []ToolCall
	for _, call := range m.calls {
		if call.Name == name {
			calls = append(calls, call)
		}
	}
	return calls
}

// GetLastCall 获取最后一次调用
func (m *MockToolManager) GetLastCall() *ToolCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.calls) == 0 {
		return nil
	}
	call := m.calls[len(m.calls)-1]
	return &call
}

// HasTool 检查工具是否存在
func (m *MockToolManager) HasTool(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.tools[name]
	return ok
}

// Reset 重置所有状态
func (m *MockToolManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = []ToolCall{}
}

// Clear 清空所有工具和状态
func (m *MockToolManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools = make(map[string]types.ToolSchema)
	m.toolFuncs = make(map[string]ToolFunc)
	m.toolResults = make(map[string]any)
	m.toolErrors = make(map[string]error)
	m.calls = []ToolCall{}
}

// =============================================================================
// 🎭 预设 ToolManager 工厂
// =============================================================================

// NewEmptyToolManager 创建空的工具管理器
func NewEmptyToolManager() *MockToolManager {
	return NewMockToolManager()
}

// NewCalculatorToolManager 创建带计算器工具的管理器
func NewCalculatorToolManager() *MockToolManager {
	return NewMockToolManager().
		WithTool("calculator", func(ctx context.Context, args map[string]any) (any, error) {
			// 简单的加法计算器
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			op, _ := args["op"].(string)

			switch op {
			case "add", "+":
				return a + b, nil
			case "sub", "-":
				return a - b, nil
			case "mul", "*":
				return a * b, nil
			case "div", "/":
				if b == 0 {
					return nil, errors.New("division by zero")
				}
				return a / b, nil
			default:
				return a + b, nil
			}
		})
}

// NewSearchToolManager 创建带搜索工具的管理器
func NewSearchToolManager(results []string) *MockToolManager {
	return NewMockToolManager().
		WithTool("search", func(ctx context.Context, args map[string]any) (any, error) {
			return results, nil
		})
}

// NewErrorToolManager 创建总是返回错误的工具管理器
func NewErrorToolManager(err error) *MockToolManager {
	return NewMockToolManager().WithDefaultError(err)
}
