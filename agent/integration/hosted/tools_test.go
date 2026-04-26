package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// --- Mock types using function callback pattern (§30) ---

type mockFileSearchStore struct {
	searchFn func(ctx context.Context, query string, limit int) ([]FileSearchResult, error)
	indexFn  func(ctx context.Context, fileID string, content []byte) error
}

func (m *mockFileSearchStore) Search(ctx context.Context, query string, limit int) ([]FileSearchResult, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, query, limit)
	}
	return nil, nil
}

func (m *mockFileSearchStore) Index(ctx context.Context, fileID string, content []byte) error {
	if m.indexFn != nil {
		return m.indexFn(ctx, fileID, content)
	}
	return nil
}

type mockCodeExecutor struct {
	executeFn func(ctx context.Context, language string, code string, timeout time.Duration) (*CodeExecOutput, error)
}

func (m *mockCodeExecutor) Execute(ctx context.Context, language string, code string, timeout time.Duration) (*CodeExecOutput, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, language, code, timeout)
	}
	return &CodeExecOutput{}, nil
}

type mockRetrievalStore struct {
	retrieveFn func(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error)
}

func (m *mockRetrievalStore) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	if m.retrieveFn != nil {
		return m.retrieveFn(ctx, query, topK)
	}
	return nil, nil
}

type hostedRiskTestTool struct {
	typ  HostedToolType
	name string
}

func (t hostedRiskTestTool) Type() HostedToolType { return t.typ }
func (t hostedRiskTestTool) Name() string         { return t.name }
func (t hostedRiskTestTool) Description() string  { return "risk test tool" }
func (t hostedRiskTestTool) Schema() types.ToolSchema {
	return types.ToolSchema{Name: t.name, Parameters: json.RawMessage(`{"type":"object"}`)}
}
func (t hostedRiskTestTool) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// --- ToolRegistry tests ---

func TestToolRegistry_RegisterGetList(t *testing.T) {
	reg := NewToolRegistry(nil)

	tool := NewFileSearchTool(&mockFileSearchStore{}, 5)
	reg.Register(tool)

	got, ok := reg.Get("file_search")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name() != "file_search" {
		t.Errorf("got name %q, want %q", got.Name(), "file_search")
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent tool")
	}

	list := reg.List()
	if len(list) != 1 {
		t.Errorf("got %d tools, want 1", len(list))
	}
}

func TestToolRegistry_GetSchemas(t *testing.T) {
	reg := NewToolRegistry(nil)
	reg.Register(NewFileSearchTool(&mockFileSearchStore{}, 5))

	schemas := reg.GetSchemas()
	if len(schemas) != 1 {
		t.Errorf("got %d schemas, want 1", len(schemas))
	}
}

