package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	a2aprotocol "github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
	mcpprotocol "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"github.com/BaSui01/agentflow/types"
)

// =============================================================================
// MCP DTO Types
// =============================================================================

// MCPResourceDTO represents an MCP resource in API responses.
type MCPResourceDTO struct {
	URI         string         `json:"uri"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	MimeType    string         `json:"mimeType,omitempty"`
	Content     any            `json:"content,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Size        int64          `json:"size,omitempty"`
	CreatedAt   time.Time      `json:"createdAt,omitempty"`
	UpdatedAt   time.Time      `json:"updatedAt,omitempty"`
}

// MCPToolDTO represents an MCP tool definition in API responses.
type MCPToolDTO struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// MCPServerInfoDTO represents MCP server information.
type MCPServerInfoDTO struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
}

// MCPCallToolResultDTO represents the result of an MCP tool call.
type MCPCallToolResultDTO struct {
	Tool   string `json:"tool"`
	Result any    `json:"result"`
}

// MCPListResourcesResultDTO represents the result of listing MCP resources.
type MCPListResourcesResultDTO struct {
	Resources []MCPResourceDTO `json:"resources"`
}

// MCPListToolsResultDTO represents the result of listing MCP tools.
type MCPListToolsResultDTO struct {
	Tools []MCPToolDTO `json:"tools"`
}

// =============================================================================
// A2A DTO Types
// =============================================================================

// A2ACapabilityDTO represents an A2A agent capability.
type A2ACapabilityDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// A2AToolDTO represents an A2A tool definition.
type A2AToolDTO struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Version     string         `json:"version,omitempty"`
}

// A2AAgentCardDTO represents an A2A Agent Card in API responses.
type A2AAgentCardDTO struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	URL          string               `json:"url"`
	Version      string               `json:"version"`
	Capabilities []A2ACapabilityDTO   `json:"capabilities"`
	InputSchema  map[string]any       `json:"input_schema,omitempty"`
	OutputSchema map[string]any       `json:"output_schema,omitempty"`
	Tools        []A2AToolDTO         `json:"tools,omitempty"`
	Metadata     map[string]string    `json:"metadata,omitempty"`
}

// =============================================================================
// ProtocolBridgeService Interface
// =============================================================================

// ProtocolBridgeService encapsulates MCP and A2A protocol operations.
// It provides a clean abstraction layer between the API handlers and
// the protocol implementations, ensuring proper layer separation.
type ProtocolBridgeService interface {
	// MCP Operations

	// ListMCPResources lists all available MCP resources.
	ListMCPResources(ctx context.Context) (*MCPListResourcesResultDTO, *types.Error)

	// GetMCPResource retrieves a specific MCP resource by URI.
	GetMCPResource(ctx context.Context, uri string) (*MCPResourceDTO, *types.Error)

	// ListMCPTools lists all available MCP tools.
	ListMCPTools(ctx context.Context) (*MCPListToolsResultDTO, *types.Error)

	// CallMCPTool invokes an MCP tool with the given arguments.
	CallMCPTool(ctx context.Context, name string, arguments map[string]any) (*MCPCallToolResultDTO, *types.Error)

	// A2A Operations

	// GetA2AAgentCard retrieves the Agent Card for a specific agent.
	GetA2AAgentCard(agentID string) (*A2AAgentCardDTO, *types.Error)

	// ServeA2ARequest handles an A2A HTTP request and writes the response.
	ServeA2ARequest(w http.ResponseWriter, r *http.Request)
}

// =============================================================================
// Default Implementation
// =============================================================================

// DefaultProtocolBridgeService is the default implementation of ProtocolBridgeService.
type DefaultProtocolBridgeService struct {
	mcpServer mcpprotocol.MCPServer
	a2aServer a2aprotocol.A2AServer
}

// NewProtocolBridgeService creates a new ProtocolBridgeService.
func NewProtocolBridgeService(mcpServer mcpprotocol.MCPServer, a2aServer a2aprotocol.A2AServer) *DefaultProtocolBridgeService {
	return &DefaultProtocolBridgeService{
		mcpServer: mcpServer,
		a2aServer: a2aServer,
	}
}

// =============================================================================
// MCP Operations Implementation
// =============================================================================

