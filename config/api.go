// config 包的 HTTP 配置管理 API。
//
// 提供配置查询、更新、热重载触发与变更历史查询能力。
//
// 安全策略：
// 1) 配置更新请求启用严格 JSON 校验（含未知字段与尾随数据检测）。
// 2) 配置 API 中间件启用独立限流，降低暴力枚举风险。
// 3) 内部通信建议在部署层启用 TLS/mTLS。
package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/api"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const (
	// maxConfigUpdateBodyBytes limits /api/v1/config update payload size.
	maxConfigUpdateBodyBytes int64 = 1 << 20 // 1 MiB
)

// --- API 类型定义 ---

// ConfigAPIHandler 处理配置 API 请求
type ConfigAPIHandler struct {
	manager       *HotReloadManager
	allowedOrigin string
	logger        *zap.Logger // X-004: 审计日志
}

type apiResponse = api.Response
type apiError = api.ErrorInfo

// configData 是配置 API 响应中 Data 字段的内部结构。
type configData struct {
	// 消息提供附加信息
	Message string `json:"message,omitempty"`

	// 配置是当前配置（已清理）
	Config map[string]any `json:"config,omitempty"`

	// Fields 列出了热可重载字段
	Fields map[string]FieldInfo `json:"fields,omitempty"`

	// 更改列出配置更改
	Changes []ConfigChange `json:"changes,omitempty"`

	// CurrentVersion 当前配置版本
	CurrentVersion int `json:"current_version,omitempty"`

	// HistorySize 历史快照数量
	HistorySize int `json:"history_size,omitempty"`

	// RequiresRestart 表示是否需要重启
	RequiresRestart bool `json:"requires_restart,omitempty"`
}

// FieldInfo 提供有关配置字段的信息
type FieldInfo struct {
	// Path是字段路径
	Path string `json:"path"`

	// 字段描述
	Description string `json:"description"`

	// RequiresRestart 指示更改是否需要重新启动
	RequiresRestart bool `json:"requires_restart"`

	// Sensitive 表示该字段是否敏感
	Sensitive bool `json:"sensitive"`

	// CurrentValue 是当前值（如果敏感则进行编辑）
	CurrentValue any `json:"current_value,omitempty"`
}

// ConfigUpdateRequest 代表配置更新请求
type ConfigUpdateRequest struct {
	// 更新是到新值的字段路径的映射
	Updates map[string]any `json:"updates"`
}

// --- API 处理器实现 ---

// NewConfigAPIHandler 创建一个新的配置 API 处理程序。
// allowedOrigin 指定 CORS 允许的来源，为空时默认不设置 Access-Control-Allow-Origin。
func NewConfigAPIHandler(manager *HotReloadManager, allowedOrigin ...string) *ConfigAPIHandler {
	origin := ""
	if len(allowedOrigin) > 0 && allowedOrigin[0] != "" {
		origin = allowedOrigin[0]
	}
	return &ConfigAPIHandler{
		manager:       manager,
		allowedOrigin: origin,
		logger:        zap.NewNop(),
	}
}

// SetLogger 设置审计日志记录器 (X-004)
func (h *ConfigAPIHandler) SetLogger(logger *zap.Logger) {
	if logger != nil {
		h.logger = logger
	}
}

// HandleConfig 处理配置的 GET 和 PUT 请求（导出方法，供外部认证中间件包装使用）
func (h *ConfigAPIHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	h.handleConfig(w, r)
}

// HandleReload 处理配置热重载请求（导出方法）
func (h *ConfigAPIHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	h.handleReload(w, r)
}

// HandleFields 返回可热重载字段列表（导出方法）
func (h *ConfigAPIHandler) HandleFields(w http.ResponseWriter, r *http.Request) {
	h.handleFields(w, r)
}

// HandleChanges 返回配置变更历史（导出方法）
func (h *ConfigAPIHandler) HandleChanges(w http.ResponseWriter, r *http.Request) {
	h.handleChanges(w, r)
}

// HandleRollback 处理配置回滚请求（导出方法）
func (h *ConfigAPIHandler) HandleRollback(w http.ResponseWriter, r *http.Request) {
	h.handleRollback(w, r)
}

// handleConfig 处理配置的 GET 和 PUT 请求
func (h *ConfigAPIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getConfig(w, r)
	case http.MethodPut:
		h.updateConfig(w, r)
	case http.MethodOptions:
		h.handleCORS(w, r)
	default:
		h.methodNotAllowed(w, r)
	}
}

