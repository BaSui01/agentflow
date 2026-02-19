package handlers

import (
	"net/http"
	"sync"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =============================================================================
// 🤖 Agent 管理 Handler
// =============================================================================

// AgentHandler Agent 管理处理器
type AgentHandler struct {
	// TODO: 使用 agent.Registry 需要先导入 agent 包
	// 注册表 *agent.Registry
	logger *zap.Logger
	mu     sync.RWMutex
}

// AgentInfo Agent 信息
type AgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`  // TODO: 使用 agent.AgentType
	State       string `json:"state"` // TODO: 使用 agent.State
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// AgentExecuteRequest Agent 执行请求
type AgentExecuteRequest struct {
	AgentID   string            `json:"agent_id" binding:"required"`
	Content   string            `json:"content" binding:"required"`
	Context   map[string]any    `json:"context,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// AgentExecuteResponse Agent 执行响应
type AgentExecuteResponse struct {
	TraceID      string         `json:"trace_id"`
	Content      string         `json:"content"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	TokensUsed   int            `json:"tokens_used,omitempty"`
	Cost         float64        `json:"cost,omitempty"`
	Duration     string         `json:"duration"`
	FinishReason string         `json:"finish_reason,omitempty"`
}

// NewAgentHandler 创建 Agent 处理器
func NewAgentHandler(logger *zap.Logger) *AgentHandler {
	// TODO: 接受 registry 参数
	// func NewAgentHandler(registry *agent.Registry, logger *zap.Logger) *AgentHandler {
	return &AgentHandler{
		// 注册表：注册表，
		logger: logger,
	}
}

// =============================================================================
// 🎯 HTTP 处理程序
// =============================================================================

// HandleListAgents 列出所有 Agent
// @Summary 列出代理
// @Description 获取所有注册代理的列表
// @Tags 代理人
// @Produce json
// @Success 200 {object} Response{data=[]AgentInfo} “代理列表”
// @Failure 500 {object} 响应“内部错误”
// @Security API密钥认证
// @Router /v1/agents [获取]
func (h *AgentHandler) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现 agent registry 后启用
	// 代理 := h.registry.ListAgents()
	// ...

	// 暂时返回空列表
	WriteSuccess(w, []AgentInfo{})
}

// HandleGetAgent 获取单个 Agent 信息
// @Summary 获取代理
// @Description 获取有关特定代理的信息
// @Tags 代理人
// @Produce json
// @Param id 路径字符串 true“代理 ID”
// @Success 200 {object} Response{data=AgentInfo} "代理信息"
// @Failure 404 {object} 响应“未找到代理”
// @Security API密钥认证
// @Router /v1/agents/{id} [获取]
func (h *AgentHandler) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现 agent registry 后启用
	err := types.NewNotFoundError("agent not found")
	WriteError(w, err, h.logger)
}

// HandleExecuteAgent 执行 Agent
// @Summary 执行代理
// @Description 使用给定的输入执行代理
// @Tags 代理人
// @Accept json
// @Produce json
// @Param 请求主体 AgentExecuteRequest true "执行请求"
// @Success 200 {object} Response{data=AgentExecuteResponse} "执行结果"
// @Failure 400 {object} 响应“无效请求”
// @Failure 404 {object} 响应“未找到代理”
// @Failure 500 {object} 响应“执行失败”
// @Security API密钥认证
// @Router /v1/agents/执行 [帖子]
func (h *AgentHandler) HandleExecuteAgent(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现 agent registry 后启用
	err := types.NewError(types.ErrInternalError, "not implemented")
	WriteError(w, err, h.logger)
}

// HandlePlanAgent 规划 Agent 执行
// @Summary 计划代理执行
// @Description 获取代理的执行计划
// @Tags 代理人
// @Accept json
// @Produce json
// @Param 请求主体 AgentExecuteRequest true "计划请求"
// @Success 200 {object} Response{data=map[string]interface{}} "执行计划"
// @Failure 400 {object} 响应“无效请求”
// @Failure 404 {object} 响应“未找到代理”
// @Failure 500 {object} 响应“计划失败”
// @Security API密钥认证
// @Router /v1/agents/plan [帖子]
func (h *AgentHandler) HandlePlanAgent(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现 agent registry 后启用
	err := types.NewError(types.ErrInternalError, "not implemented")
	WriteError(w, err, h.logger)
}

// HandleAgentHealth 检查 Agent 健康状态
// @Summary 代理健康检查
// @Description 检查代理是否健康并准备就绪
// @Tags 代理人
// @Produce json
// @Param id 查询字符串 true“代理 ID”
// @Success 200 {object} Response{data=map[string]interface{}} “代理健康”
// @Failure 404 {object} 响应“未找到代理”
// @Failure 503 {object} 响应“代理尚未准备好”
// @Security API密钥认证
// @Router /v1/agents/health [获取]
func (h *AgentHandler) HandleAgentHealth(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现 agent registry 后启用
	err := types.NewNotFoundError("agent not found")
	WriteError(w, err, h.logger)
}

// =============================================================================
// 🔧 辅助函数
// =============================================================================

// handleAgentError 处理 Agent 错误
func (h *AgentHandler) handleAgentError(w http.ResponseWriter, err error) {
	if typedErr, ok := err.(*types.Error); ok {
		WriteError(w, typedErr, h.logger)
		return
	}

	// 未知错误，包装为内部错误
	internalErr := types.NewError(types.ErrInternalError, "agent execution failed").
		WithCause(err).
		WithRetryable(false)

	WriteError(w, internalErr, h.logger)
}
