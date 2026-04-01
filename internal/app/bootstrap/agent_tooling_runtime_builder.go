package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var toolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// AgentToolingOptions carries optional dependencies for agent tool wiring.
type AgentToolingOptions struct {
	RetrievalStore      rag.VectorStore
	EmbeddingProvider   rag.EmbeddingProvider
	MCPServer           mcpproto.MCPServer
	EnableMCPTools      bool
	DB                  *gorm.DB
	ToolApprovalManager *hitl.InterruptManager
	ToolApprovalConfig  ToolApprovalConfig
}

// AgentToolingRuntime groups runtime-managed tools exposed to Agent execution.
type AgentToolingRuntime struct {
	Registry    *hosted.ToolRegistry
	ToolManager agent.ToolManager
	ToolNames   []string
	Permissions llmtools.PermissionManager

	db               *gorm.DB
	logger           *zap.Logger
	mu               sync.RWMutex
	baseToolNames    map[string]struct{}
	dynamicToolNames map[string]struct{}
}

// RegisterHostedTool allows application layer to inject custom hosted tools.
// Newly added tool names are appended into ToolNames for resolver whitelist wiring.
func (r *AgentToolingRuntime) RegisterHostedTool(tool hosted.HostedTool) {
	if r == nil || r.Registry == nil || tool == nil {
		return
	}
	r.Registry.Register(tool)
	name := strings.TrimSpace(tool.Name())
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.baseToolNames == nil {
		r.baseToolNames = make(map[string]struct{}, 8)
	}
	r.baseToolNames[name] = struct{}{}
	r.rebuildToolNamesLocked()
}

// BaseToolNames returns built-in runtime target names that registrations may alias.
func (r *AgentToolingRuntime) BaseToolNames() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.baseToolNames))
	for name := range r.baseToolNames {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ReloadBindings reloads DB-managed tool registrations and applies them to the shared registry.
func (r *AgentToolingRuntime) ReloadBindings(ctx context.Context) error {
	if r == nil || r.Registry == nil || r.db == nil {
		return nil
	}
	if err := r.reloadWebSearchProvider(ctx); err != nil {
		return err
	}

	var rows []hosted.ToolRegistration
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("load tool registrations: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for name := range r.dynamicToolNames {
		r.Registry.Unregister(name)
	}
	r.dynamicToolNames = make(map[string]struct{}, len(rows))

	for _, row := range rows {
		aliasName := strings.TrimSpace(row.Name)
		targetName := strings.TrimSpace(row.Target)
		if aliasName == "" || targetName == "" {
			continue
		}
		if aliasName == targetName {
			r.logger.Warn("skip self-referencing tool registration",
				zap.Uint("id", row.ID),
				zap.String("name", aliasName))
			continue
		}
		if _, reserved := r.baseToolNames[aliasName]; reserved {
			r.logger.Warn("skip tool registration using reserved base tool name",
				zap.Uint("id", row.ID),
				zap.String("name", aliasName))
			continue
		}
		if _, ok := r.baseToolNames[targetName]; !ok {
			r.logger.Warn("skip tool registration with unknown target",
				zap.Uint("id", row.ID),
				zap.String("name", aliasName),
				zap.String("target", targetName))
			continue
		}
		targetTool, ok := r.Registry.Get(targetName)
		if !ok || targetTool == nil {
			r.logger.Warn("skip tool registration with unresolved target",
				zap.Uint("id", row.ID),
				zap.String("name", aliasName),
				zap.String("target", targetName))
			continue
		}

		schema := targetTool.Schema()
		schema.Name = aliasName
		if strings.TrimSpace(row.Description) != "" {
			schema.Description = row.Description
		}
		if len(row.Parameters) > 0 && string(row.Parameters) != "null" {
			schema.Parameters = append(json.RawMessage(nil), row.Parameters...)
		}

		r.Registry.Register(newAliasHostedTool(aliasName, targetName, schema, r.Registry))
		r.dynamicToolNames[aliasName] = struct{}{}
	}

	r.rebuildToolNamesLocked()
	return nil
}

// BuildAgentToolingRuntime creates a hosted tool registry and ToolManager bridge
// for agent runtime. It supports retrieval (RAG) and optional MCP tool bridging.
func BuildAgentToolingRuntime(opts AgentToolingOptions, logger *zap.Logger) (*AgentToolingRuntime, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	permissionManager := newDefaultToolPermissionManager(logger)
	if approvalAware, ok := permissionManager.(*llmtools.DefaultPermissionManager); ok && opts.ToolApprovalManager != nil {
		approvalAware.SetApprovalHandler(newToolApprovalHandler(opts.ToolApprovalManager, opts.ToolApprovalConfig, logger))
	}
	registry := hosted.NewToolRegistry(logger, hosted.WithPermissionManager(permissionManager))

	baseToolNames := make(map[string]struct{}, 8)
	appendTool := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		baseToolNames[name] = struct{}{}
	}

	if opts.RetrievalStore != nil && opts.EmbeddingProvider != nil {
		retrievalTool := hosted.NewRetrievalTool(
			ragHostedToolRetrievalStore{
				store:    opts.RetrievalStore,
				embedder: opts.EmbeddingProvider,
			},
			10,
			logger,
		)
		registry.Register(retrievalTool)
		appendTool(retrievalTool.Name())
	}

	if opts.EnableMCPTools && opts.MCPServer != nil {
		tools, err := opts.MCPServer.ListTools(context.Background())
		if err != nil {
			return nil, fmt.Errorf("list mcp tools: %w", err)
		}
		for _, def := range tools {
			name := toMCPToolAlias(def.Name)
			registry.Register(newMCPHostedTool(opts.MCPServer, def, name, logger))
			appendTool(name)
		}
	}

	var manager agent.ToolManager
	if len(registry.List()) > 0 {
		manager = newHostedToolManager(registry, permissionManager, logger)
	}

	runtime := &AgentToolingRuntime{
		Registry:         registry,
		ToolManager:      manager,
		Permissions:      permissionManager,
		db:               opts.DB,
		logger:           logger.With(zap.String("component", "agent_tooling_runtime")),
		baseToolNames:    baseToolNames,
		dynamicToolNames: make(map[string]struct{}, 8),
	}
	runtime.rebuildToolNamesLocked()
	if err := runtime.ReloadBindings(context.Background()); err != nil {
		return nil, err
	}

	return runtime, nil
}

