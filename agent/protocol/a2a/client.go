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

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// A2AClient定义了A2A客户端操作的接口.
type A2AClient interface {
	// 发现从远程特工处取回特工卡
	Discover(ctx context.Context, url string) (*AgentCard, error)
	// 发送消息同步并等待回复.
	Send(ctx context.Context, msg *A2AMessage) (*A2AMessage, error)
	// SendAsync 同步发送消息并返回任务ID.
	SendAsync(ctx context.Context, msg *A2AMessage) (string, error)
	// GetResult通过任务ID检索一个同步任务的结果.
	GetResult(ctx context.Context, taskID string) (*A2AMessage, error)
}

// 客户端Config为A2A客户端持有配置.
type ClientConfig struct {
	// 超时是HTTP请求的默认超时.
	Timeout time.Duration
	// RetryCount 是失败请求的重试次数 。
	RetryCount int
	// RetryDelay是重试之间的延迟.
	RetryDelay time.Duration
	// 信头是请求中要包含的额外信头 。
	Headers map[string]string
	// AgentID 是本地代理提出请求的标识符 。
	AgentID string
}

// 默认 ClientConfig 返回有合理默认的客户端Config 。
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:    30 * time.Second,
		RetryCount: 3,
		RetryDelay: 1 * time.Second,
		Headers:    make(map[string]string),
		AgentID:    "default-agent",
	}
}

// HTTPClient是A2AClient使用HTTP的默认执行.
type HTTPClient struct {
	config     *ClientConfig
	httpClient *http.Client
	// 已发现代理卡缓存
	cardCache map[string]*cachedCard
	cacheMu   sync.RWMutex
	// 任务 Registry 跟踪任务ID 代理 URL 映射用于合成操作
	taskRegistry map[string]*taskInfo
	taskMu       sync.RWMutex
}

type cachedCard struct {
	card      *AgentCard
	expiresAt time.Time
}

// 任务 信息存储关于一个同步任务的信息 。
type taskInfo struct {
	agentURL  string
	messageID string
	createdAt time.Time
}

// NewHTTPClient以给定的配置创建了新的HTTPClient.
func NewHTTPClient(config *ClientConfig) *HTTPClient {
	if config == nil {
		config = DefaultClientConfig()
	}

	return &HTTPClient{
		config: config,
		httpClient: tlsutil.SecureHTTPClient(config.Timeout),
		cardCache:    make(map[string]*cachedCard),
		taskRegistry: make(map[string]*taskInfo),
	}
}

// 发现从给定的 URL 的远程代理取回 AgentCard 。
// URL应该是代理商的基础URL(例如"https://agent.example.com").
// 代理卡预计在"/. well-known/agent.json"提供.
func (c *HTTPClient) Discover(ctx context.Context, url string) (*AgentCard, error) {
	if url == "" {
		return nil, fmt.Errorf("%w: empty url", ErrRemoteUnavailable)
	}

	// 先检查缓存
	c.cacheMu.RLock()
	if cached, ok := c.cardCache[url]; ok && time.Now().Before(cached.expiresAt) {
		c.cacheMu.RUnlock()
		return cached.card, nil
	}
	c.cacheMu.RUnlock()

	// 构建发现 URL
	discoveryURL := url + "/.well-known/agent.json"

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加信头
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 通过重试执行请求
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

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}

	// 验证卡片
	if err := card.Validate(); err != nil {
		return nil, err
	}

	// 缓存卡( 5分 TTL )
	c.cacheMu.Lock()
	c.cardCache[url] = &cachedCard{
		card:      &card,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	c.cacheMu.Unlock()

	return &card, nil
}

// 发送消息同步并等待回复.
// 消息发送到消息"to field"中指定的代理.
func (c *HTTPClient) Send(ctx context.Context, msg *A2AMessage) (*A2AMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("%w: nil message", ErrInvalidMessage)
	}

	// 验证信件
	if err := msg.Validate(); err != nil {
		return nil, err
	}

	// 发现目标代理以获取其 URL
	card, err := c.Discover(ctx, msg.To)
	if err != nil {
		// 如果发现失败,假设msg. 为 URL
		card = &AgentCard{URL: msg.To}
	}

	// 构建信件端点 URL
	messageURL := card.URL + "/a2a/messages"

	// 序列化信件
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messageURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加信头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 通过重试执行请求
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

	// 处理错误回复
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// 解析响应
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