// getConfig 返回当前配置（已清理）
// @Summary 获取当前配置
// @Description 返回当前配置并编辑敏感字段
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} apiResponse "当前配置"
// @Failure 500 {object} apiResponse "内部服务器错误"
// @Router /api/v1/config [get]
func (h *ConfigAPIHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	if cfg := h.manager.GetConfig(); cfg == nil {
		writeAPIJSON(w, http.StatusInternalServerError, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INTERNAL_ERROR",
				Message: "Configuration unavailable",
			},
			Timestamp: time.Now(),
		})
		return
	}

	config := h.manager.SanitizedConfig()

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message: "Configuration retrieved successfully",
			Config:  config,
		},
		Timestamp: time.Now(),
	})
}

// updateConfig 更新配置字段
// @Summary 更新配置
// @Description 动态更新一个或多个配置字段
// @Tags config
// @Accept json
// @Produce json
// @Param request body ConfigUpdateRequest true "配置更新"
// @Success 200 {object} apiResponse "配置已更新"
// @Failure 400 {object} apiResponse "无效请求"
// @Failure 500 {object} apiResponse "内部服务器错误"
// @Router /api/v1/config [put]
func (h *ConfigAPIHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if !validateJSONContentType(w, r) {
		return
	}

	requestID := requestIDFromRequest(r)
	if r.ContentLength > maxConfigUpdateBodyBytes {
		writeAPIJSON(w, http.StatusRequestEntityTooLarge, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "REQUEST_TOO_LARGE",
				Message: "Request body too large",
			},
			Timestamp: time.Now(),
			RequestID: requestID,
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigUpdateBodyBytes)
	var req ConfigUpdateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeAPIJSON(w, http.StatusRequestEntityTooLarge, apiResponse{
				Success: false,
				Error: &apiError{
					Code:    "REQUEST_TOO_LARGE",
					Message: "Request body too large",
				},
				Timestamp: time.Now(),
				RequestID: requestID,
			})
			return
		}
		h.logger.Warn("invalid config update request body",
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("path", r.URL.Path),
			zap.String("request_id", requestID),
			zap.Error(err),
		)
		writeAPIJSON(w, http.StatusBadRequest, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: "Invalid request body",
			},
			Timestamp: time.Now(),
			RequestID: requestID,
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAPIJSON(w, http.StatusBadRequest, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: "Invalid request body",
			},
			Timestamp: time.Now(),
			RequestID: requestID,
		})
		return
	}

	if len(req.Updates) == 0 {
		writeAPIJSON(w, http.StatusBadRequest, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: "No updates provided",
			},
			Timestamp: time.Now(),
		})
		return
	}

	var errors []string
	var requiresRestart bool
	hotFields := GetHotReloadableFields()

	// X-004: 审计日志 — 记录配置变更请求
	for path, value := range req.Updates {
		// 检查字段是否已知
		field, known := hotFields[path]
		if !known {
			errors = append(errors, fmt.Sprintf("Unknown field: %s", path))
			continue
		}

		if field.RequiresRestart {
			requiresRestart = true
		}

		if err := h.manager.UpdateField(path, value); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to update %s: %v", path, err))
			h.logger.Warn("config update failed",
				zap.String("field", path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Error(err),
			)
		} else {
			h.logger.Info("config field updated",
				zap.String("field", path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.Bool("sensitive", field.Sensitive),
				zap.Time("timestamp", time.Now()),
			)
		}
	}

	if len(errors) > 0 {
		writeAPIJSON(w, http.StatusBadRequest, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: fmt.Sprintf("Some updates failed: %v", errors),
			},
			Data: configData{
				RequiresRestart: requiresRestart,
			},
			Timestamp: time.Now(),
		})
		return
	}

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message:         "Configuration updated successfully",
			Config:          h.manager.SanitizedConfig(),
			RequiresRestart: requiresRestart,
		},
		Timestamp: time.Now(),
	})
}

// handleReload 处理 POST 请求以从文件重新加载配置
// @Summary 从文件热重载配置
// @Description 从配置文件热重载并应用最新配置
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} apiResponse "配置已热重载"
// @Failure 500 {object} apiResponse "热重载失败"
// @Router /api/v1/config/reload [post]
func (h *ConfigAPIHandler) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.methodNotAllowed(w, r)
		return
	}

	// POST with body is optional for reload; Content-Type may be absent.
	// When present, it must be application/json (consistent with validateJSONContentType).
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeAPIJSON(w, http.StatusUnsupportedMediaType, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: "Content-Type must be application/json",
			},
			Timestamp: time.Now(),
		})
		return
	}

	if err := h.manager.ReloadFromFile(); err != nil {
		h.logger.Warn("config reload failed",
			zap.String("remote_addr", r.RemoteAddr),
			zap.Error(err),
		)
		writeAPIJSON(w, http.StatusInternalServerError, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("Failed to reload configuration: %v", err),
			},
			Timestamp: time.Now(),
		})
		return
	}

	// X-004: 审计日志 — 记录配置热重载
	h.logger.Info("config reloaded from file",
		zap.String("remote_addr", r.RemoteAddr),
		zap.Time("timestamp", time.Now()),
	)

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message: "Configuration reloaded successfully",
			Config:  h.manager.SanitizedConfig(),
		},
		Timestamp: time.Now(),
	})
}

