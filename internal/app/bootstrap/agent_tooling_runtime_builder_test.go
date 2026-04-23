package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
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
	require.NotNil(t, runtime.Permissions)
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
	require.NotNil(t, runtime.Permissions)
	assert.Contains(t, runtime.ToolNames, "mcp_echo_tool")

	results := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{
		{
			ID:        "call-2",
			Name:      "mcp_echo_tool",
			Arguments: json.RawMessage(`{"text":"ping"}`),
		},
	})
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "approval required")
	assert.Empty(t, results[0].Result)
}

func TestBuildAgentToolingRuntime_WithDBRegistrations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolRegistration{}))
	require.NoError(t, db.AutoMigrate(&hosted.ToolProviderConfig{}))
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
	require.NotNil(t, runtime.Permissions)
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

func TestBuildAgentToolingRuntime_FiltersAllowedToolsByAgentPermission(t *testing.T) {
	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		RetrievalStore:    &testVectorStore{},
		EmbeddingProvider: &testEmbeddingProvider{},
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Permissions)

	err = runtime.Permissions.SetAgentPermission(&llmtools.AgentPermission{
		AgentID:      "agent-a",
		AllowedTools: []string{"retrieval"},
	})
	require.NoError(t, err)

	schemas := runtime.ToolManager.GetAllowedTools("agent-a")
	require.Len(t, schemas, 1)
	assert.Equal(t, "retrieval", schemas[0].Name)
}

func TestDefaultToolPermissionManager_RiskTierRules(t *testing.T) {
	pm := newDefaultToolPermissionManager(zap.NewNop())

	allowed, err := pm.CheckPermission(context.Background(), &llmtools.PermissionContext{
		ToolName:  "retrieval",
		RequestAt: time.Now(),
		Metadata: map[string]string{
			"hosted_tool_risk": "safe_read",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, llmtools.PermissionAllow, allowed.Decision)

	needsApproval, err := pm.CheckPermission(context.Background(), &llmtools.PermissionContext{
		ToolName:  "run_command",
		RequestAt: time.Now(),
		Metadata: map[string]string{
			"hosted_tool_risk": "requires_approval",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, llmtools.PermissionRequireApproval, needsApproval.Decision)

	denied, err := pm.CheckPermission(context.Background(), &llmtools.PermissionContext{
		ToolName:  "unknown_tool",
		RequestAt: time.Now(),
		Metadata: map[string]string{
			"hosted_tool_risk": "unknown",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, llmtools.PermissionDeny, denied.Decision)
}

func TestBuildAgentToolingRuntime_ApprovalResolutionAllowsRetry(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
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
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: manager,
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL: 15 * time.Minute,
			Scope:    "request",
		},
	}, zap.NewNop())
	require.NoError(t, err)

	call := types.ToolCall{
		ID:        "call-approval",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"ping"}`),
	}

	first := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Error, "approval required")

	pending, err := manager.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	require.NoError(t, manager.ResolveInterrupt(context.Background(), pending[0].ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
		Comment:  "approved in test",
	}))

	second := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Error)
	assert.JSONEq(t, `{"name":"echo-tool","args":{"text":"ping"}}`, string(second[0].Result))
}

func TestBuildAgentToolingRuntime_ApprovalGrantExpiresAfterTTL(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	server := &testMCPServer{
		tools: []mcpproto.ToolDefinition{{
			Name:        "echo-tool",
			Description: "Echo args",
			InputSchema: map[string]any{"type": "object"},
		}},
	}

	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: manager,
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL: 20 * time.Millisecond,
			Scope:    "request",
		},
	}, zap.NewNop())
	require.NoError(t, err)

	call := types.ToolCall{
		ID:        "call-expire",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"ping"}`),
	}
	first := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Error, "approval required")

	pending, err := manager.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, manager.ResolveInterrupt(context.Background(), pending[0].ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
	}))

	second := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Error)

	time.Sleep(40 * time.Millisecond)

	third := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, third, 1)
	assert.Contains(t, third[0].Error, "approval required")
}

func TestBuildAgentToolingRuntime_ApprovalScopeAgentToolReusesGrantAcrossArgs(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	server := &testMCPServer{
		tools: []mcpproto.ToolDefinition{{
			Name:        "echo-tool",
			Description: "Echo args",
			InputSchema: map[string]any{"type": "object"},
		}},
	}

	runtime, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: manager,
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL: 15 * time.Minute,
			Scope:    "agent_tool",
		},
	}, zap.NewNop())
	require.NoError(t, err)

	callA := types.ToolCall{
		ID:        "call-a",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"ping"}`),
	}
	first := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{callA})
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Error, "approval required")

	pending, err := manager.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, manager.ResolveInterrupt(context.Background(), pending[0].ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
	}))

	callB := types.ToolCall{
		ID:        "call-b",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"changed"}`),
	}
	second := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{callB})
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Error)

	otherAgent := runtime.ToolManager.ExecuteForAgent(context.Background(), "agent-b", []types.ToolCall{callB})
	require.Len(t, otherAgent, 1)
	assert.Contains(t, otherAgent[0].Error, "approval required")
}