func (r *AgentToolingRuntime) rebuildToolNamesLocked() {
	merged := make(map[string]struct{}, len(r.baseToolNames)+len(r.dynamicToolNames))
	for name := range r.baseToolNames {
		merged[name] = struct{}{}
	}
	for name := range r.dynamicToolNames {
		merged[name] = struct{}{}
	}

	out := make([]string, 0, len(merged))
	for name := range merged {
		out = append(out, name)
	}
	sort.Strings(out)
	r.ToolNames = out
}

func (r *AgentToolingRuntime) reloadWebSearchProvider(ctx context.Context) error {
	var providers []hosted.ToolProviderConfig
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&providers).Error; err != nil {
		return fmt.Errorf("load web search providers: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// clear previous runtime-managed web_search entry.
	delete(r.baseToolNames, "web_search")
	r.Registry.Unregister("web_search")

	for _, row := range providers {
		tool, err := hosted.NewProviderBackedWebSearchHostedTool(row, r.logger)
		if err != nil {
			r.logger.Warn("skip invalid web search provider config",
				zap.Uint("id", row.ID),
				zap.String("provider", row.Provider),
				zap.Error(err))
			continue
		}
		r.Registry.Register(tool)
		r.baseToolNames[tool.Name()] = struct{}{}
		r.logger.Info("web search provider activated",
			zap.Uint("id", row.ID),
			zap.String("provider", row.Provider),
			zap.Int("priority", row.Priority))
		break
	}

	r.rebuildToolNamesLocked()
	return nil
}

type aliasHostedTool struct {
	name     string
	target   string
	schema   types.ToolSchema
	registry *hosted.ToolRegistry
}

func newAliasHostedTool(name string, target string, schema types.ToolSchema, registry *hosted.ToolRegistry) hosted.HostedTool {
	return &aliasHostedTool{
		name:     strings.TrimSpace(name),
		target:   strings.TrimSpace(target),
		schema:   schema,
		registry: registry,
	}
}

func (t *aliasHostedTool) Type() hosted.HostedToolType { return hosted.ToolTypeAlias }
func (t *aliasHostedTool) Name() string                { return t.name }
func (t *aliasHostedTool) Description() string {
	return t.schema.Description
}
func (t *aliasHostedTool) Schema() types.ToolSchema {
	return t.schema
}
func (t *aliasHostedTool) PermissionRisk() string {
	switch strings.TrimSpace(t.target) {
	case "web_search", "file_search", "retrieval", "read_file", "list_directory":
		return "safe_read"
	case "write_file", "edit_file", "run_command", "code_execution":
		return "requires_approval"
	default:
		if strings.HasPrefix(strings.TrimSpace(t.target), "mcp_") {
			return "requires_approval"
		}
		return "unknown"
	}
}
func (t *aliasHostedTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.registry == nil {
		return nil, fmt.Errorf("tool registry is not configured")
	}
	return t.registry.Execute(ctx, t.target, args)
}

type hostedToolManager struct {
	registry    *hosted.ToolRegistry
	permissions llmtools.PermissionManager
	logger      *zap.Logger
}

func newHostedToolManager(
	registry *hosted.ToolRegistry,
	permissions llmtools.PermissionManager,
	logger *zap.Logger,
) *hostedToolManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &hostedToolManager{
		registry:    registry,
		permissions: permissions,
		logger:      logger.With(zap.String("component", "agent_tool_manager")),
	}
}