// SendAsync 同步发送消息并返回任务ID.
// 呼叫者可以使用GetResult对结果进行投票.
func (c *HTTPClient) SendAsync(ctx context.Context, msg *A2AMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("%w: nil message", ErrInvalidMessage)
	}

	// 验证信件
	if err := msg.Validate(); err != nil {
		return "", err
	}

	// 发现目标代理以获取其 URL
	agentURL := msg.To
	card, err := c.Discover(ctx, msg.To)
	if err != nil {
		// 如果发现失败,假设msg. 为 URL
		card = &AgentCard{URL: msg.To}
	} else {
		agentURL = card.URL
	}

	// 构建消息端点 URL
	asyncURL := card.URL + "/a2a/messages/async"

	// 序列化信件
	body, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to serialize message: %w", err)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asyncURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 添加信头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 执行请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRemoteUnavailable, err)
	}
	defer resp.Body.Close()

	// 处理错误回复
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// 分析获取任务ID的响应
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

	// 注册任务供以后检索
	c.taskMu.Lock()
	c.taskRegistry[asyncResp.TaskID] = &taskInfo{
		agentURL:  agentURL,
		messageID: msg.ID,
		createdAt: time.Now(),
	}
	c.taskMu.Unlock()

	return asyncResp.TaskID, nil
}

// GetResult通过任务ID检索一个同步任务的结果.
// 如果任务仍在处理中, 返回 ErrTask NotReady 。
// 如果任务ID没有注册, 返回 ErrTask NotFound 。
func (c *HTTPClient) GetResult(ctx context.Context, taskID string) (*A2AMessage, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrInvalidMessage)
	}

	// 在登记册中查找任务
	c.taskMu.RLock()
	info, ok := c.taskRegistry[taskID]
	c.taskMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: task %s not found in registry", ErrTaskNotFound, taskID)
	}

	// 使用存储代理 URL 获取结果
	return c.GetResultFromAgent(ctx, info.agentURL, taskID)
}

// GetResultFrom Agent从特定代理中获取了某项协同任务的结果.
func (c *HTTPClient) GetResultFromAgent(ctx context.Context, agentURL, taskID string) (*A2AMessage, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: empty task_id", ErrInvalidMessage)
	}
	if agentURL == "" {
		return nil, fmt.Errorf("%w: empty agent_url", ErrRemoteUnavailable)
	}

	// 构建结果终点 URL
	resultURL := fmt.Sprintf("%s/a2a/tasks/%s/result", agentURL, taskID)

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加信头
	req.Header.Set("Accept", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 执行请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteUnavailable, err)
	}
	defer resp.Body.Close()

	// 处理特定状态代码
	switch resp.StatusCode {
	case http.StatusOK:
		// 任务完成, 分析结果
	case http.StatusAccepted:
		// 任务仍在处理
		return nil, ErrTaskNotReady
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: task %s", ErrAgentNotFound, taskID)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrRemoteUnavailable, resp.StatusCode, string(respBody))
	}

	// 解析响应
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

// AsyncResponse代表了从一个Async消息提交中获取的响应.
type AsyncResponse struct {
	TaskID        string `json:"task_id"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
	EstimatedTime int    `json:"estimated_time,omitempty"` // seconds
}

// ClearCache 清除代理卡缓存 。
func (c *HTTPClient) ClearCache() {
	c.cacheMu.Lock()
	c.cardCache = make(map[string]*cachedCard)
	c.cacheMu.Unlock()
}

// ClearTaskRegistry 清除任务注册 。
func (c *HTTPClient) ClearTaskRegistry() {
	c.taskMu.Lock()
	c.taskRegistry = make(map[string]*taskInfo)
	c.taskMu.Unlock()
}

// 注册任务用代理 URL 手动注册任务ID 。
// 当任务在 SendAsync 之外创建时, 这一点是有用的 。
func (c *HTTPClient) RegisterTask(taskID, agentURL string) {
	c.taskMu.Lock()
	c.taskRegistry[taskID] = &taskInfo{
		agentURL:  agentURL,
		createdAt: time.Now(),
	}
	c.taskMu.Unlock()
}

// 未注册的任务从登记簿中删除 。
func (c *HTTPClient) UnregisterTask(taskID string) {
	c.taskMu.Lock()
	delete(c.taskRegistry, taskID)
	c.taskMu.Unlock()
}

// 清理已过期 任务会删除比指定时间长的任务 。
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

// Setheader 为所有请求设置自定义标题 。
func (c *HTTPClient) SetHeader(key, value string) {
	c.config.Headers[key] = value
}

// SetTimeout 设置 HTTP 客户端超时.
func (c *HTTPClient) SetTimeout(timeout time.Duration) {
	c.config.Timeout = timeout
	c.httpClient.Timeout = timeout
}

// 确保 HTTPClient 执行 A2AClient 接口.
var _ A2AClient = (*HTTPClient)(nil)
