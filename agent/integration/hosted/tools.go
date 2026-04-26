package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/pkg/metrics"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Hosted ToolType 定义了主机工具的类型.
type HostedToolType string

const (
	ToolTypeWebSearch  HostedToolType = "web_search"
	ToolTypeFileSearch HostedToolType = "file_search"
	ToolTypeCodeExec   HostedToolType = "code_execution"
	ToolTypeRetrieval  HostedToolType = "retrieval"
	ToolTypeMCP        HostedToolType = "mcp"
	ToolTypeAlias      HostedToolType = "alias"
	ToolTypeFileOps    HostedToolType = "file_ops"
	ToolTypeShell      HostedToolType = "shell"

	maxResponseSize = 1 << 20
)

// HostToole 代表由提供者托管的工具.
type HostedTool interface {
	Type() HostedToolType
	Name() string
	Description() string
	Schema() types.ToolSchema
	Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// ToolExecuteFunc is the function signature for tool execution.
type ToolExecuteFunc func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// ToolMiddleware wraps tool execution with cross-cutting concerns.
type ToolMiddleware func(next ToolExecuteFunc) ToolExecuteFunc

// PermissionRiskReporter lets tools expose a stable permission risk classification.
type PermissionRiskReporter interface {
	PermissionRisk() string
}

// ToolRegistry manages hosted tools.
type ToolRegistry struct {
	tools       map[string]HostedTool
	middlewares []ToolMiddleware
	logger      *zap.Logger
	metrics     *metrics.Collector
	permissions llmtools.PermissionManager
	mu          sync.RWMutex
}

// NewToolRegistry创建了新的主机工具注册.
func NewToolRegistry(logger *zap.Logger, opts ...ToolRegistryOption) *ToolRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	r := &ToolRegistry{
		tools:  make(map[string]HostedTool),
		logger: logger.With(zap.String("component", "hosted_tools")),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ToolRegistryOption configures optional dependencies for ToolRegistry.
type ToolRegistryOption func(*ToolRegistry)

// WithToolMetrics injects a Prometheus metrics collector for tool call instrumentation.
func WithToolMetrics(c *metrics.Collector) ToolRegistryOption {
	return func(r *ToolRegistry) { r.metrics = c }
}

// WithPermissionManager injects the permission manager used by hosted tool execution.
func WithPermissionManager(pm llmtools.PermissionManager) ToolRegistryOption {
	return func(r *ToolRegistry) { r.permissions = pm }
}

// 注册注册一个主机工具 。
func (r *ToolRegistry) Register(tool HostedTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	r.logger.Info("registered hosted tool", zap.String("name", tool.Name()))
}

// Unregister removes a hosted tool by name.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; !ok {
		return
	}
	delete(r.tools, name)
	r.logger.Info("unregistered hosted tool", zap.String("name", name))
}

// 按名称获取主机工具 。
func (r *ToolRegistry) Get(name string) (HostedTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// 列表返回所有已注册的工具 。
func (r *ToolRegistry) List() []HostedTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]HostedTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetSchemas 返回所有工具的策略 。
func (r *ToolRegistry) GetSchemas() []types.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schemas := make([]types.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, t.Schema())
	}
	return schemas
}

// FileSearchTool 执行文件搜索功能.
type FileSearchTool struct {
	vectorStore FileSearchStore
	maxResults  int
}

// FileSearchStore is the store interface for file search operations.
// It operates on text queries (not raw vectors), distinct from rag.VectorStore.
type FileSearchStore interface {
	Search(ctx context.Context, query string, limit int) ([]FileSearchResult, error)
	Index(ctx context.Context, fileID string, content []byte) error
}

// FileSearchResult代表文件搜索结果.
type FileSearchResult struct {
	FileID   string         `json:"file_id"`
	FileName string         `json:"file_name"`
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewFileSearchTool创建了一个新的文件搜索工具.
func NewFileSearchTool(store FileSearchStore, maxResults int) *FileSearchTool {
	if maxResults == 0 {
		maxResults = 10
	}
	return &FileSearchTool{vectorStore: store, maxResults: maxResults}
}

func (t *FileSearchTool) Type() HostedToolType { return ToolTypeFileSearch }
func (t *FileSearchTool) Name() string         { return "file_search" }
func (t *FileSearchTool) Description() string  { return "Search through uploaded files" }

func (t *FileSearchTool) Schema() types.ToolSchema {
	params, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query"},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results"},
		},
		"required": []string{"query"},
	})
	if err != nil {
		params = []byte("{}")
	}
	return types.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
}