func TestToolRegistry_Execute_DeniedByPermissionManager(t *testing.T) {
	pm := llmtools.NewPermissionManager(zap.NewNop())
	now := time.Now()
	err := pm.AddRule(&llmtools.PermissionRule{
		ID:          "deny-file-search",
		Name:        "deny file search",
		ToolPattern: "file_search",
		Decision:    llmtools.PermissionDeny,
		Priority:    100,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	reg := NewToolRegistry(nil, WithPermissionManager(pm))
	reg.Register(NewFileSearchTool(&mockFileSearchStore{}, 5))

	_, err = reg.Execute(context.Background(), "file_search", json.RawMessage(`{"query":"test"}`))
	if err == nil {
		t.Fatal("expected permission error")
	}
}

func TestToolRegistry_Execute_ApprovalRequiredForWriteFileRisk(t *testing.T) {
	pm := llmtools.NewPermissionManager(zap.NewNop())
	now := time.Now()
	err := pm.AddRule(&llmtools.PermissionRule{
		ID:          "ask-write-file",
		Name:        "ask write file",
		ToolPattern: "*",
		Decision:    llmtools.PermissionRequireApproval,
		Priority:    100,
		Conditions: []llmtools.RuleCondition{{
			Field:    "hosted_tool_risk",
			Operator: "eq",
			Value:    "requires_approval",
		}},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	dir := t.TempDir()
	reg := NewToolRegistry(nil, WithPermissionManager(pm))
	reg.Register(NewWriteFileTool(FileOpsConfig{AllowedPaths: []string{dir}}))

	target := filepath.Join(dir, "note.txt")
	_, err = reg.Execute(context.Background(), "write_file", json.RawMessage(fmt.Sprintf(`{"path":%q,"content":"hello"}`, target)))
	if err == nil {
		t.Fatal("expected approval error")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("expected file not written, stat err=%v", statErr)
	}
}

func TestClassifyHostedToolAuthorizationContracts(t *testing.T) {
	webSearchTool, webSearchErr := NewProviderBackedWebSearchHostedTool(ToolProviderConfig{
		Provider:       string(ToolProviderDuckDuckGo),
		TimeoutSeconds: 15,
	}, zap.NewNop())
	if webSearchErr != nil {
		t.Fatalf("failed to create provider-backed web search tool: %v", webSearchErr)
	}

	cases := []struct {
		name           string
		tool           HostedTool
		wantResource   types.ResourceKind
		wantTier       types.RiskTier
		wantPolicyRisk string
	}{
		{
			name:           "web search safe read",
			tool:           webSearchTool,
			wantResource:   types.ResourceTool,
			wantTier:       types.RiskSafeRead,
			wantPolicyRisk: "safe_read",
		},
		{
			name:           "file read safe read",
			tool:           NewReadFileTool(FileOpsConfig{}),
			wantResource:   types.ResourceFileRead,
			wantTier:       types.RiskSafeRead,
			wantPolicyRisk: "safe_read",
		},
		{
			name:           "file write mutating",
			tool:           NewWriteFileTool(FileOpsConfig{}),
			wantResource:   types.ResourceFileWrite,
			wantTier:       types.RiskMutating,
			wantPolicyRisk: "requires_approval",
		},
		{
			name:           "code execution",
			tool:           NewCodeExecTool(CodeExecConfig{Executor: &mockCodeExecutor{}}),
			wantResource:   types.ResourceCodeExec,
			wantTier:       types.RiskExecution,
			wantPolicyRisk: "requires_approval",
		},
		{
			name:           "shell execution",
			tool:           NewShellTool(ShellConfig{}),
			wantResource:   types.ResourceShell,
			wantTier:       types.RiskExecution,
			wantPolicyRisk: "requires_approval",
		},
		{
			name:           "mcp network execution",
			tool:           hostedRiskTestTool{typ: ToolTypeMCP, name: "mcp_write"},
			wantResource:   types.ResourceMCPTool,
			wantTier:       types.RiskNetworkExecution,
			wantPolicyRisk: "requires_approval",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyHostedToolResourceKind(tc.tool); got != tc.wantResource {
				t.Fatalf("resource kind = %q, want %q", got, tc.wantResource)
			}
			if got := ClassifyHostedToolRiskTier(tc.tool); got != tc.wantTier {
				t.Fatalf("risk tier = %q, want %q", got, tc.wantTier)
			}
			if got := ClassifyHostedToolPermissionRisk(tc.tool); got != tc.wantPolicyRisk {
				t.Fatalf("permission risk = %q, want %q", got, tc.wantPolicyRisk)
			}
		})
	}
}

// --- FileSearchTool tests ---

func TestFileSearchTool_Execute_Success(t *testing.T) {
	store := &mockFileSearchStore{
		searchFn: func(_ context.Context, query string, limit int) ([]FileSearchResult, error) {
			if query != "find docs" {
				t.Errorf("unexpected query: %s", query)
			}
			return []FileSearchResult{
				{FileID: "f1", FileName: "doc.txt", Content: "content", Score: 0.95},
			}, nil
		},
	}

	tool := NewFileSearchTool(store, 10)
	args, _ := json.Marshal(map[string]any{"query": "find docs"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []FileSearchResult
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FileID != "f1" {
		t.Errorf("unexpected results: %+v", got)
	}
}

func TestFileSearchTool_Execute_StoreError(t *testing.T) {
	store := &mockFileSearchStore{
		searchFn: func(_ context.Context, _ string, _ int) ([]FileSearchResult, error) {
			return nil, fmt.Errorf("store unavailable")
		},
	}

	tool := NewFileSearchTool(store, 10)
	args, _ := json.Marshal(map[string]any{"query": "test"})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error from store")
	}
}

// --- CodeExecTool tests ---

func TestCodeExecTool_Execute_Success(t *testing.T) {
	executor := &mockCodeExecutor{
		executeFn: func(_ context.Context, lang string, code string, _ time.Duration) (*CodeExecOutput, error) {
			if lang != "python" {
				t.Errorf("unexpected language: %s", lang)
			}
			return &CodeExecOutput{
				Stdout:   "hello\n",
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			}, nil
		},
	}

	tool := NewCodeExecTool(CodeExecConfig{Executor: executor})
	args, _ := json.Marshal(map[string]any{"language": "python", "code": "print('hello')"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got CodeExecOutput
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if got.Stdout != "hello\n" {
		t.Errorf("got stdout %q, want %q", got.Stdout, "hello\n")
	}
}

func TestCodeExecTool_Execute_UnsupportedLanguage(t *testing.T) {
	tool := NewCodeExecTool(CodeExecConfig{Executor: &mockCodeExecutor{}})
	args, _ := json.Marshal(map[string]any{"language": "ruby", "code": "puts 'hi'"})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestCodeExecTool_Execute_EmptyCode(t *testing.T) {
	tool := NewCodeExecTool(CodeExecConfig{Executor: &mockCodeExecutor{}})
	args, _ := json.Marshal(map[string]any{"language": "python", "code": ""})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestCodeExecTool_Execute_CustomTimeout(t *testing.T) {
	var receivedTimeout time.Duration
	executor := &mockCodeExecutor{
		executeFn: func(_ context.Context, _ string, _ string, timeout time.Duration) (*CodeExecOutput, error) {
			receivedTimeout = timeout
			return &CodeExecOutput{ExitCode: 0}, nil
		},
	}

	tool := NewCodeExecTool(CodeExecConfig{Executor: executor})
	args, _ := json.Marshal(map[string]any{"language": "bash", "code": "echo hi", "timeout_seconds": 60})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedTimeout != 60*time.Second {
		t.Errorf("got timeout %v, want 60s", receivedTimeout)
	}
}

// --- RetrievalTool tests ---

func TestRetrievalTool_Execute_Success(t *testing.T) {
	store := &mockRetrievalStore{
		retrieveFn: func(_ context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
			if query != "how to deploy" {
				t.Errorf("unexpected query: %s", query)
			}
			if topK != 5 {
				t.Errorf("unexpected topK: %d", topK)
			}
			return []types.RetrievalRecord{
				{DocID: "d1", Content: "deploy guide", Score: 0.9},
			}, nil
		},
	}

	tool := NewRetrievalTool(store, 10, nil)
	args, _ := json.Marshal(map[string]any{"query": "how to deploy", "max_results": 5})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []types.RetrievalRecord
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DocID != "d1" {
		t.Errorf("unexpected results: %+v", got)
	}
}

func TestRetrievalTool_Execute_EmptyQuery(t *testing.T) {
	tool := NewRetrievalTool(&mockRetrievalStore{}, 10, nil)
	args, _ := json.Marshal(map[string]any{"query": ""})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestRetrievalTool_Execute_DefaultMaxResults(t *testing.T) {
	var receivedTopK int
	store := &mockRetrievalStore{
		retrieveFn: func(_ context.Context, _ string, topK int) ([]types.RetrievalRecord, error) {
			receivedTopK = topK
			return nil, nil
		},
	}

	tool := NewRetrievalTool(store, 10, nil)
	args, _ := json.Marshal(map[string]any{"query": "test"})

	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedTopK != 10 {
		t.Errorf("got topK %d, want 10", receivedTopK)
	}
}

// --- Middleware tests ---

func TestMiddleware_WithTimeout(t *testing.T) {
	reg := NewToolRegistry(nil)

	store := &mockFileSearchStore{
		searchFn: func(ctx context.Context, _ string, _ int) ([]FileSearchResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, nil
			}
		},
	}
	reg.Register(NewFileSearchTool(store, 5))
	reg.Use(WithTimeout(50 * time.Millisecond))

	args, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := reg.Execute(context.Background(), "file_search", args)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestMiddleware_WithLogging(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	reg := NewToolRegistry(nil)

	store := &mockFileSearchStore{
		searchFn: func(_ context.Context, _ string, _ int) ([]FileSearchResult, error) {
			return []FileSearchResult{{FileID: "f1"}}, nil
		},
	}
	reg.Register(NewFileSearchTool(store, 5))
	reg.Use(WithLogging(logger))

	args, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := reg.Execute(context.Background(), "file_search", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMiddleware_WithMetrics(t *testing.T) {
	var called bool
	var reportedErr error

	reg := NewToolRegistry(nil)
	store := &mockFileSearchStore{
		searchFn: func(_ context.Context, _ string, _ int) ([]FileSearchResult, error) {
			return nil, nil
		},
	}
	reg.Register(NewFileSearchTool(store, 5))
	reg.Use(WithMetrics(func(_ string, _ time.Duration, err error) {
		called = true
		reportedErr = err
	}))

	args, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := reg.Execute(context.Background(), "file_search", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("metrics callback was not called")
	}
	if reportedErr != nil {
		t.Errorf("unexpected reported error: %v", reportedErr)
	}
}

func TestToolRegistry_Execute_NotFound(t *testing.T) {
	reg := NewToolRegistry(nil)
	args, _ := json.Marshal(map[string]any{"query": "test"})

	_, err := reg.Execute(context.Background(), "nonexistent", args)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestMiddleware_Chain_Order(t *testing.T) {
	var order []string

	mw := func(label string) ToolMiddleware {
		return func(next ToolExecuteFunc) ToolExecuteFunc {
			return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
				order = append(order, label+"-before")
				result, err := next(ctx, args)
				order = append(order, label+"-after")
				return result, err
			}
		}
	}

	reg := NewToolRegistry(nil)
	store := &mockFileSearchStore{
		searchFn: func(_ context.Context, _ string, _ int) ([]FileSearchResult, error) {
			order = append(order, "execute")
			return nil, nil
		},
	}
	reg.Register(NewFileSearchTool(store, 5))
	reg.Use(mw("first"), mw("second"))

	args, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := reg.Execute(context.Background(), "file_search", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"first-before", "second-before", "execute", "second-after", "first-after"}
	if len(order) != len(expected) {
		t.Fatalf("got order %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}
