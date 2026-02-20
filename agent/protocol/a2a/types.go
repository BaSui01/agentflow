package a2a

import (
	"github.com/BaSui01/agentflow/agent/structured"
)

// 能力 类型代表一种代理人提供的能力类型。
type CapabilityType string

const (
	// 能力TypeTask表示代理可以执行任务.
	CapabilityTypeTask CapabilityType = "task"
	// 能力 TypeQuery表示代理可以回答询问.
	CapabilityTypeQuery CapabilityType = "query"
	// 能力 TypeStream 表示代理支持流响应.
	CapabilityTypeStream CapabilityType = "stream"
)

// 能力在A2A协议中定义了代理的能力.
type Capability struct {
	// 名称是此能力的唯一标识符 。
	Name string `json:"name"`
	// 描述为人们提供了这种能力的可读性描述.
	Description string `json:"description"`
	// 类型表示能力类型(任务,查询,流).
	Type CapabilityType `json:"type"`
}

// Tool Definition定义了代理人可以使用或曝光的工具.
type ToolDefinition struct {
	// 名称是此工具的唯一标识符 。
	Name string `json:"name"`
	// 描述提供了一种人类可以读取的描述,说明这个工具是做什么的.
	Description string `json:"description"`
	// 参数定义了该工具的输入参数的JSON Schema.
	Parameters *structured.JSONSchema `json:"parameters"`
}

// AgentCard代表一个描述代理能力和元数据的A2A代理卡.
// 它遵循了Google A2A关于特工发现和互操作性的规格.
type AgentCard struct {
	// 名称是此代理的唯一标识符 。
	Name string `json:"name"`
	// 描述提供了一种人能读取的关于该剂目的的描述.
	Description string `json:"description"`
	// URL是能够到达此代理的终点 。
	URL string `json:"url"`
	// 版本表示代理的版本.
	Version string `json:"version"`
	// 能力列出这个代理提供的能力.
	Capabilities []Capability `json:"capabilities"`
	// 输入Schema定义了代理商的预期输入格式的JSON Schema.
	InputSchema *structured.JSONSchema `json:"input_schema,omitempty"`
	// OutsionSchema定义了代理输出格式的JSON Schema.
	OutputSchema *structured.JSONSchema `json:"output_schema,omitempty"`
	// 工具列出这个代理可以使用或曝光的工具.
	Tools []ToolDefinition `json:"tools,omitempty"`
	// 元数据包含额外的可扩展性的密钥值对.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewAgentCard创建了具有所需字段的新AgentCard.
func NewAgentCard(name, description, url, version string) *AgentCard {
	return &AgentCard{
		Name:         name,
		Description:  description,
		URL:          url,
		Version:      version,
		Capabilities: make([]Capability, 0),
		Tools:        make([]ToolDefinition, 0),
		Metadata:     make(map[string]string),
	}
}

// 添加能力在代理卡上增加了一个能力.
func (c *AgentCard) AddCapability(name, description string, capType CapabilityType) *AgentCard {
	c.Capabilities = append(c.Capabilities, Capability{
		Name:        name,
		Description: description,
		Type:        capType,
	})
	return c
}

// AddTool在代理卡上添加了工具定义.
func (c *AgentCard) AddTool(name, description string, parameters *structured.JSONSchema) *AgentCard {
	c.Tools = append(c.Tools, ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	})
	return c
}

// SetInputSchema为代理卡设置输入方案.
func (c *AgentCard) SetInputSchema(schema *structured.JSONSchema) *AgentCard {
	c.InputSchema = schema
	return c
}

// SetOutputSchema为代理卡设置输出计划.
func (c *AgentCard) SetOutputSchema(schema *structured.JSONSchema) *AgentCard {
	c.OutputSchema = schema
	return c
}

// SetMetadata 设置了元数据密钥值对.
func (c *AgentCard) SetMetadata(key, value string) *AgentCard {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
	return c
}

// GetMetadata 按键检索元数据值 。
func (c *AgentCard) GetMetadata(key string) (string, bool) {
	if c.Metadata == nil {
		return "", false
	}
	value, ok := c.Metadata[key]
	return value, ok
}

// 如果代理人有特定的能力,则进行能力检查。
func (c *AgentCard) HasCapability(name string) bool {
	for _, cap := range c.Capabilities {
		if cap.Name == name {
			return true
		}
	}
	return false
}

// Get Capability 按名称检索一个能力.
func (c *AgentCard) GetCapability(name string) *Capability {
	for i := range c.Capabilities {
		if c.Capabilities[i].Name == name {
			return &c.Capabilities[i]
		}
	}
	return nil
}

// HasTool检查代理人是否有特定的名称工具.
func (c *AgentCard) HasTool(name string) bool {
	for _, tool := range c.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// GetTool 检索一个工具名称定义 。
func (c *AgentCard) GetTool(name string) *ToolDefinition {
	for i := range c.Tools {
		if c.Tools[i].Name == name {
			return &c.Tools[i]
		}
	}
	return nil
}

// 验证AgentCard是否拥有所有需要的字段 。
func (c *AgentCard) Validate() error {
	if c.Name == "" {
		return ErrMissingName
	}
	if c.Description == "" {
		return ErrMissingDescription
	}
	if c.URL == "" {
		return ErrMissingURL
	}
	if c.Version == "" {
		return ErrMissingVersion
	}
	return nil
}