func (t *FileSearchTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var searchArgs struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results,omitempty"`
	}
	if err := json.Unmarshal(args, &searchArgs); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	maxResults := searchArgs.MaxResults
	if maxResults == 0 {
		maxResults = t.maxResults
	}

	results, err := t.vectorStore.Search(ctx, searchArgs.Query, maxResults)
	if err != nil {
		return nil, err
	}

	return json.Marshal(results)
}

// Use appends middleware(s) to the registry's middleware chain.
func (r *ToolRegistry) Use(middleware ...ToolMiddleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, middleware...)
}

// Execute looks up a tool by name and executes it with the middleware chain applied.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	middlewares := make([]ToolMiddleware, len(r.middlewares))
	copy(middlewares, r.middlewares)
	mc := r.metrics
	pm := r.permissions
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	if pm != nil {
		permCtx := buildPermissionContext(ctx, tool, args)
		ctx = llmtools.WithPermissionContext(ctx, permCtx)
		result, err := pm.CheckPermission(ctx, permCtx)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		switch result.Decision {
		case llmtools.PermissionAllow:
		case llmtools.PermissionDeny:
			return nil, fmt.Errorf("permission denied: %s", result.Reason)
		case llmtools.PermissionRequireApproval:
			if result.ApprovalID != "" {
				return nil, fmt.Errorf("approval required (ID: %s): %s", result.ApprovalID, result.Reason)
			}
			return nil, fmt.Errorf("approval required: %s", result.Reason)
		default:
			return nil, fmt.Errorf("unknown permission decision: %s", result.Decision)
		}
	}

	var fn ToolExecuteFunc = tool.Execute
	for i := len(middlewares) - 1; i >= 0; i-- {
		fn = middlewares[i](fn)
	}

	start := time.Now()
	result, err := fn(ctx, args)
	if mc != nil {
		status := "success"
		if err != nil {
			status = "error"
		}
		mc.RecordToolCall(name, status, time.Since(start))
	}
	return result, err
}

// WithTimeout returns a middleware that enforces an execution timeout.
func WithTimeout(d time.Duration) ToolMiddleware {
	return func(next ToolExecuteFunc) ToolExecuteFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, args)
		}
	}
}

// WithLogging returns a middleware that logs tool invocations.
func WithLogging(logger *zap.Logger) ToolMiddleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next ToolExecuteFunc) ToolExecuteFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			start := time.Now()
			result, err := next(ctx, args)
			duration := time.Since(start)
			if err != nil {
				logger.Warn("tool execution failed",
					zap.Duration("duration", duration),
					zap.Error(err),
				)
			} else {
				logger.Debug("tool execution completed",
					zap.Duration("duration", duration),
				)
			}
			return result, err
		}
	}
}

// WithMetrics returns a middleware that calls a metrics callback after execution.
func WithMetrics(onExecute func(name string, duration time.Duration, err error)) ToolMiddleware {
	return func(next ToolExecuteFunc) ToolExecuteFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			start := time.Now()
			result, err := next(ctx, args)
			if onExecute != nil {
				onExecute("", time.Since(start), err)
			}
			return result, err
		}
	}
}

