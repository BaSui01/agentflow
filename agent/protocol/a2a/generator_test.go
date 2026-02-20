package a2a

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentCardGenerator(t *testing.T) {
	gen := NewAgentCardGenerator()
	assert.NotNil(t, gen)
	assert.Equal(t, "1.0.0", gen.defaultVersion)
}

func TestNewAgentCardGeneratorWithVersion(t *testing.T) {
	gen := NewAgentCardGeneratorWithVersion("2.0.0")
	assert.NotNil(t, gen)
	assert.Equal(t, "2.0.0", gen.defaultVersion)
}

func TestAgentCardGenerator_Generate(t *testing.T) {
	tests := []struct {
		name     string
		config   *SimpleAgentConfig
		baseURL  string
		wantName string
		wantURL  string
	}{
		{
			name: "basic agent",
			config: &SimpleAgentConfig{
				AgentID:          "agent-1",
				AgentName:        "Test Agent",
				AgentType:        "assistant",
				AgentDescription: "A test assistant agent",
			},
			baseURL:  "https://api.example.com",
			wantName: "Test Agent",
			wantURL:  "https://api.example.com/agents/agent-1",
		},
		{
			name: "agent with trailing slash in baseURL",
			config: &SimpleAgentConfig{
				AgentID:          "agent-2",
				AgentName:        "Another Agent",
				AgentType:        "analyzer",
				AgentDescription: "An analyzer agent",
			},
			baseURL:  "https://api.example.com/",
			wantName: "Another Agent",
			wantURL:  "https://api.example.com/agents/agent-2",
		},
	}

	gen := NewAgentCardGenerator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := gen.Generate(tt.config, tt.baseURL)

			assert.Equal(t, tt.wantName, card.Name)
			assert.Equal(t, tt.wantURL, card.URL)
			assert.Equal(t, tt.config.AgentDescription, card.Description)
			assert.Equal(t, "1.0.0", card.Version)
			assert.NotEmpty(t, card.Capabilities)
		})
	}
}

func TestAgentCardGenerator_GenerateWithMetadata(t *testing.T) {
	gen := NewAgentCardGenerator()

	config := &SimpleAgentConfig{
		AgentID:          "agent-meta",
		AgentName:        "Meta Agent",
		AgentType:        "generic",
		AgentDescription: "Agent with metadata",
		AgentMetadata: map[string]string{
			"version": "3.0.0",
			"author":  "test",
			"env":     "production",
		},
	}

	card := gen.Generate(config, "https://api.example.com")

	// 版本应来自元数据
	assert.Equal(t, "3.0.0", card.Version)

	// 其他元数据应复制
	author, ok := card.GetMetadata("author")
	assert.True(t, ok)
	assert.Equal(t, "test", author)

	env, ok := card.GetMetadata("env")
	assert.True(t, ok)
	assert.Equal(t, "production", env)

	// 代理类型和ID应在元数据中
	agentType, ok := card.GetMetadata("agent_type")
	assert.True(t, ok)
	assert.Equal(t, "generic", agentType)

	agentID, ok := card.GetMetadata("agent_id")
	assert.True(t, ok)
	assert.Equal(t, "agent-meta", agentID)
}

func TestAgentCardGenerator_CapabilitiesByType(t *testing.T) {
	tests := []struct {
		agentType      AgentType
		wantCapability string
	}{
		{"assistant", "chat"},
		{"analyzer", "analysis"},
		{"translator", "translation"},
		{"summarizer", "summarization"},
		{"reviewer", "review"},
		{"generic", "execute"},
		{"custom", "execute"},
	}

	gen := NewAgentCardGenerator()

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			config := &SimpleAgentConfig{
				AgentID:          "test-agent",
				AgentName:        "Test",
				AgentType:        tt.agentType,
				AgentDescription: "Test agent",
			}

			card := gen.Generate(config, "https://api.example.com")
			assert.True(t, card.HasCapability(tt.wantCapability))
		})
	}
}

// 模拟工具 Provider 执行工具Schema 提供测试。
type mockToolProvider struct {
	tools map[string][]llm.ToolSchema
}

func (m *mockToolProvider) GetAllowedTools(agentID string) []llm.ToolSchema {
	if tools, ok := m.tools[agentID]; ok {
		return tools
	}
	return nil
}

func TestAgentCardGenerator_GenerateWithTools(t *testing.T) {
	gen := NewAgentCardGenerator()

	config := &SimpleAgentConfig{
		AgentID:          "tool-agent",
		AgentName:        "Tool Agent",
		AgentType:        "assistant",
		AgentDescription: "Agent with tools",
	}

	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
		},
		"required": []string{"query"},
	}
	paramsJSON, _ := json.Marshal(params)

	toolProvider := &mockToolProvider{
		tools: map[string][]llm.ToolSchema{
			"tool-agent": {
				{
					Name:        "search",
					Description: "Search for information",
					Parameters:  paramsJSON,
				},
				{
					Name:        "calculate",
					Description: "Perform calculations",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	}

	card := gen.GenerateWithTools(config, "https://api.example.com", toolProvider)

	assert.Len(t, card.Tools, 2)
	assert.True(t, card.HasTool("search"))
	assert.True(t, card.HasTool("calculate"))

	searchTool := card.GetTool("search")
	require.NotNil(t, searchTool)
	assert.Equal(t, "Search for information", searchTool.Description)
	assert.NotNil(t, searchTool.Parameters)
}

func TestAgentCardGenerator_GenerateValidCard(t *testing.T) {
	gen := NewAgentCardGenerator()

	config := &SimpleAgentConfig{
		AgentID:          "valid-agent",
		AgentName:        "Valid Agent",
		AgentType:        "assistant",
		AgentDescription: "A valid agent",
	}

	card := gen.Generate(config, "https://api.example.com")

	// 卡片应该通过验证
	err := card.Validate()
	assert.NoError(t, err)
}

func TestBuildAgentURL(t *testing.T) {
	tests := []struct {
		baseURL  string
		agentID  string
		expected string
	}{
		{"https://api.example.com", "agent-1", "https://api.example.com/agents/agent-1"},
		{"https://api.example.com/", "agent-2", "https://api.example.com/agents/agent-2"},
		{"http://localhost:8080", "test", "http://localhost:8080/agents/test"},
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			result := buildAgentURL(tt.baseURL, tt.agentID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToolSchema(t *testing.T) {
	params := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)

	schema := llm.ToolSchema{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  params,
	}

	toolDef := convertToolSchema(schema)

	assert.Equal(t, "test_tool", toolDef.Name)
	assert.Equal(t, "A test tool", toolDef.Description)
	assert.NotNil(t, toolDef.Parameters)
	assert.Equal(t, structured.TypeObject, toolDef.Parameters.Type)
}

func TestConvertToolSchema_InvalidJSON(t *testing.T) {
	schema := llm.ToolSchema{
		Name:        "bad_tool",
		Description: "Tool with invalid params",
		Parameters:  json.RawMessage(`{invalid json}`),
	}

	toolDef := convertToolSchema(schema)

	assert.Equal(t, "bad_tool", toolDef.Name)
	// 应该有一个倒计时
	assert.NotNil(t, toolDef.Parameters)
	assert.Equal(t, structured.TypeObject, toolDef.Parameters.Type)
}
