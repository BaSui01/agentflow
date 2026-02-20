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

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// Hosted ToolType 定义了主机工具的类型.
type HostedToolType string

const (
	ToolTypeWebSearch  HostedToolType = "web_search"
	ToolTypeFileSearch HostedToolType = "file_search"
	ToolTypeCodeExec   HostedToolType = "code_execution"
	ToolTypeRetrieval  HostedToolType = "retrieval"
)

// HostToole 代表由提供者托管的工具.
type HostedTool interface {
	Type() HostedToolType
	Name() string
	Description() string
	Schema() llm.ToolSchema
	Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// ToolRegistry 管理主机工具 。
type ToolRegistry struct {
	tools  map[string]HostedTool
	logger *zap.Logger
	mu     sync.RWMutex
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
func (r *ToolRegistry) GetSchemas() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schemas := make([]llm.ToolSchema, 0, len(r.tools))
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
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     config.APIKey,
		endpoint:   config.Endpoint,
		maxResults: maxResults,
	}
}

func (t *WebSearchTool) Type() HostedToolType { return ToolTypeWebSearch }
func (t *WebSearchTool) Name() string         { return "web_search" }
func (t *WebSearchTool) Description() string  { return "Search the web for current information" }

func (t *WebSearchTool) Schema() llm.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query"},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results"},
		},
		"required": []string{"query"},
	})
	return llm.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
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

	maxResults := searchArgs.MaxResults
	if maxResults == 0 {
		maxResults = t.maxResults
	}

	// 构建搜索 URL
	searchURL := fmt.Sprintf("%s?q=%s&max=%d", t.endpoint, url.QueryEscape(searchArgs.Query), maxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// FileSearchTool 执行文件搜索功能.
type FileSearchTool struct {
	vectorStore VectorStore
	maxResults  int
}

// 用于文件搜索的矢量Store接口.
type VectorStore interface {
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
func NewFileSearchTool(store VectorStore, maxResults int) *FileSearchTool {
	if maxResults == 0 {
		maxResults = 10
	}
	return &FileSearchTool{vectorStore: store, maxResults: maxResults}
}

func (t *FileSearchTool) Type() HostedToolType { return ToolTypeFileSearch }
func (t *FileSearchTool) Name() string         { return "file_search" }
func (t *FileSearchTool) Description() string  { return "Search through uploaded files" }

func (t *FileSearchTool) Schema() llm.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query"},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results"},
		},
		"required": []string{"query"},
	})
	return llm.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: params}
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
