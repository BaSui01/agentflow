package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// 特征:代理-框架-2026-增强,财产9:代理卡完整性
// 审定:要求5.1、5.2、5.3
// 对于任何注册的代理,生成的代理卡应包含非空名,说明,
// URL,版本字段,以及能力列表应反映代理的实际能力.

// propMock Agent 仪器代理. 属性测试代理界面 。
type propMockAgent struct {
	id          string
	name        string
	agentType   agent.AgentType
	description string
	tools       []string
	metadata    map[string]string
}

func (m *propMockAgent) ID() string                         { return m.id }
func (m *propMockAgent) Name() string                       { return m.name }
func (m *propMockAgent) Type() agent.AgentType              { return m.agentType }
func (m *propMockAgent) State() agent.State                 { return agent.StateReady }
func (m *propMockAgent) Init(ctx context.Context) error     { return nil }
func (m *propMockAgent) Teardown(ctx context.Context) error { return nil }
func (m *propMockAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
	return nil, nil
}
func (m *propMockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: "test"}, nil
}
func (m *propMockAgent) Observe(ctx context.Context, feedback *agent.Feedback) error { return nil }

// 描述返回代理描述 。
func (m *propMockAgent) Description() string { return m.description }

// 工具返回代理工具。
func (m *propMockAgent) Tools() []string { return m.tools }

// 元数据返回代理元数据 。
func (m *propMockAgent) Metadata() map[string]string { return m.metadata }

// propTool Provider执行工具Schema Provider用于属性测试.
type propToolProvider struct {
	tools map[string][]llm.ToolSchema
}

func (p *propToolProvider) GetAllowedTools(agentID string) []llm.ToolSchema {
	if tools, ok := p.tools[agentID]; ok {
		return tools
	}
	return nil
}

// 生成 AgentCard 的 Property  AgentCard  完成性测试 拥有所有所需的字段 。
func TestProperty_AgentCard_Completeness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机代理配置
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		agentName := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`).Draw(rt, "agentName")
		agentDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ,.]{10,100}`).Draw(rt, "agentDesc")
		agentType := rapid.SampledFrom([]agent.AgentType{
			agent.TypeAssistant,
			agent.TypeAnalyzer,
			agent.TypeTranslator,
			agent.TypeSummarizer,
			agent.TypeReviewer,
		}).Draw(rt, "agentType")
		baseURL := rapid.SampledFrom([]string{
			"https://api.example.com",
			"http://localhost:8080",
			"https://agents.mycompany.io",
		}).Draw(rt, "baseURL")

		// 创建模拟代理
		ag := &propMockAgent{
			id:          agentID,
			name:        agentName,
			agentType:   agentType,
			description: agentDesc,
		}

		// 生成代理卡
		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), baseURL)

		// 属性: 名称必须是非空的
		assert.NotEmpty(t, card.Name, "AgentCard.Name should not be empty")

		// 属性: 描述必须是非空的
		assert.NotEmpty(t, card.Description, "AgentCard.Description should not be empty")

		// 属性: URL 必须是非空的
		assert.NotEmpty(t, card.URL, "AgentCard.URL should not be empty")

		// 属性: 版本必须是非空的
		assert.NotEmpty(t, card.Version, "AgentCard.Version should not be empty")

		// 属性: 卡片应该通过验证
		err := card.Validate()
		assert.NoError(t, err, "AgentCard should pass validation")
	})
}

// 测试 Property AgentCard Capabilitys ReflectAgentType 测试能力反映代理类型.
func TestProperty_AgentCard_CapabilitiesReflectAgentType(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 将代理类型映射到预期能力
		typeCapabilities := map[agent.AgentType]string{
			agent.TypeAssistant:  "chat",
			agent.TypeAnalyzer:   "analysis",
			agent.TypeTranslator: "translation",
			agent.TypeSummarizer: "summarization",
			agent.TypeReviewer:   "review",
		}

		agentType := rapid.SampledFrom([]agent.AgentType{
			agent.TypeAssistant,
			agent.TypeAnalyzer,
			agent.TypeTranslator,
			agent.TypeSummarizer,
			agent.TypeReviewer,
		}).Draw(rt, "agentType")

		ag := &propMockAgent{
			id:          "test-agent",
			name:        "Test Agent",
			agentType:   agentType,
			description: "A test agent for property testing",
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// 属性: 能力列表不应为空
		assert.NotEmpty(t, card.Capabilities, "AgentCard.Capabilities should not be empty")

		// 属性: 能力应反映代理类型
		expectedCap := typeCapabilities[agentType]
		assert.True(t, card.HasCapability(expectedCap),
			"AgentCard should have capability '%s' for agent type '%s'", expectedCap, agentType)
	})
}

