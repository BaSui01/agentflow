// Package hosted provides hosted tool implementations like Web Search and File Search.
// Implements OpenAI SDK-style hosted tools that run on provider infrastructure.
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

// HostedToolType defines the type of hosted tool.
type HostedToolType string

const (
	ToolTypeWebSearch  HostedToolType = "web_search"
	ToolTypeFileSearch HostedToolType = "file_search"
	ToolTypeCodeExec   HostedToolType = "code_execution"
	ToolTypeRetrieval  HostedToolType = "retrieval"
)

// HostedTool represents a tool hosted by the provider.
type HostedTool interface {
	Type() HostedToolType
	Name() string
	Description() string
	Schema() llm.ToolSchema
	Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// ToolRegistry manages hosted tools.
type ToolRegistry struct {
	tools  map[string]HostedTool
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewToolRegistry creates a new hosted tool registry.
func NewToolRegistry(logger *zap.Logger) *ToolRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolRegistry{
		tools:  make(map[string]HostedTool),
		logger: logger.With(zap.String("component", "hosted_tools")),
	}
}

// Register registers a hosted tool.
func (r *ToolRegistry) Register(tool HostedTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	r.logger.Info("registered hosted tool", zap.String("name", tool.Name()))
}

// Get retrieves a hosted tool by name.
func (r *ToolRegistry) Get(name string) (HostedTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *ToolRegistry) List() []HostedTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]HostedTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetSchemas returns schemas for all tools.
func (r *ToolRegistry) GetSchemas() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schemas := make([]llm.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, t.Schema())
	}
	return schemas
}

// WebSearchTool implements web search functionality.
type WebSearchTool struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
	maxResults int
}

// WebSearchConfig configures the web search tool.
type WebSearchConfig struct {
	APIKey     string
	Endpoint   string
	MaxResults int
	Timeout    time.Duration
}

// NewWebSearchTool creates a new web search tool.
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

// WebSearchArgs represents web search arguments.
type WebSearchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// WebSearchResult represents a search result.
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

	// Build search URL
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

// FileSearchTool implements file search functionality.
type FileSearchTool struct {
	vectorStore VectorStore
	maxResults  int
}

// VectorStore interface for file search.
type VectorStore interface {
	Search(ctx context.Context, query string, limit int) ([]FileSearchResult, error)
	Index(ctx context.Context, fileID string, content []byte) error
}

// FileSearchResult represents a file search result.
type FileSearchResult struct {
	FileID   string         `json:"file_id"`
	FileName string         `json:"file_name"`
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewFileSearchTool creates a new file search tool.
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
