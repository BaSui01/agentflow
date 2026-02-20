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
	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// A2AServer定义了A2A服务器操作的接口.
type A2AServer interface {
	// 注册代理在服务器上注册本地代理 。
	RegisterAgent(agent agent.Agent) error
	// Unregister Agent 从服务器中删除一个代理 。
	UnregisterAgent(agentID string) error
	// ServiHTTP 执行 http. 服务A2A请求的掌上电脑
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// Get AgentCard为注册代理人取回代理卡.
	GetAgentCard(agentID string) (*AgentCard, error)
}

// 服务器Config持有A2A服务器的配置.
type ServerConfig struct {
	// BaseURL 是此服务器可访问的基础 URL 。
	BaseURL string
	// 默认代理ID是在没有特定代理目标时使用的代理ID.
	DefaultAgentID string
	// 请求超时是处理请求的超时.
	RequestTimeout time.Duration
	// 启用 Auth 允许对收到的请求进行认证 。
	EnableAuth bool
	// AuthToken 是预期的认证符( 如果 EullAuth 是真实的) 。
	AuthToken string
	// logger 是日志实例 。
	Logger *zap.Logger
}

// 默认ServerConfig 返回带有合理默认的服务器Config 。
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 30 * time.Second,
		EnableAuth:     false,
		Logger:         zap.NewNop(),
	}
}

// HTTPServer是A2AServer使用HTTP的默认执行.
// 支持任务持续在服务重启后恢复 。
type HTTPServer struct {
	config *ServerConfig
	logger *zap.Logger

	// 代理通过身份证储存注册代理
	agents   map[string]agent.Agent
	agentsMu sync.RWMutex

	// 代理卡缓存生成代理卡
	agentCards   map[string]*AgentCard
	agentCardsMu sync.RWMutex

	// asyncTasks 存储 async 任务状态( 在记忆缓存中)
	asyncTasks   map[string]*asyncTask
	asyncTasksMu sync.RWMutex

	// 任务Store 为同步任务提供持续存储
	taskStore persistence.TaskStore

	// 从代理生成代理卡
	cardGenerator *AgentCardGenerator
}

// ayncTask 代表正在处理的同步任务。
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

// NewHTTPServer用给定的配置创建了新的HTTPServer.
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

// NewHTTPServer With TaskStore创建了新的HTTPServer,任务持续.
func NewHTTPServerWithTaskStore(config *ServerConfig, taskStore persistence.TaskStore) *HTTPServer {
	server := NewHTTPServer(config)
	server.taskStore = taskStore
	return server
}

// SetTaskStore 设置任务存储,用于持久性(依赖性注射).
func (s *HTTPServer) SetTaskStore(store persistence.TaskStore) {
	s.taskStore = store
}

// RecoverTasks在服务重启后从持续存储中恢复任务.
func (s *HTTPServer) RecoverTasks(ctx context.Context) error {
	if s.taskStore == nil {
		return nil
	}

	s.logger.Info("recovering tasks from persistent storage")

	tasks, err := s.taskStore.GetRecoverableTasks(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recoverable tasks: %w", err)
	}

	recovered := 0
	for _, persistTask := range tasks {
		// 找到此任务的代理
		s.agentsMu.RLock()
		ag, ok := s.agents[persistTask.AgentID]
		s.agentsMu.RUnlock()

		if !ok {
			s.logger.Warn("agent not found for task recovery",
				zap.String("task_id", persistTask.ID),
				zap.String("agent_id", persistTask.AgentID),
			)
			continue
		}

		// 转换为内部任务格式
		task := s.convertFromPersistTask(persistTask)

		// 添加到内存缓存
		s.asyncTasksMu.Lock()
		s.asyncTasks[task.ID] = task
		s.asyncTasksMu.Unlock()

		// 重新执行运行中的任务
		if persistTask.Status == persistence.TaskStatusRunning {
			execCtx, cancel := context.WithTimeout(ctx, s.config.RequestTimeout)
			task.cancel = cancel
			go s.executeAsyncTask(execCtx, ag, task)
			s.logger.Info("task re-execution started",
				zap.String("task_id", task.ID),
			)
		}

		recovered++
	}

	s.logger.Info("task recovery completed",
		zap.Int("recovered", recovered),
	)

	return nil
}

