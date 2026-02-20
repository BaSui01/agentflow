package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// MCP (Model Context Protocol) 标准接口
// 基于 Anthropic MCP 规范实现

// MCPVersion MCP 协议版本
const MCPVersion = "2024-11-05"

// ResourceType 资源类型
type ResourceType string

const (
	ResourceTypeText   ResourceType = "text"
	ResourceTypeImage  ResourceType = "image"
	ResourceTypeFile   ResourceType = "file"
	ResourceTypeData   ResourceType = "data"
	ResourceTypeStream ResourceType = "stream"
)

// Resource MCP 资源
type Resource struct {
	URI         string                 `json:"uri"`         // 资源 URI
	Name        string                 `json:"name"`        // 资源名称
	Description string                 `json:"description"` // 资源描述
	Type        ResourceType           `json:"type"`        // 资源类型
	MimeType    string                 `json:"mimeType"`    // MIME 类型
	Content     any            `json:"content"`     // 资源内容
	Metadata    map[string]any `json:"metadata"`    // 元数据
	Size        int64                  `json:"size"`        // 资源大小（字节）
	CreatedAt   time.Time              `json:"createdAt"`   // 创建时间
	UpdatedAt   time.Time              `json:"updatedAt"`   // 更新时间
}

// ToolDefinition MCP 工具定义
type ToolDefinition struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"` // JSON Schema
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// PromptTemplate MCP 提示词模板
type PromptTemplate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Template    string                 `json:"template"`
	Variables   []string               `json:"variables"`
	Examples    []PromptExample        `json:"examples,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PromptExample 提示词示例
type PromptExample struct {
	Variables map[string]string `json:"variables"`
	Output    string            `json:"output"`
}

// MCPServer MCP 服务器接口
type MCPServer interface {
	// 服务器信息
	GetServerInfo() ServerInfo

	// 资源管理
	ListResources(ctx context.Context) ([]Resource, error)
	GetResource(ctx context.Context, uri string) (*Resource, error)
	SubscribeResource(ctx context.Context, uri string) (<-chan Resource, error)

	// 工具管理
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)

	// 提示词管理
	ListPrompts(ctx context.Context) ([]PromptTemplate, error)
	GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error)

	// 日志
	SetLogLevel(level string) error
}

// ServerInfo 服务器信息
type ServerInfo struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    ServerCapabilities     `json:"capabilities"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// ServerCapabilities 服务器能力
type ServerCapabilities struct {
	Resources bool `json:"resources"`
	Tools     bool `json:"tools"`
	Prompts   bool `json:"prompts"`
	Logging   bool `json:"logging"`
	Sampling  bool `json:"sampling"`
}

// MCPClient MCP 客户端接口
type MCPClient interface {
	// 连接管理
	Connect(ctx context.Context, serverURL string) error
	Disconnect(ctx context.Context) error
	IsConnected() bool

	// 服务器交互
	GetServerInfo(ctx context.Context) (*ServerInfo, error)

	// 资源操作
	ListResources(ctx context.Context) ([]Resource, error)
	ReadResource(ctx context.Context, uri string) (*Resource, error)

	// 工具操作
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)

	// 提示词操作
	ListPrompts(ctx context.Context) ([]PromptTemplate, error)
	GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error)
}

// MCPMessage MCP 消息（JSON-RPC 2.0）
type MCPMessage struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string                 `json:"method,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *MCPError              `json:"error,omitempty"`
}

// MCPError MCP 错误
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    any `json:"data,omitempty"`
}

// 标准错误码
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// ToLLMToolSchema 将 MCP 工具定义转换为 LLM 工具 Schema
func (t *ToolDefinition) ToLLMToolSchema() llm.ToolSchema {
	// 将 map[string]any 转换为 json.RawMessage
	parametersJSON, _ := json.Marshal(t.InputSchema)

	return llm.ToolSchema{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  parametersJSON,
	}
}

// FromLLMToolSchema 从 LLM 工具 Schema 创建 MCP 工具定义
func FromLLMToolSchema(schema llm.ToolSchema) ToolDefinition {
	// 将 json.RawMessage 转换为 map[string]any
	var inputSchema map[string]any
	_ = json.Unmarshal(schema.Parameters, &inputSchema)

	return ToolDefinition{
		Name:        schema.Name,
		Description: schema.Description,
		InputSchema: inputSchema,
	}
}

// ValidateResource 验证资源
func (r *Resource) Validate() error {
	if r.URI == "" {
		return fmt.Errorf("resource URI is required")
	}
	if r.Name == "" {
		return fmt.Errorf("resource name is required")
	}
	if r.Type == "" {
		return fmt.Errorf("resource type is required")
	}
	return nil
}

// ValidateToolDefinition 验证工具定义
func (t *ToolDefinition) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Description == "" {
		return fmt.Errorf("tool description is required")
	}
	if t.InputSchema == nil {
		return fmt.Errorf("tool input schema is required")
	}
	return nil
}

// ValidatePromptTemplate 验证提示词模板
func (p *PromptTemplate) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("prompt name is required")
	}
	if p.Template == "" {
		return fmt.Errorf("prompt template is required")
	}
	return nil
}

// RenderPrompt 渲染提示词模板
func (p *PromptTemplate) RenderPrompt(vars map[string]string) (string, error) {
	result := p.Template

	for _, varName := range p.Variables {
		value, ok := vars[varName]
		if !ok {
			return "", fmt.Errorf("variable %s not provided", varName)
		}

		placeholder := "{{" + varName + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// MarshalJSON 自定义 JSON 序列化
func (m *MCPMessage) MarshalJSON() ([]byte, error) {
	type Alias MCPMessage
	return json.Marshal(&struct {
		JSONRPC string `json:"jsonrpc"`
		*Alias
	}{
		JSONRPC: "2.0",
		Alias:   (*Alias)(m),
	})
}

// NewMCPRequest 创建 MCP 请求
func NewMCPRequest(id any, method string, params map[string]any) *MCPMessage {
	return &MCPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// NewMCPResponse 创建 MCP 响应
func NewMCPResponse(id any, result any) *MCPMessage {
	return &MCPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// NewMCPError 创建 MCP 错误响应
func NewMCPError(id any, code int, message string, data any) *MCPMessage {
	return &MCPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}