func (s *DefaultProtocolBridgeService) ListMCPResources(ctx context.Context) (*MCPListResourcesResultDTO, *types.Error) {
	if s.mcpServer == nil {
		return nil, types.NewServiceUnavailableError("MCP server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}

	resources, err := s.mcpServer.ListResources(ctx)
	if err != nil {
		return nil, types.NewInternalError("failed to list resources").WithCause(err)
	}

	dtos := make([]MCPResourceDTO, 0, len(resources))
	for _, res := range resources {
		dtos = append(dtos, toMCPResourceDTO(res))
	}

	return &MCPListResourcesResultDTO{Resources: dtos}, nil
}

func (s *DefaultProtocolBridgeService) GetMCPResource(ctx context.Context, uri string) (*MCPResourceDTO, *types.Error) {
	if s.mcpServer == nil {
		return nil, types.NewServiceUnavailableError("MCP server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}

	if uri == "" {
		return nil, types.NewInvalidRequestError("resource URI is required").
			WithHTTPStatus(http.StatusBadRequest)
	}

	resource, err := s.mcpServer.GetResource(ctx, uri)
	if err != nil {
		return nil, types.NewNotFoundError("resource not found: " + uri).
			WithCause(err).
			WithHTTPStatus(http.StatusNotFound)
	}

	dto := toMCPResourceDTO(*resource)
	return &dto, nil
}

func (s *DefaultProtocolBridgeService) ListMCPTools(ctx context.Context) (*MCPListToolsResultDTO, *types.Error) {
	if s.mcpServer == nil {
		return nil, types.NewServiceUnavailableError("MCP server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}

	tools, err := s.mcpServer.ListTools(ctx)
	if err != nil {
		return nil, types.NewInternalError("failed to list tools").WithCause(err)
	}

	dtos := make([]MCPToolDTO, 0, len(tools))
	for _, tool := range tools {
		dtos = append(dtos, toMCPToolDTO(tool))
	}

	return &MCPListToolsResultDTO{Tools: dtos}, nil
}

func (s *DefaultProtocolBridgeService) CallMCPTool(ctx context.Context, name string, arguments map[string]any) (*MCPCallToolResultDTO, *types.Error) {
	if s.mcpServer == nil {
		return nil, types.NewServiceUnavailableError("MCP server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}

	if name == "" {
		return nil, types.NewInvalidRequestError("tool name is required").
			WithHTTPStatus(http.StatusBadRequest)
	}

	result, err := s.mcpServer.CallTool(ctx, name, arguments)
	if err != nil {
		return nil, types.NewInternalError("tool call failed: " + err.Error()).WithCause(err)
	}

	return &MCPCallToolResultDTO{
		Tool:   name,
		Result: result,
	}, nil
}

// =============================================================================
// A2A Operations Implementation
// =============================================================================

func (s *DefaultProtocolBridgeService) GetA2AAgentCard(agentID string) (*A2AAgentCardDTO, *types.Error) {
	if s.a2aServer == nil {
		return nil, types.NewServiceUnavailableError("A2A server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
	}

	if agentID == "" {
		return nil, types.NewInvalidRequestError("agent_id is required").
			WithHTTPStatus(http.StatusBadRequest)
	}

	card, err := s.a2aServer.GetAgentCard(agentID)
	if err != nil {
		return nil, types.NewNotFoundError("agent not found: " + agentID).
			WithCause(err).
			WithHTTPStatus(http.StatusNotFound)
	}

	return toA2AAgentCardDTO(card), nil
}

func (s *DefaultProtocolBridgeService) ServeA2ARequest(w http.ResponseWriter, r *http.Request) {
	if s.a2aServer == nil {
		apiErr := types.NewServiceUnavailableError("A2A server is not configured").
			WithHTTPStatus(http.StatusServiceUnavailable)
		WriteJSONError(w, apiErr)
		return
	}

	s.a2aServer.ServeHTTP(w, r)
}

// =============================================================================
// Conversion Helpers
// =============================================================================

func toMCPResourceDTO(res mcpprotocol.Resource) MCPResourceDTO {
	return MCPResourceDTO{
		URI:         res.URI,
		Name:        res.Name,
		Description: res.Description,
		Type:        string(res.Type),
		MimeType:    res.MimeType,
		Content:     res.Content,
		Metadata:    res.Metadata,
		Size:        res.Size,
		CreatedAt:   res.CreatedAt,
		UpdatedAt:   res.UpdatedAt,
	}
}

func toMCPToolDTO(tool mcpprotocol.ToolDefinition) MCPToolDTO {
	return MCPToolDTO{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
	}
}

func toA2AAgentCardDTO(card *a2aprotocol.AgentCard) *A2AAgentCardDTO {
	if card == nil {
		return nil
	}

	capabilities := make([]A2ACapabilityDTO, 0, len(card.Capabilities))
	for _, cap := range card.Capabilities {
		capabilities = append(capabilities, A2ACapabilityDTO{
			Name:        cap.Name,
			Description: cap.Description,
			Type:        string(cap.Type),
		})
	}

	tools := make([]A2AToolDTO, 0, len(card.Tools))
	for _, tool := range card.Tools {
		tools = append(tools, A2AToolDTO{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  jsonSchemaToMap(tool.Parameters),
			Version:     tool.Version,
		})
	}

	return &A2AAgentCardDTO{
		Name:         card.Name,
		Description:  card.Description,
		URL:          card.URL,
		Version:      card.Version,
		Capabilities: capabilities,
		InputSchema:  jsonSchemaToMap(card.InputSchema),
		OutputSchema: jsonSchemaToMap(card.OutputSchema),
		Tools:        tools,
		Metadata:     card.Metadata,
	}
}

// jsonSchemaToMap converts a JSONSchema pointer to map[string]any using JSON serialization.
// Returns nil if the schema is nil or if conversion fails.
func jsonSchemaToMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// WriteJSONError writes a types.Error as JSON to the response writer.
func WriteJSONError(w http.ResponseWriter, apiErr *types.Error) {
	status := apiErr.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := map[string]any{
		"success": false,
		"error": map[string]any{
			"code":    string(apiErr.Code),
			"message": apiErr.Error(),
		},
		"timestamp": time.Now().UTC(),
	}

	_ = json.NewEncoder(w).Encode(response)
}
