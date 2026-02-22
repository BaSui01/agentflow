// config 包的 HTTP 配置管理 API。
//
// 提供配置查询、更新、热重载触发与变更历史查询能力。
package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/BaSui01/agentflow/api"
)

// --- API 类型定义 ---

// ConfigAPIHandler 处理配置 API 请求
type ConfigAPIHandler struct {
	manager       *HotReloadManager
	allowedOrigin string
}

// apiResponse is a type alias for api.Response — the canonical API envelope (§38).
type apiResponse = api.Response

// apiError is a type alias for api.ErrorInfo — the canonical error structure (§38).
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
	}
}

// RegisterRoutes 注册配置 API 路由
func (h *ConfigAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/config", h.handleConfig)
	mux.HandleFunc("/api/v1/config/reload", h.handleReload)
	mux.HandleFunc("/api/v1/config/fields", h.handleFields)
	mux.HandleFunc("/api/v1/config/changes", h.handleChanges)
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
	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIJSON(w, http.StatusBadRequest, apiResponse{
			Success: false,
			Error: &apiError{
				Code:    "INVALID_REQUEST",
				Message: fmt.Sprintf("Invalid request body: %v", err),
			},
			Timestamp: time.Now(),
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

	for path, value := range req.Updates {
		// 检查字段是否已知
		field, known := hotReloadableFields[path]
		if !known {
			errors = append(errors, fmt.Sprintf("Unknown field: %s", path))
			continue
		}

		if field.RequiresRestart {
			requiresRestart = true
		}

		if err := h.manager.UpdateField(path, value); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to update %s: %v", path, err))
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

	if err := h.manager.ReloadFromFile(); err != nil {
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
	for path, field := range hotReloadableFields {
		info := FieldInfo{
			Path:            path,
			Description:     field.Description,
			RequiresRestart: field.RequiresRestart,
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
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	changes := h.manager.GetChangeLog(limit)

	writeAPIJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data: configData{
			Message: fmt.Sprintf("Retrieved %d configuration changes", len(changes)),
			Changes: changes,
		},
		Timestamp: time.Now(),
	})
}

// --- 辅助方法 ---

// writeAPIJSON writes a JSON response using the marshal-first pattern (§6).
// Uses the same Content-Type and security headers as handlers.WriteJSON.
func writeAPIJSON(w http.ResponseWriter, status int, data any) {
	buf, err := json.Marshal(data)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"INTERNAL_ERROR","message":"failed to encode response"}}`)) //nolint:errcheck // Write 错误可安全忽略（客户端断开）
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf) //nolint:errcheck // Write 错误可安全忽略（客户端断开）
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
}

// NewConfigAPIMiddleware 创建一个新的配置API中间件
func NewConfigAPIMiddleware(handler *ConfigAPIHandler, apiKey string) *ConfigAPIMiddleware {
	return &ConfigAPIMiddleware{
		handler: handler,
		apiKey:  apiKey,
	}
}

// RequireAuth 使用 API 密钥身份验证包装处理程序
func (m *ConfigAPIMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 跳过 OPTIONS 请求的身份验证（CORS 预检）
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		// 检查 API 密钥（如果已配置）
		if m.apiKey != "" {
			apiKey := r.Header.Get("X-API-Key")
			// 不再支持 query string 传递 API key（安全风险：会暴露在日志和浏览器历史中）

			if apiKey != m.apiKey {
				writeAPIJSON(w, http.StatusUnauthorized, apiResponse{
					Success: false,
					Error: &apiError{
						Code:    "UNAUTHORIZED",
						Message: "Invalid or missing API key",
					},
					Timestamp: time.Now(),
				})
				return
			}
		}

		next(w, r)
	}
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