func TestBuildAgentToolingRuntime_ApprovalGrantPersistsAcrossRuntimeRebuild(t *testing.T) {
	manager := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	server := &testMCPServer{
		tools: []mcpproto.ToolDefinition{{
			Name:        "echo-tool",
			Description: "Echo args",
			InputSchema: map[string]any{"type": "object"},
		}},
	}
	storePath := filepath.Join(t.TempDir(), "tool_approval_grants.json")

	opts := AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: manager,
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL:    15 * time.Minute,
			Scope:       "request",
			PersistPath: storePath,
		},
	}

	runtimeA, err := BuildAgentToolingRuntime(opts, zap.NewNop())
	require.NoError(t, err)

	call := types.ToolCall{
		ID:        "call-persist",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"ping"}`),
	}
	first := runtimeA.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Error, "approval required")

	pending, err := manager.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, manager.ResolveInterrupt(context.Background(), pending[0].ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
	}))

	second := runtimeA.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Error)

	// Rebuild runtime and approval manager to simulate process restart.
	runtimeB, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL:    15 * time.Minute,
			Scope:       "request",
			PersistPath: storePath,
		},
	}, zap.NewNop())
	require.NoError(t, err)

	third := runtimeB.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, third, 1)
	assert.Empty(t, third[0].Error)
	assert.JSONEq(t, `{"name":"echo-tool","args":{"text":"ping"}}`, string(third[0].Result))
}

func TestBuildAgentToolingRuntime_ApprovalGrantPersistsAcrossRuntimeRebuildWithRedisStore(t *testing.T) {
	mr := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer clientA.Close()

	managerA := hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop())
	server := &testMCPServer{
		tools: []mcpproto.ToolDefinition{{
			Name:        "echo-tool",
			Description: "Echo args",
			InputSchema: map[string]any{"type": "object"},
		}},
	}
	store := NewRedisToolApprovalGrantStore(clientA, "agentflow:test:approval", zap.NewNop())

	runtimeA, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: managerA,
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL:   15 * time.Minute,
			Scope:      "request",
			GrantStore: store,
		},
	}, zap.NewNop())
	require.NoError(t, err)

	call := types.ToolCall{
		ID:        "call-redis-persist",
		Name:      "mcp_echo_tool",
		Arguments: json.RawMessage(`{"text":"ping"}`),
	}
	first := runtimeA.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Error, "approval required")

	pending, err := managerA.ListInterrupts(context.Background(), ToolApprovalWorkflowID(), hitl.InterruptStatusPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, managerA.ResolveInterrupt(context.Background(), pending[0].ID, &hitl.Response{
		OptionID: "approve",
		Approved: true,
	}))

	second := runtimeA.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Error)

	clientB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer clientB.Close()
	runtimeB, err := BuildAgentToolingRuntime(AgentToolingOptions{
		MCPServer:           server,
		EnableMCPTools:      true,
		ToolApprovalManager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		ToolApprovalConfig: ToolApprovalConfig{
			GrantTTL:   15 * time.Minute,
			Scope:      "request",
			GrantStore: NewRedisToolApprovalGrantStore(clientB, "agentflow:test:approval", zap.NewNop()),
		},
	}, zap.NewNop())
	require.NoError(t, err)

	third := runtimeB.ToolManager.ExecuteForAgent(context.Background(), "agent-a", []types.ToolCall{call})
	require.Len(t, third, 1)
	assert.Empty(t, third[0].Error)
}

type testVectorStore struct{}

func (s *testVectorStore) AddDocuments(ctx context.Context, docs []core.Document) error { return nil }
func (s *testVectorStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
	return []core.VectorSearchResult{
		{
			Document: core.Document{
				ID:      "doc-1",
				Content: "hello world",
			},
			Score: 0.9,
		},
	}, nil
}
func (s *testVectorStore) DeleteDocuments(ctx context.Context, ids []string) error     { return nil }
func (s *testVectorStore) UpdateDocument(ctx context.Context, doc core.Document) error { return nil }
func (s *testVectorStore) Count(ctx context.Context) (int, error)                      { return 1, nil }

type testEmbeddingProvider struct{}

func (p *testEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return []float64{0.1, 0.2}, nil
}
func (p *testEmbeddingProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return [][]float64{{0.1, 0.2}}, nil
}
func (p *testEmbeddingProvider) Name() string { return "test-embed" }

var (
	_ core.VectorStore       = (*testVectorStore)(nil)
	_ core.EmbeddingProvider = (*testEmbeddingProvider)(nil)
)

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
