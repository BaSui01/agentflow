package shared

import "github.com/BaSui01/agentflow/agent/adapters/structured"

// CapabilityType 代表一种代理提供的能力类型。
type CapabilityType string

const (
	// CapabilityTypeTask 表示代理可以执行任务。
	CapabilityTypeTask CapabilityType = "task"
	// CapabilityTypeQuery 表示代理可以回答查询。
	CapabilityTypeQuery CapabilityType = "query"
	// CapabilityTypeStream 表示代理支持流式响应。
	CapabilityTypeStream CapabilityType = "stream"
)

// Capability 定义代理能力。
type Capability struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        CapabilityType `json:"type"`
}

// ToolDefinition 定义代理可用或暴露的工具。
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  *structured.JSONSchema `json:"parameters"`
	Version     string                 `json:"version,omitempty"`
}

// AgentCard 描述代理能力与元数据。
type AgentCard struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	URL          string                 `json:"url"`
	Version      string                 `json:"version"`
	Capabilities []Capability           `json:"capabilities"`
	InputSchema  *structured.JSONSchema `json:"input_schema,omitempty"`
	OutputSchema *structured.JSONSchema `json:"output_schema,omitempty"`
	Tools        []ToolDefinition       `json:"tools,omitempty"`
	Metadata     map[string]string      `json:"metadata,omitempty"`
}

// NewAgentCard 创建 AgentCard。
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

// AddCapability 在代理卡上添加能力。
func (c *AgentCard) AddCapability(name, description string, capType CapabilityType) *AgentCard {
	c.Capabilities = append(c.Capabilities, Capability{
		Name:        name,
		Description: description,
		Type:        capType,
	})
	return c
}

// AddTool 在代理卡上添加工具定义。
func (c *AgentCard) AddTool(name, description string, parameters *structured.JSONSchema) *AgentCard {
	c.Tools = append(c.Tools, ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	})
	return c
}

// SetInputSchema 设置输入 schema。
func (c *AgentCard) SetInputSchema(schema *structured.JSONSchema) *AgentCard {
	c.InputSchema = schema
	return c
}

// SetOutputSchema 设置输出 schema。
func (c *AgentCard) SetOutputSchema(schema *structured.JSONSchema) *AgentCard {
	c.OutputSchema = schema
	return c
}

// SetMetadata 设置 metadata 键值。
func (c *AgentCard) SetMetadata(key, value string) *AgentCard {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
	return c
}

// GetMetadata 获取 metadata。
func (c *AgentCard) GetMetadata(key string) (string, bool) {
	if c.Metadata == nil {
		return "", false
	}
	value, ok := c.Metadata[key]
	return value, ok
}

// HasCapability 检查是否存在指定能力。
func (c *AgentCard) HasCapability(name string) bool {
	for _, cap := range c.Capabilities {
		if cap.Name == name {
			return true
		}
	}
	return false
}

// GetCapability 获取指定能力。
func (c *AgentCard) GetCapability(name string) *Capability {
	for i := range c.Capabilities {
		if c.Capabilities[i].Name == name {
			return &c.Capabilities[i]
		}
	}
	return nil
}

// HasTool 检查是否存在指定工具。
func (c *AgentCard) HasTool(name string) bool {
	for _, tool := range c.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// GetTool 获取指定工具。
func (c *AgentCard) GetTool(name string) *ToolDefinition {
	for i := range c.Tools {
		if c.Tools[i].Name == name {
			return &c.Tools[i]
		}
	}
	return nil
}
