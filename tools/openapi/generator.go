// Package openapi provides automatic tool generation from OpenAPI specifications.
// Implements Google ADK-style OpenAPI to Tool conversion.
package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// OpenAPISpec represents a parsed OpenAPI specification.
type OpenAPISpec struct {
	OpenAPI string              `json:"openapi"`
	Info    Info                `json:"info"`
	Servers []Server            `json:"servers,omitempty"`
	Paths   map[string]PathItem `json:"paths"`
}

// Info contains API metadata.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// Server represents an API server.
type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// PathItem represents operations on a path.
type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

// Operation represents an API operation.
type Operation struct {
	OperationID string       `json:"operationId,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Parameters  []Parameter  `json:"parameters,omitempty"`
	RequestBody *RequestBody `json:"requestBody,omitempty"`
	Responses   Responses    `json:"responses,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
}

// Parameter represents an operation parameter.
type Parameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"` // query, path, header, cookie
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Schema      *JSONSchema `json:"schema,omitempty"`
}

// RequestBody represents a request body.
type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType represents a media type.
type MediaType struct {
	Schema *JSONSchema `json:"schema,omitempty"`
}

// Responses represents operation responses.
type Responses map[string]ResponseObj

// ResponseObj represents a response.
type ResponseObj struct {
	Description string               `json:"description,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// JSONSchema represents a JSON Schema.
type JSONSchema struct {
	Type        string                `json:"type,omitempty"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]JSONSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
	Items       *JSONSchema           `json:"items,omitempty"`
	Enum        []any                 `json:"enum,omitempty"`
	Default     any                   `json:"default,omitempty"`
}

// GeneratedTool represents a tool generated from OpenAPI.
type GeneratedTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      llm.ToolSchema `json:"schema"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	BaseURL     string         `json:"base_url"`
	Parameters  []Parameter    `json:"parameters"`
	RequestBody *RequestBody   `json:"request_body,omitempty"`
}

// Generator generates tools from OpenAPI specifications.
type Generator struct {
	httpClient *http.Client
	logger     *zap.Logger
	cache      map[string]*OpenAPISpec
	mu         sync.RWMutex
}

// GeneratorConfig configures the generator.
type GeneratorConfig struct {
	Timeout time.Duration
}

// NewGenerator creates a new OpenAPI tool generator.
func NewGenerator(config GeneratorConfig, logger *zap.Logger) *Generator {
	if logger == nil {
		logger = zap.NewNop()
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Generator{
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger.With(zap.String("component", "openapi_generator")),
		cache:      make(map[string]*OpenAPISpec),
	}
}

// LoadSpec loads an OpenAPI spec from URL or file path.
func (g *Generator) LoadSpec(ctx context.Context, source string) (*OpenAPISpec, error) {
	g.mu.RLock()
	if spec, ok := g.cache[source]; ok {
		g.mu.RUnlock()
		return spec, nil
	}
	g.mu.RUnlock()

	var data []byte
	var err error

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		data, err = g.fetchFromURL(ctx, source)
	} else {
		return nil, fmt.Errorf("file loading not implemented, use URL")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load spec: %w", err)
	}

	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}

	g.mu.Lock()
	g.cache[source] = &spec
	g.mu.Unlock()

	g.logger.Info("loaded OpenAPI spec",
		zap.String("title", spec.Info.Title),
		zap.String("version", spec.Info.Version),
		zap.Int("paths", len(spec.Paths)),
	)

	return &spec, nil
}

func (g *Generator) fetchFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GenerateTools generates tools from an OpenAPI spec.
func (g *Generator) GenerateTools(spec *OpenAPISpec, opts GenerateOptions) ([]*GeneratedTool, error) {
	var tools []*GeneratedTool
	baseURL := ""
	if len(spec.Servers) > 0 {
		baseURL = spec.Servers[0].URL
	}
	if opts.BaseURL != "" {
		baseURL = opts.BaseURL
	}

	for path, pathItem := range spec.Paths {
		operations := map[string]*Operation{
			"GET":    pathItem.Get,
			"POST":   pathItem.Post,
			"PUT":    pathItem.Put,
			"DELETE": pathItem.Delete,
			"PATCH":  pathItem.Patch,
		}

		for method, op := range operations {
			if op == nil {
				continue
			}

			if len(opts.IncludeTags) > 0 && !hasAnyTag(op.Tags, opts.IncludeTags) {
				continue
			}
			if len(opts.ExcludeTags) > 0 && hasAnyTag(op.Tags, opts.ExcludeTags) {
				continue
			}

			tool := g.operationToTool(spec, path, method, op, baseURL)
			tools = append(tools, tool)
		}
	}

	g.logger.Info("generated tools", zap.Int("count", len(tools)))
	return tools, nil
}

func (g *Generator) operationToTool(spec *OpenAPISpec, path, method string, op *Operation, baseURL string) *GeneratedTool {
	name := op.OperationID
	if name == "" {
		name = fmt.Sprintf("%s_%s", strings.ToLower(method), sanitizePath(path))
	}

	description := op.Summary
	if description == "" {
		description = op.Description
	}
	if description == "" {
		description = fmt.Sprintf("%s %s", method, path)
	}

	// Build parameters schema
	properties := make(map[string]JSONSchema)
	var required []string

	for _, param := range op.Parameters {
		properties[param.Name] = JSONSchema{
			Type:        param.Schema.Type,
			Description: param.Description,
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	if op.RequestBody != nil {
		if content, ok := op.RequestBody.Content["application/json"]; ok && content.Schema != nil {
			properties["body"] = *content.Schema
			if op.RequestBody.Required {
				required = append(required, "body")
			}
		}
	}

	paramsSchema := JSONSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}

	paramsJSON, _ := json.Marshal(paramsSchema)

	return &GeneratedTool{
		Name:        name,
		Description: description,
		Schema: llm.ToolSchema{
			Name:        name,
			Description: description,
			Parameters:  paramsJSON,
		},
		Method:      method,
		Path:        path,
		BaseURL:     baseURL,
		Parameters:  op.Parameters,
		RequestBody: op.RequestBody,
	}
}

// GenerateOptions configures tool generation.
type GenerateOptions struct {
	BaseURL     string
	IncludeTags []string
	ExcludeTags []string
	Prefix      string
}

func hasAnyTag(tags, targets []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	for _, t := range targets {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func sanitizePath(path string) string {
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.Trim(path, "_")
	return path
}