// 转换 ToPersistTask 将内部任务转换为持久性格式。
func (s *HTTPServer) convertToPersistTask(task *asyncTask) *persistence.AsyncTask {
	var input map[string]interface{}
	if task.Message != nil && task.Message.Payload != nil {
		if m, ok := task.Message.Payload.(map[string]interface{}); ok {
			input = m
		}
	}

	persistTask := &persistence.AsyncTask{
		ID:        task.ID,
		AgentID:   task.AgentID,
		Type:      "a2a_message",
		Input:     input,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	// 转换状态
	switch task.Status {
	case "pending":
		persistTask.Status = persistence.TaskStatusPending
	case "processing":
		persistTask.Status = persistence.TaskStatusRunning
	case "completed":
		persistTask.Status = persistence.TaskStatusCompleted
	case "failed":
		persistTask.Status = persistence.TaskStatusFailed
	default:
		persistTask.Status = persistence.TaskStatusPending
	}

	if task.Error != "" {
		persistTask.Error = task.Error
	}

	if task.Result != nil {
		persistTask.Result = task.Result
	}

	return persistTask
}

// FromPersistTask将持久性格式转换为内部任务.
func (s *HTTPServer) convertFromPersistTask(persistTask *persistence.AsyncTask) *asyncTask {
	task := &asyncTask{
		ID:        persistTask.ID,
		AgentID:   persistTask.AgentID,
		CreatedAt: persistTask.CreatedAt,
		UpdatedAt: persistTask.UpdatedAt,
	}

	// 转换状态
	switch persistTask.Status {
	case persistence.TaskStatusPending:
		task.Status = "pending"
	case persistence.TaskStatusRunning:
		task.Status = "processing"
	case persistence.TaskStatusCompleted:
		task.Status = "completed"
	case persistence.TaskStatusFailed:
		task.Status = "failed"
	default:
		task.Status = "pending"
	}

	task.Error = persistTask.Error

	if persistTask.Result != nil {
		if result, ok := persistTask.Result.(*A2AMessage); ok {
			task.Result = result
		}
	}

	// 从输入中重建信件
	if persistTask.Input != nil {
		task.Message = &A2AMessage{
			ID:      persistTask.ID,
			Payload: persistTask.Input,
		}
	}

	return task
}

// 注册代理在服务器上注册本地代理 。
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

	// 使用适配器生成和缓存代理卡
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

// 特工Adapter 适应代理。 代理ConfigProvider接口的代理服务器.
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
	// 如果执行描述方法, 请尝试从代理获取描述
	if desc, ok := a.ag.(interface{ Description() string }); ok {
		return desc.Description()
	}
	// 基于名称和类型的默认描述
	return fmt.Sprintf("%s agent of type %s", a.ag.Name(), a.ag.Type())
}

func (a *agentAdapter) Tools() []string {
	// 执行工具方法时尝试从代理获取工具
	if tools, ok := a.ag.(interface{ Tools() []string }); ok {
		return tools.Tools()
	}
	return nil
}

func (a *agentAdapter) Metadata() map[string]string {
	// 如果执行元数据方法, 尝试从代理获取元数据
	if meta, ok := a.ag.(interface{ Metadata() map[string]string }); ok {
		return meta.Metadata()
	}
	return nil
}

// Unregister Agent 从服务器中删除一个代理 。
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

// Get AgentCard为注册代理人取回代理卡.
func (s *HTTPServer) GetAgentCard(agentID string) (*AgentCard, error) {
	s.agentCardsMu.RLock()
	card, ok := s.agentCards[agentID]
	s.agentCardsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return card, nil
}

// 获得代理通过身份检索注册代理。
func (s *HTTPServer) getAgent(agentID string) (agent.Agent, error) {
	s.agentsMu.RLock()
	ag, ok := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return ag, nil
}

// 获得Default Agent 返回默认代理或第一个注册代理。
func (s *HTTPServer) getDefaultAgent() (agent.Agent, error) {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	// 首先尝试默认代理ID
	if s.config.DefaultAgentID != "" {
		if ag, ok := s.agents[s.config.DefaultAgentID]; ok {
			return ag, nil
		}
	}

	// 返回第一个可用的代理
	for _, ag := range s.agents {
		return ag, nil
	}

	return nil, ErrAgentNotFound
}

// ServiHTTP 执行 http. 服务A2A请求的掌上电脑
func (s *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 认证检查
	if s.config.EnableAuth {
		if !s.authenticate(r) {
			s.writeError(w, http.StatusUnauthorized, ErrAuthFailed)
			return
		}
	}

	// 路线请求
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

// 认证请求是否被认证 。
func (s *HTTPServer) authenticate(r *http.Request) bool {
	if !s.config.EnableAuth {
		return true
	}

	// 检查授权页眉
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}

	// 支持“ Bearer <token>” 格式
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return token == s.config.AuthToken
	}

	return auth == s.config.AuthToken
}