func buildPermissionContext(ctx context.Context, tool HostedTool, args json.RawMessage) *llmtools.PermissionContext {
	permCtx, ok := llmtools.GetPermissionContext(ctx)
	if !ok || permCtx == nil {
		permCtx = &llmtools.PermissionContext{}
	} else {
		cloned := *permCtx
		if permCtx.Roles != nil {
			cloned.Roles = append([]string(nil), permCtx.Roles...)
		}
		if permCtx.Metadata != nil {
			cloned.Metadata = make(map[string]string, len(permCtx.Metadata))
			for key, value := range permCtx.Metadata {
				cloned.Metadata[key] = value
			}
		}
		if permCtx.Arguments != nil {
			cloned.Arguments = make(map[string]any, len(permCtx.Arguments))
			for key, value := range permCtx.Arguments {
				cloned.Arguments[key] = value
			}
		}
		permCtx = &cloned
	}

	toolName := ""
	if tool != nil {
		toolName = tool.Name()
	}
	permCtx.ToolName = toolName
	if permCtx.RequestAt.IsZero() {
		permCtx.RequestAt = time.Now()
	}
	if permCtx.Metadata == nil {
		permCtx.Metadata = make(map[string]string)
	}

	if permCtx.AgentID == "" {
		if agentID, ok := types.AgentID(ctx); ok {
			permCtx.AgentID = agentID
		}
	}
	if permCtx.UserID == "" {
		if userID, ok := types.UserID(ctx); ok {
			permCtx.UserID = userID
		}
	}
	if len(permCtx.Roles) == 0 {
		if roles, ok := types.Roles(ctx); ok {
			permCtx.Roles = roles
		}
	}
	if permCtx.TraceID == "" {
		if traceID, ok := types.TraceID(ctx); ok {
			permCtx.TraceID = traceID
		}
	}
	if permCtx.SessionID == "" {
		if runID, ok := types.RunID(ctx); ok {
			permCtx.SessionID = runID
			permCtx.Metadata["run_id"] = runID
		}
	}
	if spanID, ok := types.SpanID(ctx); ok {
		permCtx.Metadata["span_id"] = spanID
	}
	if tool != nil {
		permCtx.Metadata["hosted_tool_type"] = string(tool.Type())
		permCtx.Metadata["hosted_tool_risk"] = ClassifyHostedToolPermissionRisk(tool)
	}

	if len(args) > 0 && string(args) != "null" {
		var arguments map[string]any
		if err := json.Unmarshal(args, &arguments); err == nil && arguments != nil {
			permCtx.Arguments = arguments
		}
	}

	return permCtx
}

// ClassifyHostedToolPermissionRisk returns the stable policy metadata value
// used by PermissionManager rules.
func ClassifyHostedToolPermissionRisk(tool HostedTool) string {
	if tool == nil {
		return "unknown"
	}

	name := strings.TrimSpace(tool.Name())
	switch tool.Type() {
	case ToolTypeWebSearch, ToolTypeFileSearch, ToolTypeRetrieval:
		return "safe_read"
	case ToolTypeShell, ToolTypeCodeExec, ToolTypeMCP:
		return "requires_approval"
	case ToolTypeFileOps:
		switch name {
		case "read_file", "list_directory":
			return "safe_read"
		case "write_file", "edit_file":
			return "requires_approval"
		default:
			return "unknown"
		}
	case ToolTypeAlias:
		if reporter, ok := tool.(PermissionRiskReporter); ok {
			return reporter.PermissionRisk()
		}
		return "unknown"
	default:
		return "unknown"
	}
}

// ClassifyHostedToolRiskTier maps hosted tool capability to the shared
// authorization RiskTier contract.
func ClassifyHostedToolRiskTier(tool HostedTool) types.RiskTier {
	switch ClassifyHostedToolPermissionRisk(tool) {
	case "safe_read":
		return types.RiskSafeRead
	case "sensitive_read":
		return types.RiskSensitiveRead
	case "mutating":
		return types.RiskMutating
	}
	if tool == nil {
		return types.RiskExecution
	}
	switch tool.Type() {
	case ToolTypeMCP:
		return types.RiskNetworkExecution
	case ToolTypeFileOps:
		switch strings.TrimSpace(tool.Name()) {
		case "write_file", "edit_file":
			return types.RiskMutating
		}
		return types.RiskExecution
	default:
		return types.RiskExecution
	}
}

// ClassifyHostedToolResourceKind maps hosted tools to shared authorization
// resource kinds so agent, workflow, and MCP hosted paths speak the same policy language.
func ClassifyHostedToolResourceKind(tool HostedTool) types.ResourceKind {
	if tool == nil {
		return types.ResourceTool
	}
	switch tool.Type() {
	case ToolTypeMCP:
		return types.ResourceMCPTool
	case ToolTypeShell:
		return types.ResourceShell
	case ToolTypeCodeExec:
		return types.ResourceCodeExec
	case ToolTypeFileOps:
		switch strings.TrimSpace(tool.Name()) {
		case "read_file", "list_directory":
			return types.ResourceFileRead
		case "write_file", "edit_file":
			return types.ResourceFileWrite
		}
	}
	return types.ResourceTool
}

func classifyAliasRisk(target string) string {
	switch strings.TrimSpace(target) {
	case "web_search", "file_search", "retrieval", "read_file", "list_directory":
		return "safe_read"
	case "write_file", "edit_file", "run_command", "code_execution":
		return "requires_approval"
	default:
		if strings.HasPrefix(strings.TrimSpace(target), "mcp_") {
			return "requires_approval"
		}
		return "unknown"
	}
}
