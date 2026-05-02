package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

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
		return secureTokenEqual(token, s.config.AuthToken)
	}

	return secureTokenEqual(auth, s.config.AuthToken)
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
	msg, err := s.parseMessage(w, r)
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
	msg, err := s.parseMessage(w, r)
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
		Status:    asyncTaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		cancel:    cancel,
	}

	// 持久化任务
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
	case asyncTaskStatusPending, asyncTaskStatusProcessing:
		// 任务仍在进行中
		resp := AsyncResponse{
			TaskID:  taskID,
			Status:  task.Status,
			Message: "Task is still processing",
		}
		s.writeJSON(w, http.StatusAccepted, resp)
	case asyncTaskStatusCompleted:
		// 返回结果
		s.writeJSON(w, http.StatusOK, task.Result)
	case asyncTaskStatusFailed:
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
func (s *HTTPServer) parseMessage(w http.ResponseWriter, r *http.Request) (*A2AMessage, error) {
	if r.ContentLength > maxA2ARequestBodyBytes {
		return nil, fmt.Errorf("request body too large: max %d bytes", maxA2ARequestBodyBytes)
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxA2ARequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("request body too large: max %d bytes", maxA2ARequestBodyBytes)
		}
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var msg A2AMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, ErrInvalidMessage
	}
	if err := validateIncomingMessage(&msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// 路由Message 向合适的代理商传递消息。
func (s *HTTPServer) routeMessage(msg *A2AMessage) (Agent, error) {
	// 在“ 到” 字段中找到代理
	agentID := strings.TrimSpace(msg.To)
	if agentID == "" {
		return s.getDefaultAgent()
	}

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

	return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
}

