package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/api"
	"go.uber.org/zap"
)

// =============================================================================
// 🏥 健康检查 Handler
// =============================================================================

// HealthHandler 健康检查处理器
type HealthHandler struct {
	logger *zap.Logger
	checks []HealthCheck
	mu     sync.RWMutex
}

// HealthCheck 健康检查接口
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
}

// ServiceHealthResponse 服务级别的健康状态响应（HTTP 健康端点 DTO）。
// 注意：这是 HTTP 健康检查端点的响应类型，与 llm.HealthStatus（Provider 级别）
// 和 agent.HealthStatus（Agent 级别）是不同概念。
type ServiceHealthResponse struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version,omitempty"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
}

// CheckResult 单个检查结果
type CheckResult struct {
	Status  string `json:"status"` // "pass", "fail"
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		logger: logger,
		checks: make([]HealthCheck, 0),
	}
}

// RegisterCheck 注册健康检查
func (h *HealthHandler) RegisterCheck(check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, check)
}

// =============================================================================
// 🎯 HTTP 处理程序
// =============================================================================

// HandleHealth 处理 /health 请求（简单健康检查）
// @Summary 健康检查
// @Description 简单的健康检查端点
// @Tags 健康
// @Produce json
// @Success 200 {object} api.Response "服务正常，data 为 ServiceHealthResponse"
// @Failure 503 {object} api.Response "服务不健康"
// @Router /health [get]
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w = NewResponseWriter(w)
	WriteJSON(w, http.StatusOK, api.Response{
		Success:   true,
		Data:      ServiceHealthResponse{Status: "healthy", Timestamp: time.Now()},
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

// HandleHealthz 处理 /healthz 请求（Kubernetes 风格）
// @Summary Kubernetes 活跃度探针
// @Description Kubernetes 的活跃度探针
// @Tags 健康
// @Produce json
// @Success 200 {object} api.Response "服务处于活动状态，data 为 ServiceHealthResponse"
// @Router /healthz [get]
func (h *HealthHandler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w = NewResponseWriter(w)
	WriteJSON(w, http.StatusOK, api.Response{
		Success:   true,
		Data:      ServiceHealthResponse{Status: "healthy", Timestamp: time.Now()},
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

// HandleReady 处理 /ready 或 /readyz 请求（就绪检查）
// @Summary 准备情况检查
// @Description 检查服务是否准备好接受流量
// @Tags 健康
// @Produce json
// @Success 200 {object} ServiceHealthResponse "服务已准备就绪"
// @Failure 503 {object} ServiceHealthResponse "服务尚未准备好"
// @Router /ready [get]
func (h *HealthHandler) HandleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	h.mu.RLock()
	checks := make([]HealthCheck, len(h.checks))
	copy(checks, h.checks)
	h.mu.RUnlock()

	status := ServiceHealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Checks:    make(map[string]CheckResult),
	}

	allHealthy := true
	for _, check := range checks {
		start := time.Now()
		err := check.Check(ctx)
		latency := time.Since(start)

		result := CheckResult{
			Status:  "pass",
			Latency: latency.String(),
		}

		if err != nil {
			result.Status = "fail"
			result.Message = err.Error()
			allHealthy = false

			h.logger.Warn("health check failed",
				zap.String("check", check.Name()),
				zap.Error(err),
				zap.Duration("latency", latency),
			)
		}

		status.Checks[check.Name()] = result
	}

	if !allHealthy {
		status.Status = "unhealthy"
		WriteJSON(w, http.StatusServiceUnavailable, api.Response{
			Success:   false,
			Data:      status,
			Timestamp: time.Now(),
			RequestID: w.Header().Get("X-Request-ID"),
		})
		return
	}

	WriteJSON(w, http.StatusOK, api.Response{
		Success:   true,
		Data:      status,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

// HandleVersion 处理 /version 请求
// @Summary 版本信息
// @Description 返回版本信息
// @Tags 健康
// @Produce json
// @Success 200 {object} map[string]string "版本信息"
// @Router /version [get]
func (h *HealthHandler) HandleVersion(version, buildTime, gitCommit string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w = NewResponseWriter(w)
		WriteJSON(w, http.StatusOK, api.Response{
			Success:   true,
			Data:      map[string]string{"version": version, "build_time": buildTime, "git_commit": gitCommit},
			Timestamp: time.Now(),
			RequestID: w.Header().Get("X-Request-ID"),
		})
	}
}

// =============================================================================
// 🔧 内置健康检查实现
// =============================================================================

// DatabaseHealthCheck 数据库健康检查
type DatabaseHealthCheck struct {
	name string
	ping func(ctx context.Context) error
}

// NewDatabaseHealthCheck 创建数据库健康检查
func NewDatabaseHealthCheck(name string, ping func(ctx context.Context) error) *DatabaseHealthCheck {
	return &DatabaseHealthCheck{
		name: name,
		ping: ping,
	}
}

func (c *DatabaseHealthCheck) Name() string {
	return c.name
}

func (c *DatabaseHealthCheck) Check(ctx context.Context) error {
	return c.ping(ctx)
}

// RedisHealthCheck Redis 健康检查
type RedisHealthCheck struct {
	name string
	ping func(ctx context.Context) error
}

// NewRedisHealthCheck 创建 Redis 健康检查
func NewRedisHealthCheck(name string, ping func(ctx context.Context) error) *RedisHealthCheck {
	return &RedisHealthCheck{
		name: name,
		ping: ping,
	}
}

func (c *RedisHealthCheck) Name() string {
	return c.name
}

func (c *RedisHealthCheck) Check(ctx context.Context) error {
	return c.ping(ctx)
}

