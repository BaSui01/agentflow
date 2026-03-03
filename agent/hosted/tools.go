package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
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

	// maxResponseSize is the maximum allowed response body size (1MB).
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

// ToolRegistry manages hosted tools.
type ToolRegistry struct {
	tools       map[string]HostedTool
	middlewares []ToolMiddleware
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewToolRegistry创建了新的主机工具注册.
func NewToolRegistry(logger *zap.Logger) *ToolRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolRegistry{
		tools:  make(map[string]HostedTool),
		logger: logger.With(zap.String("component", "hosted_tools")),
	}
}

// 注册注册一个主机工具 。
func (r *ToolRegistry) Register(tool HostedTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	r.logger.Info("registered hosted tool", zap.String("name", tool.Name()))
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

// WebSearchTool执行网络搜索功能.
type WebSearchTool struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
	maxResults int
}

// WebSearchConfig 配置了网络搜索工具.
type WebSearchConfig struct {
	APIKey     string
	Endpoint   string
	MaxResults int
	Timeout    time.Duration
}

// 新WebSearchTooll创建了新的网络搜索工具.
func NewWebSearchTool(config WebSearchConfig) *WebSearchTool {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	maxResults := config.MaxResults
	if maxResults == 0 {
		maxResults = 10
	}
	return &WebSearchTool{
		httpClient: tlsutil.SecureHTTPClient(timeout),
		apiKey:     config.APIKey,
		endpoint:   config.Endpoint,
		maxResults: maxResults,
	}
}

func (t *WebSearchTool) Type() HostedToolType { return ToolTypeWebSearch }
func (t *WebSearchTool) Name() string         { return "web_search" }
func (t *WebSearchTool) Description() string  { return "Search the web for current information" }

func (t *WebSearchTool) Schema() types.ToolSchema {
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

// WebSearchArgs 代表网络搜索参数.
type WebSearchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// WebSearchResult代表搜索结果.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var searchArgs WebSearchArgs
	if err := json.Unmarshal(args, &searchArgs); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if searchArgs.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	maxResults := searchArgs.MaxResults
	if maxResults == 0 {
		maxResults = t.maxResults
	}

	// Build search URL
	searchURL := fmt.Sprintf("%s?q=%s&max=%d", t.endpoint, url.QueryEscape(searchArgs.Query), maxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	// Enforce 1MB response size limit
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response into structured results
	var results []WebSearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return json.Marshal(results)
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
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Build the middleware chain: last middleware wraps first, so iterate in reverse.
	var fn ToolExecuteFunc = tool.Execute
	for i := len(middlewares) - 1; i >= 0; i-- {
		fn = middlewares[i](fn)
	}

	return fn(ctx, args)
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