// handleFields 返回热可重载字段的列表
// @Summary 获取可热重载字段
// @Description 返回支持热重载的配置字段列表
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} apiResponse "可热重载字段"
// @Router /api/v1/config/fields [get]
func (h *ConfigAPIHandler) handleFields(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, r)
		return
	}

	fields := make(map[string]FieldInfo)
	for path, field := range GetHotReloadableFields() {
		reloadable := IsHotReloadable(path)
		info := FieldInfo{
			Path:            path,
			Description:     field.Description,
			RequiresRestart: !reloadable,
			Sensitive:       field.Sensitive,
		}

		// 如果不敏感则获取当前值
		if !field.Sensitive {
			if value, err := h.manager.getFieldValue(path); err == nil {
				info.CurrentValue = value
			}
		}

		fields[path] = info
	}

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message: "Hot reloadable fields retrieved",
			Fields:  fields,
		},
		Timestamp: time.Now(),
	})
}

// handleChanges 返回配置更改历史记录
// @Summary 获取配置更改历史记录
// @Description 返回配置更改的历史记录
// @Tags config
// @Accept json
// @Produce json
// @Param limit query int false "返回的最大更改数量" default(50)
// @Success 200 {object} apiResponse "配置更改"
// @Router /api/v1/config/changes [get]
func (h *ConfigAPIHandler) handleChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, r)
		return
	}

	// 解析限制参数
	const maxPageLimit = 1000
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if limit <= 0 {
		limit = 50
	}

	changes := h.manager.GetChangeLog(limit)
	history := h.manager.GetConfigHistory()
	currentVersion := h.manager.GetCurrentVersion()

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message:        fmt.Sprintf("Retrieved %d configuration changes", len(changes)),
			Changes:        changes,
			CurrentVersion: currentVersion,
			HistorySize:    len(history),
		},
		Timestamp: time.Now(),
	})
}

// handleRollback 处理配置回滚请求
// @Summary 回滚配置
// @Description 回滚到上一个版本，或通过 version 参数回滚到指定版本
// @Tags config
// @Accept json
// @Produce json
// @Param version query int false "目标版本号"
// @Success 200 {object} apiResponse "配置已回滚"
// @Failure 400 {object} apiResponse "无效请求"
// @Failure 500 {object} apiResponse "回滚失败"
// @Router /api/v1/config/rollback [post]
func (h *ConfigAPIHandler) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleCORS(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.methodNotAllowed(w, r)
		return
	}

	var rollbackErr error
	if versionStr := strings.TrimSpace(r.URL.Query().Get("version")); versionStr != "" {
		version, err := strconv.Atoi(versionStr)
		if err != nil || version <= 0 {
			writeAPIJSON(w, http.StatusBadRequest, apiResponse{
				Success: false,
				Error: &apiError{
					Code:    "INVALID_REQUEST",
					Message: "version must be a positive integer",
				},
				Timestamp: time.Now(),
				RequestID: requestIDFromRequest(r),
			})
			return
		}
		rollbackErr = h.manager.RollbackToVersion(version)
	} else {
		rollbackErr = h.manager.Rollback()
	}

	if rollbackErr != nil {
		writeAPIJSON(w, http.StatusInternalServerError, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("Failed to rollback configuration: %v", rollbackErr),
			},
			Timestamp: time.Now(),
			RequestID: requestIDFromRequest(r),
		})
		return
	}

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message:        "Configuration rolled back successfully",
			Config:         h.manager.SanitizedConfig(),
			CurrentVersion: h.manager.GetCurrentVersion(),
		},
		Timestamp: time.Now(),
		RequestID: requestIDFromRequest(r),
	})
}

// --- 辅助方法 ---

// validateJSONContentType checks that the request Content-Type is application/json.
// Returns true if valid; on false the caller should return immediately (415 already written).
func validateJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeAPIJSON(w, http.StatusUnsupportedMediaType, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: "Content-Type must be application/json",
			},
			Timestamp: time.Now(),
		})
		return false
	}
	return true
}

