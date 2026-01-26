package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// A2AServer defines the interface for A2A server operations.
type A2AServer interface {
	// RegisterAgent registers a local agent with the server.
	RegisterAgent(agent agent.Agent) error
	// UnregisterAgent removes an agent from the server.
	UnregisterAgent(agentID string) error
	// ServeHTTP implements http.Handler for serving A2A requests.
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// GetAgentCard retrieves the agent card for a registered agent.
	GetAgentCard(agentID string) (*AgentCard, error)
}

// ServerConfig holds configuration for the A2A server.
type ServerConfig struct {
	// BaseURL is the base URL where this server is accessible.
	BaseURL string
	// DefaultAgentID is the agent ID to use when no specific agent is targeted.
	DefaultAgentID string
	// RequestTimeout is the timeout for processing requests.
	RequestTimeout time.Duration
	// EnableAuth enables authentication for incoming requests.
	EnableAuth bool
	// AuthToken is the expected authentication token (if EnableAuth is true).
	AuthToken string
	// Logger is the logger instance.
	Logger *zap.Logger
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 30 * time.Second,
		EnableAuth:     false,
		Logger:         zap.NewNop(),
	}
}

// HTTPServer is the default implementation of A2AServer using HTTP.
type HTTPServer struct {
	config *ServerConfig
	logger *zap.Logger

	// agents stores registered agents by ID
	agents   map[string]agent.Agent
	agentsMu sync.RWMutex

	// agentCards caches generated agent cards
	agentCards   map[string]*AgentCard
	agentCardsMu sync.RWMutex

	// asyncTasks stores async task state
	asyncTasks   map[string]*asyncTask
	asyncTasksMu sync.RWMutex

	// cardGenerator generates agent cards from agents
	cardGenerator *AgentCardGenerator
}

