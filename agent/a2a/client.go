package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// A2AClient defines the interface for A2A client operations.
type A2AClient interface {
	// Discover retrieves the AgentCard from a remote agent.
	Discover(ctx context.Context, url string) (*AgentCard, error)
	// Send sends a message synchronously and waits for a response.
	Send(ctx context.Context, msg *A2AMessage) (*A2AMessage, error)
	// SendAsync sends a message asynchronously and returns a task ID.
	SendAsync(ctx context.Context, msg *A2AMessage) (string, error)
	// GetResult retrieves the result of an async task by task ID.
	GetResult(ctx context.Context, taskID string) (*A2AMessage, error)
}

// ClientConfig holds configuration for the A2A client.
type ClientConfig struct {
	// Timeout is the default timeout for HTTP requests.
	Timeout time.Duration
	// RetryCount is the number of retries for failed requests.
	RetryCount int
	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
	// Headers are additional headers to include in requests.
	Headers map[string]string
	// AgentID is the identifier of the local agent making requests.
	AgentID string
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:    30 * time.Second,
		RetryCount: 3,
		RetryDelay: 1 * time.Second,
		Headers:    make(map[string]string),
		AgentID:    "default-agent",
	}
}

// HTTPClient is the default implementation of A2AClient using HTTP.
type HTTPClient struct {
	config     *ClientConfig
	httpClient *http.Client
	// cardCache caches discovered agent cards
	cardCache map[string]*cachedCard
	cacheMu   sync.RWMutex
	// taskRegistry tracks task ID to agent URL mappings for async operations
	taskRegistry map[string]*taskInfo
	taskMu       sync.RWMutex
}

type cachedCard struct {
	card      *AgentCard
	expiresAt time.Time
}

// taskInfo stores information about an async task.
type taskInfo struct {
	agentURL  string
	messageID string
	createdAt time.Time
}

// NewHTTPClient creates a new HTTPClient with the given configuration.
func NewHTTPClient(config *ClientConfig) *HTTPClient {
	if config == nil {
		config = DefaultClientConfig()
	}

	return &HTTPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		cardCache:    make(map[string]*cachedCard),
		taskRegistry: make(map[string]*taskInfo),
	}
}

// Discover retrieves the AgentCard from a remote agent at the given URL.
// The URL should be the base URL of the agent (e.g., "https://agent.example.com").
// The agent card is expected to be available at "/.well-known/agent.json".
func (c *HTTPClient) Discover(ctx context.Context, url string) (*AgentCard, error) {
	if url == "" {
		return nil, fmt.Errorf("%w: empty url", ErrRemoteUnavailable)
	}

	// Check cache first
	c.cacheMu.RLock()
	if cached, ok := c.cardCache[url]; ok && time.Now().Before(cached.expiresAt) {
		c.cacheMu.RUnlock()
		return cached.card, nil
	}
	c.cacheMu.RUnlock()

	// Build the discovery URL
	discoveryURL := url + "/.well-known/agent.json"

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// Execute request with retry
	var resp *http.Response
	var lastErr error
	for i := 0; i <= c.config.RetryCount; i++ {
		resp, lastErr = c.httpClient.Do(req)
		if lastErr == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if i < c.config.RetryCount {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay):
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteUnavailable, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status code %d", ErrRemoteUnavailable, resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}

	// Validate the card
	if err := card.Validate(); err != nil {
		return nil, err
	}

	// Cache the card (5 minutes TTL)
	c.cacheMu.Lock()
	c.cardCache[url] = &cachedCard{
		card:      &card,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	c.cacheMu.Unlock()

	return &card, nil
}

// Send sends a message synchronously and waits for a response.
// The message is sent to the agent specified in the message's To field.
func (c *HTTPClient) Send(ctx context.Context, msg *A2AMessage) (*A2AMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("%w: nil message", ErrInvalidMessage)
	}

	// Validate message
	if err := msg.Validate(); err != nil {
		return nil, err
	}

	// Discover the target agent to get its URL
	card, err := c.Discover(ctx, msg.To)
	if err != nil {
		// If discovery fails, assume msg.To is the URL
		card = &AgentCard{URL: msg.To}
	}

	// Build the message endpoint URL
	messageURL := card.URL + "/a2a/messages"

	// Serialize message
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messageURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// Execute request with retry
	var resp *http.Response
	var lastErr error
	for i := 0; i <= c.config.RetryCount; i++ {
		resp, lastErr = c.httpClient.Do(req)
		if lastErr == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted) {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if i < c.config.RetryCount {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay):
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteUnavailable, lastErr)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response A2AMessage
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}

	return &response, nil
}