// writeAPIJSON writes a JSON response using the marshal-first pattern (§6).
// Uses the same Content-Type and security headers as handlers.WriteJSON.
func writeAPIJSON(w http.ResponseWriter, status int, data any) {
	api.WriteJSONResponse(w, status, data)
}

// handleCORS 处理 CORS 预检请求
func (h *ConfigAPIHandler) handleCORS(w http.ResponseWriter, r *http.Request) { //nolint:unparam // r 参数保留以符合 http.HandlerFunc 签名
	if h.allowedOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", h.allowedOrigin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// methodNotAllowed 返回 405 方法不允许响应
func (h *ConfigAPIHandler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeAPIJSON(w, http.StatusMethodNotAllowed, apiResponse{
		Success: false,
		Error: &apiError{
			Code:    "METHOD_NOT_ALLOWED",
			Message: fmt.Sprintf("Method %s not allowed", r.Method),
		},
		Timestamp: time.Now(),
	})
}

// --- 中间件 ---

// ConfigAPIMiddleware 为配置API提供中间件
type ConfigAPIMiddleware struct {
	handler *ConfigAPIHandler
	apiKey  string
	mu      sync.Mutex
	limiter map[string]*configAPILimiterEntry

	lastCleanup time.Time
}

type configAPILimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewConfigAPIMiddleware 创建一个新的配置API中间件
func NewConfigAPIMiddleware(handler *ConfigAPIHandler, apiKey string) *ConfigAPIMiddleware {
	return &ConfigAPIMiddleware{
		handler:     handler,
		apiKey:      apiKey,
		limiter:     make(map[string]*configAPILimiterEntry),
		lastCleanup: time.Now(),
	}
}

// RequireAuth 使用 API 密钥身份验证包装处理程序
func (m *ConfigAPIMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.allowRequest(r.RemoteAddr) {
			writeAPIJSON(w, http.StatusTooManyRequests, apiResponse{
				Success: false,
				Error: &apiError{
					Code:    "RATE_LIMITED",
					Message: "Too many requests",
				},
				Timestamp: time.Now(),
				RequestID: requestIDFromRequest(r),
			})
			return
		}

		// 跳过 OPTIONS 请求的身份验证（CORS 预检）
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		// 检查 API 密钥（如果已配置）
		if m.apiKey != "" {
			apiKey := r.Header.Get("X-API-Key")
			// 不再支持 query string 传递 API key（安全风险：会暴露在日志和浏览器历史中）

			if !secureTokenEqual(apiKey, m.apiKey) {
				m.handler.logger.Warn("config api authentication failed",
					zap.String("remote_addr", r.RemoteAddr),
					zap.String("path", r.URL.Path),
					zap.String("method", r.Method),
					zap.String("provided_api_key", MaskAPIKey(apiKey)),
					zap.String("request_id", requestIDFromRequest(r)),
				)
				writeAPIJSON(w, http.StatusUnauthorized, apiResponse{
					Success: false,
					Error: &apiError{
						Code:    "UNAUTHORIZED",
						Message: "Invalid or missing API key",
					},
					Timestamp: time.Now(),
					RequestID: requestIDFromRequest(r),
				})
				return
			}
		}

		next(w, r)
	}
}

func (m *ConfigAPIMiddleware) allowRequest(remoteAddr string) bool {
	const (
		rps             = 5.0
		burst           = 20
		cleanupInterval = time.Minute
		entryTTL        = 3 * time.Minute
	)
	now := time.Now()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}

	m.mu.Lock()
	if now.Sub(m.lastCleanup) >= cleanupInterval {
		for key, entry := range m.limiter {
			if now.Sub(entry.lastSeen) > entryTTL {
				delete(m.limiter, key)
			}
		}
		m.lastCleanup = now
	}

	entry, ok := m.limiter[ip]
	if !ok {
		entry = &configAPILimiterEntry{
			limiter: rate.NewLimiter(rate.Limit(rps), burst),
		}
		m.limiter[ip] = entry
	}
	entry.lastSeen = now
	allow := entry.limiter.Allow()
	m.mu.Unlock()
	return allow
}

// LogRequests 使用请求日志记录来包装处理程序
func (m *ConfigAPIMiddleware) LogRequests(next http.HandlerFunc, logger func(method, path string, status int, duration time.Duration)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装响应编写器以捕获状态代码
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next(wrapped, r)

		if logger != nil {
			logger(r.Method, r.URL.Path, wrapped.status, time.Since(start))
		}
	}
}

// responseWriter 包装 http.ResponseWriter 来捕获状态码
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func secureTokenEqual(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if requestID := strings.TrimSpace(r.Header.Get("X-Request-ID")); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(r.Header.Get("X-Request-Id"))
}
