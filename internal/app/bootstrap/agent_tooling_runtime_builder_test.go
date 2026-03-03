package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/agent/hosted"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestBuildAgentToolingRuntime_WithRetrievalTool(t *testing.T) {
	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		RetrievalStore:    &testVectorStore{},
		EmbeddingProvider: &testEmbeddingProvider{},
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.ToolManager)
	assert.Contains(t, runtime.ToolNames, "retrieval")

	schemas := runtime.ToolManager.GetAllowedTools("agent-a")
	require.NotEmpty(t, schemas)
	var names []string
	for _, schema := range schemas {
		names = append(names, schema.Name)
	}
	assert.Contains(t, names, "retrieval")

	results := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{
		{
			ID:        "call-1",
			Name:      "retrieval",
			Arguments: json.RawMessage(`{"query":"hello","max_results":2}`),
		},
	})
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	assert.NotEmpty(t, results[0].Result)
}

func TestBuildAgentToolingRuntime_WithMCPTools(t *testing.T) {
	server := &testMCPServer{
		tools: []mcpproto.ToolDefinition{
			{
				Name:        "echo-tool",
				Description: "Echo args",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:      server,
		EnableMCPTools: true,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.ToolManager)
	assert.Contains(t, runtime.ToolNames, "mcp_echo_tool")

	results := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{
		{
			ID:        "call-2",
			Name:      "mcp_echo_tool",
			Arguments: json.RawMessage(`{"text":"ping"}`),
		},
	})
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	assert.JSONEq(t, `{"name":"echo-tool","args":{"text":"ping"}}`, string(results[0].Result))
}

func TestBuildAgentToolingRuntime_WithDBRegistrations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolRegistration{}))
	require.NoError(t, db.Create(&hosted.ToolRegistration{
		Name:    "knowledge_search",
		Target:  "retrieval",
		Enabled: true,
	}).Error)

	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		RetrievalStore:    &testVectorStore{},
		EmbeddingProvider: &testEmbeddingProvider{},
		DB:                db,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	assert.Contains(t, runtime.ToolNames, "retrieval")
	assert.Contains(t, runtime.ToolNames, "knowledge_search")

	results := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{
		{
			ID:        "call-3",
			Name:      "knowledge_search",
			Arguments: json.RawMessage(`{"query":"hello","max_results":2}`),
		},
	})
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	assert.NotEmpty(t, results[0].Result)
}

type testVectorStore struct{}

func (s *testVectorStore) AddDocuments(ctx context.Context, docs []rag.Document) error { return nil }
func (s *testVectorStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]rag.VectorSearchResult, error) {
	return []rag.VectorSearchResult{
		{
			Document: rag.Document{
				ID:      "doc-1",
				Content: "hello world",
			},
			Score: 0.9,
		},
	}, nil
}
func (s *testVectorStore) DeleteDocuments(ctx context.Context, ids []string) error    { return nil }
func (s *testVectorStore) UpdateDocument(ctx context.Context, doc rag.Document) error { return nil }
func (s *testVectorStore) Count(ctx context.Context) (int, error)                     { return 1, nil }

type testEmbeddingProvider struct{}

func (p *testEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return []float64{0.1, 0.2}, nil
}
func (p *testEmbeddingProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return [][]float64{{0.1, 0.2}}, nil
}
func (p *testEmbeddingProvider) Name() string { return "test-embed" }

type testMCPServer struct {
	tools []mcpproto.ToolDefinition
}

func (s *testMCPServer) GetServerInfo() mcpproto.ServerInfo { return mcpproto.ServerInfo{} }
func (s *testMCPServer) ListResources(ctx context.Context) ([]mcpproto.Resource, error) {
	return nil, nil
}
func (s *testMCPServer) GetResource(ctx context.Context, uri string) (*mcpproto.Resource, error) {
	return nil, nil
}
func (s *testMCPServer) SubscribeResource(ctx context.Context, uri string) (<-chan mcpproto.Resource, error) {
	ch := make(chan mcpproto.Resource)
	close(ch)
	return ch, nil
}
func (s *testMCPServer) ListTools(ctx context.Context) ([]mcpproto.ToolDefinition, error) {
	return s.tools, nil
}
func (s *testMCPServer) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	return map[string]any{"name": name, "args": args}, nil
}
func (s *testMCPServer) ListPrompts(ctx context.Context) ([]mcpproto.PromptTemplate, error) {
	return nil, nil
}
func (s *testMCPServer) GetPrompt(ctx context.Context, name string, vars map[string]string) (string, error) {
	return "", nil
}
func (s *testMCPServer) SetLogLevel(level string) error { return nil }