// 测试Property AgentCard Tools ReflectAgentTools 测试工具列表反映了代理的实际工具.
func TestProperty_AgentCard_ToolsReflectAgentTools(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")

		// 生成工具的随机数量
		numTools := rapid.IntRange(0, 5).Draw(rt, "numTools")
		toolSchemas := make([]llm.ToolSchema, numTools)
		toolNames := make([]string, numTools)

		for i := 0; i < numTools; i++ {
			toolName := rapid.StringMatching(`[a-z][a-z_]{2,15}`).Draw(rt, fmt.Sprintf("toolName_%d", i))
			toolDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ]{5,50}`).Draw(rt, fmt.Sprintf("toolDesc_%d", i))

			toolNames[i] = toolName
			toolSchemas[i] = llm.ToolSchema{
				Name:        toolName,
				Description: toolDesc,
				Parameters:  json.RawMessage(`{"type":"object"}`),
			}
		}

		ag := &propMockAgent{
			id:          agentID,
			name:        "Tool Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with tools",
			tools:       toolNames,
		}

		toolProvider := &propToolProvider{
			tools: map[string][]llm.ToolSchema{
				agentID: toolSchemas,
			},
		}

		gen := NewAgentCardGenerator()
		card := gen.GenerateWithTools(newAgentAdapter(ag), "https://api.example.com", toolProvider)

		// 属性: 工具计数应匹配
		assert.Len(t, card.Tools, numTools, "AgentCard.Tools count should match agent tools")

		// 属性: 所有工具名称都应存在
		for _, toolName := range toolNames {
			assert.True(t, card.HasTool(toolName),
				"AgentCard should have tool '%s'", toolName)
		}
	})
}

// 测试Property AgentCard Metadata 预留测试元数据保存在代理卡.
func TestProperty_AgentCard_MetadataPreserved(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成随机元数据
		numMeta := rapid.IntRange(1, 5).Draw(rt, "numMeta")
		metadata := make(map[string]string)

		for i := 0; i < numMeta; i++ {
			key := rapid.StringMatching(`[a-z][a-z_]{2,10}`).Draw(rt, fmt.Sprintf("metaKey_%d", i))
			value := rapid.StringMatching(`[a-zA-Z0-9]{3,20}`).Draw(rt, fmt.Sprintf("metaValue_%d", i))
			// 跳过特殊处理的“ 版本” 密钥
			if key != "version" {
				metadata[key] = value
			}
		}

		ag := &propMockAgent{
			id:          "meta-agent",
			name:        "Meta Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with metadata",
			metadata:    metadata,
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// 属性: 所有元数据应当保存(版本除外)
		for key, value := range metadata {
			if key != "version" {
				cardValue, ok := card.GetMetadata(key)
				assert.True(t, ok, "Metadata key '%s' should be present", key)
				assert.Equal(t, value, cardValue, "Metadata value for '%s' should match", key)
			}
		}

		// 属性:代理 类型和代理 id应在元数据中
		agentType, ok := card.GetMetadata("agent_type")
		assert.True(t, ok, "agent_type should be in metadata")
		assert.Equal(t, string(ag.agentType), agentType)

		agentIDMeta, ok := card.GetMetadata("agent_id")
		assert.True(t, ok, "agent_id should be in metadata")
		assert.Equal(t, ag.id, agentIDMeta)
	})
}

// 测试Property AgentCard Version FromMetadata测试,如果存在,该版本取自元数据.
func TestProperty_AgentCard_VersionFromMetadata(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		version := rapid.StringMatching(`[0-9]+\.[0-9]+\.[0-9]+`).Draw(rt, "version")

		ag := &propMockAgent{
			id:          "versioned-agent",
			name:        "Versioned Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent with version",
			metadata: map[string]string{
				"version": version,
			},
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), "https://api.example.com")

		// 属性: 版本应来自元数据
		assert.Equal(t, version, card.Version, "Version should be taken from metadata")
	})
}

// TestProperty AgentCard URLFormat 测试 URL 正确格式化.
func TestProperty_AgentCard_URLFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		baseURL := rapid.SampledFrom([]string{
			"https://api.example.com",
			"https://api.example.com/",
			"http://localhost:8080",
			"http://localhost:8080/",
		}).Draw(rt, "baseURL")

		ag := &propMockAgent{
			id:          agentID,
			name:        "URL Agent",
			agentType:   agent.TypeAssistant,
			description: "Agent for URL testing",
		}

		gen := NewAgentCardGenerator()
		card := gen.Generate(newAgentAdapter(ag), baseURL)

		// 属性: URL 应包含代理ID
		assert.Contains(t, card.URL, agentID, "URL should contain agent ID")

		// 属性: URL 不应有双斜线( 协议除外)
		urlWithoutProtocol := card.URL[8:] // Skip "https://" or "http://"
		assert.NotContains(t, urlWithoutProtocol, "//", "URL should not have double slashes")
	})
}

// 测试Property AgentCard Registered AgentHasValidCard测试 注册代理持有有效卡.
func TestProperty_AgentCard_RegisteredAgentHasValidCard(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentID := rapid.StringMatching(`[a-z][a-z0-9-]{2,20}`).Draw(rt, "agentID")
		agentName := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{2,30}`).Draw(rt, "agentName")
		agentDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ,.]{10,100}`).Draw(rt, "agentDesc")

		ag := &propMockAgent{
			id:          agentID,
			name:        agentName,
			agentType:   agent.TypeAssistant,
			description: agentDesc,
		}

		// 创建服务器和注册代理
		server := NewHTTPServer(&ServerConfig{
			BaseURL: "https://api.example.com",
		})

		err := server.RegisterAgent(ag)
		require.NoError(t, err, "Should register agent successfully")

		// 获取代理卡
		card, err := server.GetAgentCard(agentID)
		require.NoError(t, err, "Should get agent card")

		// 属性: 卡片应包含所有需要的字段
		assert.NotEmpty(t, card.Name, "Name should not be empty")
		assert.NotEmpty(t, card.Description, "Description should not be empty")
		assert.NotEmpty(t, card.URL, "URL should not be empty")
		assert.NotEmpty(t, card.Version, "Version should not be empty")

		// 属性: 卡片应该通过验证
		err = card.Validate()
		assert.NoError(t, err, "Card should pass validation")

		// 属性: 能力不应为空
		assert.NotEmpty(t, card.Capabilities, "Capabilities should not be empty")
	})
}

// 测试Property AgentCard  ToolDefinition Information 测试工具定义完成.
func TestProperty_AgentCard_ToolDefinitionCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		toolName := rapid.StringMatching(`[a-z][a-z_]{2,15}`).Draw(rt, "toolName")
		toolDesc := rapid.StringMatching(`[A-Za-z][a-zA-Z0-9 ]{5,50}`).Draw(rt, "toolDesc")

		// 创建带有参数的工具计划
		params := &structured.JSONSchema{
			Type: structured.TypeObject,
			Properties: map[string]*structured.JSONSchema{
				"input": {
					Type:        structured.TypeString,
					Description: "Input parameter",
				},
			},
			Required: []string{"input"},
		}
		paramsJSON, _ := json.Marshal(params)

		toolSchema := llm.ToolSchema{
			Name:        toolName,
			Description: toolDesc,
			Parameters:  paramsJSON,
		}

		toolDef := convertToolSchema(toolSchema)

		// 属性: 工具定义应该有名称
		assert.Equal(t, toolName, toolDef.Name, "Tool name should match")

		// 属性: 工具定义应当有描述
		assert.Equal(t, toolDesc, toolDef.Description, "Tool description should match")

		// 属性: 工具定义应当有参数
		assert.NotNil(t, toolDef.Parameters, "Tool parameters should not be nil")
	})
}