// asyncTask represents an async task being processed.
type asyncTask struct {
	ID        string      `json:"id"`
	AgentID   string      `json:"agent_id"`
	Message   *A2AMessage `json:"message"`
	Status    string      `json:"status"` // pending, processing, completed, failed
	Result    *A2AMessage `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	cancel    context.CancelFunc
}

// NewHTTPServer creates a new HTTPServer with the given configuration.
func NewHTTPServer(config *ServerConfig) *HTTPServer {
	if config == nil {
		config = DefaultServerConfig()
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &HTTPServer{
		config:        config,
		logger:        config.Logger,
		agents:        make(map[string]agent.Agent),
		agentCards:    make(map[string]*AgentCard),
		asyncTasks:    make(map[string]*asyncTask),
		cardGenerator: NewAgentCardGenerator(),
	}
}

// RegisterAgent registers a local agent with the server.
func (s *HTTPServer) RegisterAgent(ag agent.Agent) error {
	if ag == nil {
		return fmt.Errorf("%w: nil agent", ErrInvalidMessage)
	}

	agentID := ag.ID()
	if agentID == "" {
		return fmt.Errorf("%w: agent has empty ID", ErrInvalidMessage)
	}

	s.agentsMu.Lock()
	s.agents[agentID] = ag
	s.agentsMu.Unlock()

	// Generate and cache agent card using adapter
	adapter := newAgentAdapter(ag)
	card := s.cardGenerator.Generate(adapter, s.config.BaseURL)
	s.agentCardsMu.Lock()
	s.agentCards[agentID] = card
	s.agentCardsMu.Unlock()

	s.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.String("agent_name", ag.Name()),
	)

	return nil
}

// agentAdapter adapts agent.Agent to AgentConfigProvider interface.
type agentAdapter struct {
	ag agent.Agent
}

func newAgentAdapter(ag agent.Agent) *agentAdapter {
	return &agentAdapter{ag: ag}
}

func (a *agentAdapter) ID() string {
	return a.ag.ID()
}

func (a *agentAdapter) Name() string {
	return a.ag.Name()
}

func (a *agentAdapter) Type() AgentType {
	return AgentType(a.ag.Type())
}

func (a *agentAdapter) Description() string {
	// Try to get description from agent if it implements a Description method
	if desc, ok := a.ag.(interface{ Description() string }); ok {
		return desc.Description()
	}
	// Default description based on name and type
	return fmt.Sprintf("%s agent of type %s", a.ag.Name(), a.ag.Type())
}

func (a *agentAdapter) Tools() []string {
	// Try to get tools from agent if it implements a Tools method
	if tools, ok := a.ag.(interface{ Tools() []string }); ok {
		return tools.Tools()
	}
	return nil
}

func (a *agentAdapter) Metadata() map[string]string {
	// Try to get metadata from agent if it implements a Metadata method
	if meta, ok := a.ag.(interface{ Metadata() map[string]string }); ok {
		return meta.Metadata()
	}
	return nil
}

// UnregisterAgent removes an agent from the server.
func (s *HTTPServer) UnregisterAgent(agentID string) error {
	s.agentsMu.Lock()
	delete(s.agents, agentID)
	s.agentsMu.Unlock()

	s.agentCardsMu.Lock()
	delete(s.agentCards, agentID)
	s.agentCardsMu.Unlock()

	s.logger.Info("agent unregistered", zap.String("agent_id", agentID))
	return nil
}

// GetAgentCard retrieves the agent card for a registered agent.
func (s *HTTPServer) GetAgentCard(agentID string) (*AgentCard, error) {
	s.agentCardsMu.RLock()
	card, ok := s.agentCards[agentID]
	s.agentCardsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return card, nil
}

// getAgent retrieves a registered agent by ID.
func (s *HTTPServer) getAgent(agentID string) (agent.Agent, error) {
	s.agentsMu.RLock()
	ag, ok := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return ag, nil
}

// getDefaultAgent returns the default agent or the first registered agent.
func (s *HTTPServer) getDefaultAgent() (agent.Agent, error) {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	// Try default agent ID first
	if s.config.DefaultAgentID != "" {
		if ag, ok := s.agents[s.config.DefaultAgentID]; ok {
			return ag, nil
		}
	}

	// Return first available agent
	for _, ag := range s.agents {
		return ag, nil
	}

	return nil, ErrAgentNotFound
}

// ServeHTTP implements http.Handler for serving A2A requests.
func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authentication check
	if s.config.EnableAuth {
		if !s.authenticate(r) {
			s.writeError(w, http.StatusUnauthorized, ErrAuthFailed)
			return
		}
	}

	// Route requests
	path := r.URL.Path
	method := r.Method

	switch {
	case path == "/.well-known/agent.json" && method == http.MethodGet:
		s.handleAgentCardDiscovery(w, r)
	case path == "/a2a/messages" && method == http.MethodPost:
		s.handleSyncMessage(w, r)
	case path == "/a2a/messages/async" && method == http.MethodPost:
		s.handleAsyncMessage(w, r)
	case strings.HasPrefix(path, "/a2a/tasks/") && strings.HasSuffix(path, "/result") && method == http.MethodGet:
		s.handleGetTaskResult(w, r)
	case strings.HasPrefix(path, "/a2a/agents/") && strings.HasSuffix(path, "/card") && method == http.MethodGet:
		s.handleGetSpecificAgentCard(w, r)
	default:
		s.writeError(w, http.StatusNotFound, fmt.Errorf("endpoint not found: %s %s", method, path))
	}
}

// authenticate checks if the request is authenticated.
func (s *HTTPServer) authenticate(r *http.Request) bool {
	if !s.config.EnableAuth {
		return true
	}

	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	// Support "Bearer <token>" format
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return token == s.config.AuthToken
	}

	return auth == s.config.AuthToken
}

// handleAgentCardDiscovery handles GET /.well-known/agent.json
func (s *HTTPServer) handleAgentCardDiscovery(w http.ResponseWriter, r *http.Request) {
	// Get agent ID from query parameter or use default
	agentID := r.URL.Query().Get("agent_id")

	var card *AgentCard
	var err error

	if agentID != "" {
		card, err = s.GetAgentCard(agentID)
	} else {
		// Return default agent's card
		ag, agErr := s.getDefaultAgent()
		if agErr != nil {
			s.writeError(w, http.StatusNotFound, agErr)
			return
		}
		card, err = s.GetAgentCard(ag.ID())
	}

	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	s.writeJSON(w, http.StatusOK, card)
}

// handleGetSpecificAgentCard handles GET /a2a/agents/{agentID}/card
func (s *HTTPServer) handleGetSpecificAgentCard(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /a2a/agents/{agentID}/card
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/a2a/agents/")
	path = strings.TrimSuffix(path, "/card")
	agentID := path

	if agentID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("missing agent_id"))
		return
	}

	card, err := s.GetAgentCard(agentID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	s.writeJSON(w, http.StatusOK, card)
}

// handleSyncMessage handles POST /a2a/messages (synchronous)
func (s *HTTPServer) handleSyncMessage(w http.ResponseWriter, r *http.Request) {
	// Parse message
	msg, err := s.parseMessage(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// Route to agent
	ag, err := s.routeMessage(msg)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.config.RequestTimeout)
	defer cancel()

	// Execute task
	result, err := s.executeTask(ctx, ag, msg)
	if err != nil {
		// Return error message
		errMsg := msg.CreateReply(A2AMessageTypeError, map[string]string{
			"error": err.Error(),
		})
		s.writeJSON(w, http.StatusOK, errMsg)
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// handleAsyncMessage handles POST /a2a/messages/async
func (s *HTTPServer) handleAsyncMessage(w http.ResponseWriter, r *http.Request) {
	// Parse message
	msg, err := s.parseMessage(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// Route to agent
	ag, err := s.routeMessage(msg)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	// Create async task
	taskID := uuid.New().String()
	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)

	task := &asyncTask{
		ID:        taskID,
		AgentID:   ag.ID(),
		Message:   msg,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		cancel:    cancel,
	}

	s.asyncTasksMu.Lock()
	s.asyncTasks[taskID] = task
	s.asyncTasksMu.Unlock()

	// Execute task asynchronously
	go s.executeAsyncTask(ctx, ag, task)

	// Return task ID
	resp := AsyncResponse{
		TaskID:  taskID,
		Status:  "accepted",
		Message: "Task accepted for processing",
	}

	s.writeJSON(w, http.StatusAccepted, resp)
}

// handleGetTaskResult handles GET /a2a/tasks/{taskID}/result
func (s *HTTPServer) handleGetTaskResult(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path: /a2a/tasks/{taskID}/result
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/a2a/tasks/")
	path = strings.TrimSuffix(path, "/result")
	taskID := path

	if taskID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("missing task_id"))
		return
	}

	s.asyncTasksMu.RLock()
	task, ok := s.asyncTasks[taskID]
	s.asyncTasksMu.RUnlock()

	if !ok {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID))
		return
	}

	switch task.Status {
	case "pending", "processing":
		// Task still in progress
		resp := AsyncResponse{
			TaskID:  taskID,
			Status:  task.Status,
			Message: "Task is still processing",
		}
		s.writeJSON(w, http.StatusAccepted, resp)
	case "completed":
		// Return result
		s.writeJSON(w, http.StatusOK, task.Result)
	case "failed":
		// Return error
		errMsg := &A2AMessage{
			ID:        uuid.New().String(),
			Type:      A2AMessageTypeError,
			From:      task.AgentID,
			To:        task.Message.From,
			Payload:   map[string]string{"error": task.Error},
			Timestamp: time.Now().UTC(),
			ReplyTo:   task.Message.ID,
		}
		s.writeJSON(w, http.StatusOK, errMsg)
	default:
		s.writeError(w, http.StatusInternalServerError, fmt.Errorf("unknown task status: %s", task.Status))
	}
}

// parseMessage parses an A2A message from the request body.
func (s *HTTPServer) parseMessage(r *http.Request) (*A2AMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	msg, err := ParseA2AMessage(body)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// routeMessage routes a message to the appropriate agent.
func (s *HTTPServer) routeMessage(msg *A2AMessage) (agent.Agent, error) {
	// Try to find agent by the "To" field
	agentID := msg.To

	// If "To" is a URL, extract agent ID from it
	if strings.Contains(agentID, "/") {
		// Try to extract agent ID from URL path
		parts := strings.Split(agentID, "/")
		for i, part := range parts {
			if part == "agents" && i+1 < len(parts) {
				agentID = parts[i+1]
				break
			}
		}
	}

	// Try to find the agent
	ag, err := s.getAgent(agentID)
	if err == nil {
		return ag, nil
	}

	// Fall back to default agent
	return s.getDefaultAgent()
}

// executeTask executes a task synchronously.
func (s *HTTPServer) executeTask(ctx context.Context, ag agent.Agent, msg *A2AMessage) (*A2AMessage, error) {
	s.logger.Info("executing task",
		zap.String("agent_id", ag.ID()),
		zap.String("message_id", msg.ID),
		zap.String("message_type", string(msg.Type)),
	)

	// Convert payload to input content
	content, err := s.payloadToContent(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload: %w", err)
	}

	// Create agent input
	input := &agent.Input{
		TraceID: msg.ID,
		Content: content,
		Context: map[string]any{
			"a2a_message_id":   msg.ID,
			"a2a_message_type": string(msg.Type),
			"a2a_from":         msg.From,
		},
	}

	// Execute agent
	output, err := ag.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// Create result message
	result := msg.CreateReply(A2AMessageTypeResult, map[string]any{
		"content":       output.Content,
		"tokens_used":   output.TokensUsed,
		"duration_ms":   output.Duration.Milliseconds(),
		"finish_reason": output.FinishReason,
	})

	s.logger.Info("task completed",
		zap.String("agent_id", ag.ID()),
		zap.String("message_id", msg.ID),
		zap.Duration("duration", output.Duration),
	)

	return result, nil
}

// executeAsyncTask executes a task asynchronously.
func (s *HTTPServer) executeAsyncTask(ctx context.Context, ag agent.Agent, task *asyncTask) {
	defer task.cancel()

	// Update status to processing
	s.asyncTasksMu.Lock()
	task.Status = "processing"
	task.UpdatedAt = time.Now()
	s.asyncTasksMu.Unlock()

	// Execute task
	result, err := s.executeTask(ctx, ag, task.Message)

	// Update task with result
	s.asyncTasksMu.Lock()
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
	} else {
		task.Status = "completed"
		task.Result = result
	}
	task.UpdatedAt = time.Now()
	s.asyncTasksMu.Unlock()

	s.logger.Info("async task completed",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
	)
}

// payloadToContent converts a message payload to string content.
func (s *HTTPServer) payloadToContent(payload any) (string, error) {
	if payload == nil {
		return "", nil
	}

	switch v := payload.(type) {
	case string:
		return v, nil
	case map[string]any:
		// Try to extract "content" field
		if content, ok := v["content"].(string); ok {
			return content, nil
		}
		// Try to extract "message" field
		if message, ok := v["message"].(string); ok {
			return message, nil
		}
		// Try to extract "query" field
		if query, ok := v["query"].(string); ok {
			return query, nil
		}
		// Serialize the whole map
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		// Try to serialize
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// writeJSON writes a JSON response.
func (s *HTTPServer) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to write JSON response", zap.Error(err))
	}
}

// writeError writes an error response.
func (s *HTTPServer) writeError(w http.ResponseWriter, status int, err error) {
	s.logger.Warn("request error",
		zap.Int("status", status),
		zap.Error(err),
	)

	resp := map[string]string{
		"error": err.Error(),
	}

	s.writeJSON(w, status, resp)
}

// CleanupExpiredTasks removes completed or failed tasks older than the specified duration.
func (s *HTTPServer) CleanupExpiredTasks(maxAge time.Duration) int {
	s.asyncTasksMu.Lock()
	defer s.asyncTasksMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	count := 0

	for taskID, task := range s.asyncTasks {
		if task.Status == "completed" || task.Status == "failed" {
			if task.UpdatedAt.Before(cutoff) {
				delete(s.asyncTasks, taskID)
				count++
			}
		}
	}

	return count
}

// GetTaskStatus returns the status of an async task.
func (s *HTTPServer) GetTaskStatus(taskID string) (string, error) {
	s.asyncTasksMu.RLock()
	task, ok := s.asyncTasks[taskID]
	s.asyncTasksMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	return task.Status, nil
}

// CancelTask cancels an async task.
func (s *HTTPServer) CancelTask(taskID string) error {
	s.asyncTasksMu.Lock()
	task, ok := s.asyncTasks[taskID]
	if !ok {
		s.asyncTasksMu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	if task.Status == "pending" || task.Status == "processing" {
		task.cancel()
		task.Status = "failed"
		task.Error = "task cancelled"
		task.UpdatedAt = time.Now()
	}
	s.asyncTasksMu.Unlock()

	return nil
}

// ListAgents returns a list of registered agent IDs.
func (s *HTTPServer) ListAgents() []string {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// AgentCount returns the number of registered agents.
func (s *HTTPServer) AgentCount() int {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()
	return len(s.agents)
}

// Ensure HTTPServer implements A2AServer interface.
var _ A2AServer = (*HTTPServer)(nil)
