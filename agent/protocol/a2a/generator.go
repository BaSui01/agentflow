// a2a包提供A2A(代理对代理)协议支持.
package a2a

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
)

// Agent Config Provider 定义访问代理配置的接口.
// 这使得生成器能够配合任何代理执行.
type AgentConfigProvider interface {
	ID() string
	Name() string
	Type() AgentType
	Description() string
	Tools() []string
	Metadata() map[string]string
}

// AgentType代表着特工的类型.
type AgentType string

// ToolSchema Provider为一代理提供了工具计划.
type ToolSchemaProvider interface {
	GetAllowedTools(agentID string) []llm.ToolSchema
}

// AgentCardGenerator通过代理配置生成代理卡.
type AgentCardGenerator struct {
	// 当代理商不指定一个版本时使用默认Version.
	defaultVersion string
}

// 新代理CardGenerator创建了新的代理CardGenerator.
func NewAgentCardGenerator() *AgentCardGenerator {
	return &AgentCardGenerator{
		defaultVersion: "1.0.0",
	}
}

// NewAgentCardGenerator With Version 创建了自定义默认版本的新AgentCardGenerator.
func NewAgentCardGeneratorWithVersion(version string) *AgentCardGenerator {
	return &AgentCardGenerator{
		defaultVersion: version,
	}
}

// 从代理配置和基址生成 AgentCard 。
// 碱基URL应该是能到达剂的终点.
func (g *AgentCardGenerator) Generate(config AgentConfigProvider, baseURL string) *AgentCard {
	return g.GenerateWithTools(config, baseURL, nil)
}

// 生成 With Tools 创建了 AgentCard 从 ToolSchema Provider 生成工具定义.
func (g *AgentCardGenerator) GenerateWithTools(config AgentConfigProvider, baseURL string, toolProvider ToolSchemaProvider) *AgentCard {
	// 构建代理 URL
	agentURL := buildAgentURL(baseURL, config.ID())

	// 从元数据或默认使用中确定版本
	version := g.defaultVersion
	if meta := config.Metadata(); meta != nil {
		if v, ok := meta["version"]; ok && v != "" {
			version = v
		}
	}

	// 创建代理卡
	card := NewAgentCard(
		config.Name(),
		config.Description(),
		agentURL,
		version,
	)

	// 根据代理类型添加默认能力
	g.addCapabilitiesFromType(card, config.Type())

	// 可用提供者时添加工具
	if toolProvider != nil {
		g.addToolsFromProvider(card, config.ID(), toolProvider)
	}

	// 复制元数据
	if meta := config.Metadata(); meta != nil {
		for k, v := range meta {
			if k != "version" { // version is already used
				card.SetMetadata(k, v)
			}
		}
	}

	// 添加代理类型到元数据
	card.SetMetadata("agent_type", string(config.Type()))
	card.SetMetadata("agent_id", config.ID())

	return card
}

// 添加基于代理类型的默认能力。
func (g *AgentCardGenerator) addCapabilitiesFromType(card *AgentCard, agentType AgentType) {
	typeStr := string(agentType)

	switch typeStr {
	case "assistant":
		card.AddCapability("chat", "Interactive conversation and assistance", CapabilityTypeQuery)
		card.AddCapability("task_execution", "Execute tasks based on user requests", CapabilityTypeTask)
	case "analyzer":
		card.AddCapability("analysis", "Analyze data and provide insights", CapabilityTypeTask)
	case "translator":
		card.AddCapability("translation", "Translate text between languages", CapabilityTypeTask)
	case "summarizer":
		card.AddCapability("summarization", "Summarize text content", CapabilityTypeTask)
	case "reviewer":
		card.AddCapability("review", "Review and provide feedback on content", CapabilityTypeTask)
	default:
		// 通用剂具有基本的任务能力
		card.AddCapability("execute", "Execute general tasks", CapabilityTypeTask)
	}
}

// 添加 ToolsFromProvider 从工具SchemaProvider中添加工具定义.
func (g *AgentCardGenerator) addToolsFromProvider(card *AgentCard, agentID string, provider ToolSchemaProvider) {
	schemas := provider.GetAllowedTools(agentID)
	for _, schema := range schemas {
		toolDef := convertToolSchema(schema)
		card.Tools = append(card.Tools, toolDef)
	}
}

// 转换 ToolSchema 转换一个 llm。 ToolSchema 到工具定义。
func convertToolSchema(schema llm.ToolSchema) ToolDefinition {
	var params *structured.JSONSchema

	// 将参数 JSON 分析为 JSONSchema
	if len(schema.Parameters) > 0 {
		params = &structured.JSONSchema{}
		if err := json.Unmarshal(schema.Parameters, params); err != nil {
			// 如果解析失败, 请创建基本对象计划
			params = &structured.JSONSchema{
				Type:        structured.TypeObject,
				Description: "Tool parameters",
			}
		}
	}

	return ToolDefinition{
		Name:        schema.Name,
		Description: schema.Description,
		Parameters:  params,
	}
}

// 构建 AgentURL 从基址和代理 ID 构建完整的代理 URL 。
func buildAgentURL(baseURL, agentID string) string {
	// 确保碱基URL不以斜线结束
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建代理端点 URL
	return fmt.Sprintf("%s/agents/%s", baseURL, agentID)
}

// SimpleAgentConfig是AgentConfig Provider的简单执行,用于测试和基本使用.
type SimpleAgentConfig struct {
	AgentID          string
	AgentName        string
	AgentType        AgentType
	AgentDescription string
	AgentTools       []string
	AgentMetadata    map[string]string
}

// ID返回代理ID.
func (c *SimpleAgentConfig) ID() string { return c.AgentID }

// 名称返回代理名称 。
func (c *SimpleAgentConfig) Name() string { return c.AgentName }

// 类型返回代理类型。
func (c *SimpleAgentConfig) Type() AgentType { return c.AgentType }

// 描述返回代理描述 。
func (c *SimpleAgentConfig) Description() string { return c.AgentDescription }

// 工具返回工具名称列表 。
func (c *SimpleAgentConfig) Tools() []string { return c.AgentTools }

// 元数据返回代理元数据 。
func (c *SimpleAgentConfig) Metadata() map[string]string { return c.AgentMetadata }