// /. 熟知/代理人.json
func (s *HTTPServer) handleAgentCardDiscovery(w http.ResponseWriter, r *http.Request) {
	// 从查询参数或使用默认情况下获取代理ID
	agentID := r.URL.Query().Get("agent_id")

	var card *AgentCard
	var err error

	if agentID != "" {
		card, err = s.GetAgentCard(agentID)
	} else {
		// 返回默认代理卡
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

// 手柄 GetSpecificial AgentCard 手柄 Get /a2a/ agents/{ agentID}/ card
func (s *HTTPServer) handleGetSpecificAgentCard(w http.ResponseWriter, r *http.Request) {
	// 从路径提取代理 ID: /a2a/ agents/{ agentID}/ card
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

// 同步处理 POST / a2a/ 消息( 同步)
func (s *HTTPServer) handleSyncMessage(w http.ResponseWriter, r *http.Request) {
	// 解析消息
	msg, err := s.parseMessage(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// 联系代理的路线
	ag, err := s.routeMessage(msg)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	// 以超时创建上下文
	ctx, cancel := context.WithTimeout(r.Context(), s.config.RequestTimeout)
	defer cancel()

	// 执行任务
	result, err := s.executeTask(ctx, ag, msg)
	if err != nil {
		// 返回错误消息
		errMsg := msg.CreateReply(A2AMessageTypeError, map[string]string{
			"error": err.Error(),
		})
		s.writeJSON(w, http.StatusOK, errMsg)
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// 同步处理 POST /a2a/消息/同步
func (s *HTTPServer) handleAsyncMessage(w http.ResponseWriter, r *http.Request) {
	// 解析消息
	msg, err := s.parseMessage(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}

	// 联系代理的路线
	ag, err := s.routeMessage(msg)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err)
		return
	}

	// 创建同步任务
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

	// 配置存储时坚持任务
	if s.taskStore != nil {
		persistTask := s.convertToPersistTask(task)
		if err := s.taskStore.SaveTask(r.Context(), persistTask); err != nil {
			s.logger.Error("failed to persist task",
				zap.String("task_id", taskID),
				zap.Error(err),
			)
			// 继续, 即使持久性失败 - 任务仍然会执行
		}
	}

	s.asyncTasksMu.Lock()
	s.asyncTasks[taskID] = task
	s.asyncTasksMu.Unlock()

	// 同步执行任务
	go s.executeAsyncTask(ctx, ag, task)

	// 返回任务标识
	resp := AsyncResponse{
		TaskID:  taskID,
		Status:  "accepted",
		Message: "Task accepted for processing",
	}

	s.writeJSON(w, http.StatusAccepted, resp)
}

// 手柄 Get 任务结果控件获得 /a2a/ 任务/{任务ID}/ 结果
func (s *HTTPServer) handleGetTaskResult(w http.ResponseWriter, r *http.Request) {
	// 从路径中提取任务ID: /a2a/ 任务/{任务ID}/ 结果
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
		// 任务仍在进行中
		resp := AsyncResponse{
			TaskID:  taskID,
			Status:  task.Status,
			Message: "Task is still processing",
		}
		s.writeJSON(w, http.StatusAccepted, resp)
	case "completed":
		// 返回结果
		s.writeJSON(w, http.StatusOK, task.Result)
	case "failed":
		// 返回错误
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

// 解析请求机构的 A2A 信件 。
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

// 路由Message 向合适的代理商传递消息。
func (s *HTTPServer) routeMessage(msg *A2AMessage) (agent.Agent, error) {
	// 在“ 到” 字段中找到代理
	agentID := msg.To

	// 如果“ To” 是 URL, 请从中提取代理 ID
	if strings.Contains(agentID, "/") {
		// 尝试从 URL 路径提取代理 ID
		parts := strings.Split(agentID, "/")
		for i, part := range parts {
			if part == "agents" && i+1 < len(parts) {
				agentID = parts[i+1]
				break
			}
		}
	}

	// 试着找到代理
	ag, err := s.getAgent(agentID)
	if err == nil {
		return ag, nil
	}

	// 返回默认代理
	return s.getDefaultAgent()
}

// 执行任务同步执行任务 。
func (s *HTTPServer) executeTask(ctx context.Context, ag agent.Agent, msg *A2AMessage) (*A2AMessage, error) {
	s.logger.Info("executing task",
		zap.String("agent_id", ag.ID()),
		zap.String("message_id", msg.ID),
		zap.String("message_type", string(msg.Type)),
	)

	// 将有效载荷转换为输入内容
	content, err := s.payloadToContent(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload: %w", err)
	}

	// 创建代理输入
	input := &agent.Input{
		TraceID: msg.ID,
		Content: content,
		Context: map[string]any{
			"a2a_message_id":   msg.ID,
			"a2a_message_type": string(msg.Type),
			"a2a_from":         msg.From,
		},
	}

	// 执行代理
	output, err := ag.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// 创建结果消息
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

// 执行 AsyncTask 同步执行任务 。
func (s *HTTPServer) executeAsyncTask(ctx context.Context, ag agent.Agent, task *asyncTask) {
	defer task.cancel()

	// 处理状态更新
	s.asyncTasksMu.Lock()
	task.Status = "processing"
	task.UpdatedAt = time.Now()
	s.asyncTasksMu.Unlock()

	// 更新持久性存储
	if s.taskStore != nil {
		if err := s.taskStore.UpdateStatus(ctx, task.ID, persistence.TaskStatusRunning, nil, ""); err != nil {
			s.logger.Warn("failed to update task status in store",
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
		}
	}

	// 执行任务
	result, err := s.executeTask(ctx, ag, task.Message)

	// 结果更新任务
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

	// 更新持久性存储
	if s.taskStore != nil {
		var status persistence.TaskStatus
		var errMsg string
		if err != nil {
			status = persistence.TaskStatusFailed
			errMsg = err.Error()
		} else {
			status = persistence.TaskStatusCompleted
		}
		if updateErr := s.taskStore.UpdateStatus(ctx, task.ID, status, result, errMsg); updateErr != nil {
			s.logger.Warn("failed to update task status in store",
				zap.String("task_id", task.ID),
				zap.Error(updateErr),
			)
		}
	}

	s.logger.Info("async task completed",
		zap.String("task_id", task.ID),
		zap.String("status", task.Status),
	)
}

// 有效载荷ToContent将消息有效载荷转换为字符串内容.
func (s *HTTPServer) payloadToContent(payload any) (string, error) {
	if payload == nil {
		return "", nil
	}

	switch v := payload.(type) {
	case string:
		return v, nil
	case map[string]any:
		// 尝试提取“ 内容” 字段
		if content, ok := v["content"].(string); ok {
			return content, nil
		}
		// 尝试提取“ message” 字段
		if message, ok := v["message"].(string); ok {
			return message, nil
		}
		// 尝试取出“ query” 字段
		if query, ok := v["query"].(string); ok {
			return query, nil
		}
		// 按顺序排列整个地图
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		// 尝试序列化
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// 写JSON写下JSON的回应.
func (s *HTTPServer) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to write JSON response", zap.Error(err))
	}
}

// 写入错误反应 。
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

// 清理已过期 任务删除超过指定期限的已完成或失败的任务 。
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

	// 还清理了持久性储存
	if s.taskStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		persistCount, err := s.taskStore.Cleanup(ctx, maxAge)
		if err != nil {
			s.logger.Warn("failed to cleanup persistent task store",
				zap.Error(err),
			)
		} else if persistCount > 0 {
			s.logger.Debug("cleaned up persistent tasks",
				zap.Int("count", persistCount),
			)
		}
	}

	return count
}

// StartCleanupLoop 启动背景goroutine以定期清理已过期的任务 。
func (s *HTTPServer) StartCleanupLoop(ctx context.Context, interval time.Duration, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count := s.CleanupExpiredTasks(maxAge)
				if count > 0 {
					s.logger.Debug("cleaned up expired tasks",
						zap.Int("count", count),
					)
				}
			}
		}
	}()
}

// TaskStats 返回关于任务存储的统计数据 。
func (s *HTTPServer) TaskStats(ctx context.Context) (*persistence.TaskStoreStats, error) {
	if s.taskStore == nil {
		return nil, fmt.Errorf("no task store configured")
	}
	return s.taskStore.Stats(ctx)
}

// GetTaskStatus 返回同步任务状态 。
func (s *HTTPServer) GetTaskStatus(taskID string) (string, error) {
	s.asyncTasksMu.RLock()
	task, ok := s.asyncTasks[taskID]
	s.asyncTasksMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	return task.Status, nil
}

// 取消任务取消一个同步任务 。
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

// ListAgents返回注册代理ID列表.
func (s *HTTPServer) ListAgents() []string {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// Agent Count返回注册代理的数量.
func (s *HTTPServer) AgentCount() int {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()
	return len(s.agents)
}

// 确保 HTTPServer 执行 A2AServer 接口。
var _ A2AServer = (*HTTPServer)(nil)