func (m *hostedToolManager) GetAllowedTools(agentID string) []types.ToolSchema {
	if m == nil || m.registry == nil {
		return nil
	}
	return filterToolSchemasByAgentPermission(m.permissions, agentID, m.registry.GetSchemas())
}

func (m *hostedToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llmtools.ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c types.ToolCall) {
			defer wg.Done()

			if err := ctx.Err(); err != nil {
				out[idx] = llmtools.ToolResult{
					ToolCallID: c.ID,
					Name:       c.Name,
					Error:      err.Error(),
				}
				return
			}

			callCtx := types.WithAgentID(ctx, agentID)

			start := time.Now()
			raw, err := m.registry.Execute(callCtx, c.Name, c.Arguments)
			out[idx] = llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Duration:   time.Since(start),
			}
			if err != nil {
				out[idx].Error = err.Error()
				return
			}
			out[idx].Result = raw
		}(i, call)
	}
	wg.Wait()
	return out
}

type ragHostedToolRetrievalStore struct {
	store    rag.VectorStore
	embedder rag.EmbeddingProvider
}

func (s ragHostedToolRetrievalStore) Retrieve(ctx context.Context, query string, topK int) ([]hosted.RetrievalResult, error) {
	if s.store == nil || s.embedder == nil {
		return nil, fmt.Errorf("agent retrieval dependencies are not configured")
	}
	emb, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	results, err := s.store.Search(ctx, emb, topK)
	if err != nil {
		return nil, err
	}
	out := make([]hosted.RetrievalResult, 0, len(results))
	for _, item := range results {
		out = append(out, hosted.RetrievalResult{
			DocumentID: item.Document.ID,
			Content:    item.Document.Content,
			Score:      item.Score,
			Metadata:   item.Document.Metadata,
		})
	}
	return out, nil
}

type mcpHostedTool struct {
	server       mcpproto.MCPServer
	definition   mcpproto.ToolDefinition
	exposedName  string
	exposedParam json.RawMessage
}

func newMCPHostedTool(server mcpproto.MCPServer, def mcpproto.ToolDefinition, exposedName string, logger *zap.Logger) hosted.HostedTool {
	params := json.RawMessage(`{"type":"object","additionalProperties":true}`)
	if len(def.InputSchema) > 0 {
		if raw, err := json.Marshal(def.InputSchema); err == nil {
			params = raw
		}
	}
	return &mcpHostedTool{
		server:       server,
		definition:   def,
		exposedName:  exposedName,
		exposedParam: params,
	}
}

func (t *mcpHostedTool) Type() hosted.HostedToolType { return hosted.ToolTypeMCP }
func (t *mcpHostedTool) Name() string                { return t.exposedName }
func (t *mcpHostedTool) Description() string {
	if strings.TrimSpace(t.definition.Description) == "" {
		return "MCP bridged tool"
	}
	return t.definition.Description
}
func (t *mcpHostedTool) Schema() types.ToolSchema {
	return types.ToolSchema{
		Name:        t.exposedName,
		Description: t.Description(),
		Parameters:  t.exposedParam,
	}
}

func (t *mcpHostedTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.server == nil {
		return nil, fmt.Errorf("mcp server is not configured")
	}
	payload := make(map[string]any)
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &payload); err != nil {
			return nil, fmt.Errorf("invalid mcp tool args: %w", err)
		}
	}
	result, err := t.server.CallTool(ctx, t.definition.Name, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func toMCPToolAlias(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "mcp_tool"
	}
	n = toolNameSanitizer.ReplaceAllString(n, "_")
	n = strings.Trim(n, "_")
	if n == "" {
		n = "tool"
	}
	return "mcp_" + n
}