// SendAsync sends a message asynchronously and returns a task ID.
// The caller can use GetResult to poll for the result.
func (c *HTTPClient) SendAsync(ctx context.Context, msg *A2AMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("%w: nil message", ErrInvalidMessage)
	}

	// Validate message
	if err := msg.Validate(); err != nil {
		return "", err
	}

	// Discover the target agent to get its URL
	agentURL := msg.To
	card, err := c.Discover(ctx, msg.To)
	if err != nil {
		// If discovery fails, assume msg.To is the URL
		card = &AgentCard{URL: msg.To}
	} else {
		agentURL = card.URL
	}

	// Build the async message endpoint URL
	asyncURL := card.URL + "/a2a/messages/async"

	// Serialize message
	body, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asyncURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRemoteUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// Parse response to get task ID
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var asyncResp AsyncResponse
	if err := json.Unmarshal(respBody, &asyncResp); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}

	if asyncResp.TaskID == "" {
		return "", fmt.Errorf("%w: missing task_id in response", ErrInvalidMessage)
	}

	// Register the task for later retrieval
	c.taskMu.Lock()
	c.taskRegistry[asyncResp.TaskID] = &taskInfo{
		agentURL:  agentURL,
		messageID: msg.ID,
		createdAt: time.Now(),
	}
	c.taskMu.Unlock()

	return asyncResp.TaskID, nil
}

// GetResult retrieves the result of an async task by task ID.
// Returns ErrTaskNotReady if the task is still processing.
// Returns ErrTaskNotFound if the task ID is not registered.
func (c *HTTPClient) GetResult(ctx context.Context, taskID string) (*A2AMessage, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrInvalidMessage)
	}

	// Look up the task in the registry
	c.taskMu.RLock()
	info, ok := c.taskRegistry[taskID]
	c.taskMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: task %s not found in registry", ErrTaskNotFound, taskID)
	}

	// Use the stored agent URL to retrieve the result
	return c.GetResultFromAgent(ctx, info.agentURL, taskID)
}

// GetResultFromAgent retrieves the result of an async task from a specific agent.
func (c *HTTPClient) GetResultFromAgent(ctx context.Context, agentURL, taskID string) (*A2AMessage, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrInvalidMessage)
	}
	if agentURL == "" {
		return nil, fmt.Errorf("%w: empty agent_url", ErrRemoteUnavailable)
	}

	// Build the result endpoint URL
	resultURL := fmt.Sprintf("%s/a2a/tasks/%s/result", agentURL, taskID)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteUnavailable, err)
	}
	defer resp.Body.Close()

	// Handle specific status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Task completed, parse result
	case http.StatusAccepted:
		// Task still processing
		return nil, ErrTaskNotReady
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: task %s", ErrAgentNotFound, taskID)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result A2AMessage
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}

	return &result, nil
}

// AsyncResponse represents the response from an async message submission.
type AsyncResponse struct {
	TaskID        string `json:"task_id"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
	EstimatedTime int    `json:"estimated_time,omitempty"` // seconds
}

// ClearCache clears the agent card cache.
func (c *HTTPClient) ClearCache() {
	c.cacheMu.Lock()
	c.cardCache = make(map[string]*cachedCard)
	c.cacheMu.Unlock()
}

// ClearTaskRegistry clears the task registry.
func (c *HTTPClient) ClearTaskRegistry() {
	c.taskMu.Lock()
	c.taskRegistry = make(map[string]*taskInfo)
	c.taskMu.Unlock()
}

// RegisterTask manually registers a task ID with an agent URL.
// This is useful when the task was created outside of SendAsync.
func (c *HTTPClient) RegisterTask(taskID, agentURL string) {
	c.taskMu.Lock()
	c.taskRegistry[taskID] = &taskInfo{
		agentURL:  agentURL,
		createdAt: time.Now(),
	}
	c.taskMu.Unlock()
}

// UnregisterTask removes a task from the registry.
func (c *HTTPClient) UnregisterTask(taskID string) {
	c.taskMu.Lock()
	delete(c.taskRegistry, taskID)
	c.taskMu.Unlock()
}

// CleanupExpiredTasks removes tasks older than the specified duration.
func (c *HTTPClient) CleanupExpiredTasks(maxAge time.Duration) int {
	c.taskMu.Lock()
	defer c.taskMu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	count := 0
	for taskID, info := range c.taskRegistry {
		if info.createdAt.Before(cutoff) {
			delete(c.taskRegistry, taskID)
			count++
		}
	}
	return count
}

// SetHeader sets a custom header for all requests.
func (c *HTTPClient) SetHeader(key, value string) {
	c.config.Headers[key] = value
}

// SetTimeout sets the HTTP client timeout.
func (c *HTTPClient) SetTimeout(timeout time.Duration) {
	c.config.Timeout = timeout
	c.httpClient.Timeout = timeout
}

// Ensure HTTPClient implements A2AClient interface.
var _ A2AClient = (*HTTPClient)(nil)
