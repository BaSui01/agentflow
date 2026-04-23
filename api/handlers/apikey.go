package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// APIKeyHandler 处理 API Key 管理的 CRUD 操作
type APIKeyHandler struct {
	svc    usecase.APIKeyService
	logger *zap.Logger
}

func NewAPIKeyHandler(service usecase.APIKeyService, logger *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{
		svc:    service,
		logger: logger,
	}
}

// maskAPIKey 脱敏 API Key，仅显示末 4 位
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(key)-4) + key[len(key)-4:]
}

// extractProviderID 从请求中提取 provider ID（Go 1.22+ PathValue 优先，回退到路径解析）
func extractProviderID(r *http.Request) (uint, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		// 回退：从路径手动解析
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			return 0, false
		}
		idStr = parts[3]
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

func extractKeyID(r *http.Request) (uint, bool) {
	idStr := r.PathValue("keyId")
	if idStr == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 6 {
			return 0, false
		}
		idStr = parts[5]
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// HandleListProviders GET /api/v1/providers
func (h *APIKeyHandler) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providersData, svcErr := h.svc.ListProviders()
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	WriteSuccess(w, providersData)
}

// apiKeyResponse 脱敏后的 API Key 响应
type apiKeyResponse struct {
	ID             uint   `json:"id"`
	ProviderID     uint   `json:"provider_id"`
	APIKeyMasked   string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Label          string `json:"label"`
	Priority       int    `json:"priority"`
	Weight         int    `json:"weight"`
	Enabled        bool   `json:"enabled"`
	TotalRequests  int64  `json:"total_requests"`
	FailedRequests int64  `json:"failed_requests"`
	RateLimitRPM   int    `json:"rate_limit_rpm"`
	RateLimitRPD   int    `json:"rate_limit_rpd"`
}

type apiKeyStatsResponse struct {
	KeyID          uint       `json:"key_id"`
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	Enabled        bool       `json:"enabled"`
	IsHealthy      bool       `json:"is_healthy"`
	TotalRequests  int64      `json:"total_requests"`
	FailedRequests int64      `json:"failed_requests"`
	SuccessRate    float64    `json:"success_rate"`
	CurrentRPM     int        `json:"current_rpm"`
	CurrentRPD     int        `json:"current_rpd"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	LastErrorAt    *time.Time `json:"last_error_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

func toAPIKeyResponse(k llm.LLMProviderAPIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:             k.ID,
		ProviderID:     k.ProviderID,
		APIKeyMasked:   maskAPIKey(k.APIKey),
		BaseURL:        k.BaseURL,
		Label:          k.Label,
		Priority:       k.Priority,
		Weight:         k.Weight,
		Enabled:        k.Enabled,
		TotalRequests:  k.TotalRequests,
		FailedRequests: k.FailedRequests,
		RateLimitRPM:   k.RateLimitRPM,
		RateLimitRPD:   k.RateLimitRPD,
	}
}

// HandleListAPIKeys GET /api/v1/providers/{id}/api-keys
// Upper bound is enforced by store layer Limit; no handler-level pagination.
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providerID, ok := extractProviderID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid provider ID", h.logger)
		return
	}

	resp, svcErr := h.svc.ListAPIKeys(providerID)
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	WriteSuccess(w, resp)
}

// createAPIKeyRequest 创建 API Key 请求体
type createAPIKeyRequest struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	Label        string `json:"label"`
	Priority     int    `json:"priority"`
	Weight       int    `json:"weight"`
	Enabled      *bool  `json:"enabled"`
	RateLimitRPM int    `json:"rate_limit_rpm"`
	RateLimitRPD int    `json:"rate_limit_rpd"`
}

// HandleCreateAPIKey POST /api/v1/providers/{id}/api-keys
func (h *APIKeyHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providerID, ok := extractProviderID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid provider ID", h.logger)
		return
	}

	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req createAPIKeyRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, svcErr := h.svc.CreateAPIKey(providerID, usecase.CreateAPIKeyInput{
		APIKey:       req.APIKey,
		BaseURL:      req.BaseURL,
		Label:        req.Label,
		Priority:     req.Priority,
		Weight:       req.Weight,
		Enabled:      req.Enabled,
		RateLimitRPM: req.RateLimitRPM,
		RateLimitRPD: req.RateLimitRPD,
	})
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	WriteJSON(w, http.StatusCreated, api.Response{
		Success:   true,
		Data:      resp,
		Timestamp: time.Now(),
		RequestID: w.Header().Get("X-Request-ID"),
	})
}

// updateAPIKeyRequest 更新 API Key 请求体
type updateAPIKeyRequest struct {
	BaseURL      *string `json:"base_url"`
	Label        *string `json:"label"`
	Priority     *int    `json:"priority"`
	Weight       *int    `json:"weight"`
	Enabled      *bool   `json:"enabled"`
	RateLimitRPM *int    `json:"rate_limit_rpm"`
	RateLimitRPD *int    `json:"rate_limit_rpd"`
}

// HandleUpdateAPIKey PUT /api/v1/providers/{id}/api-keys/{keyId}
func (h *APIKeyHandler) HandleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providerID, ok := extractProviderID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid provider ID", h.logger)
		return
	}

	keyID, ok := extractKeyID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid key ID", h.logger)
		return
	}

	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req updateAPIKeyRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, svcErr := h.svc.UpdateAPIKey(providerID, keyID, usecase.UpdateAPIKeyInput{
		BaseURL:      req.BaseURL,
		Label:        req.Label,
		Priority:     req.Priority,
		Weight:       req.Weight,
		Enabled:      req.Enabled,
		RateLimitRPM: req.RateLimitRPM,
		RateLimitRPD: req.RateLimitRPD,
	})
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}
	WriteSuccess(w, resp)
}

// HandleDeleteAPIKey DELETE /api/v1/providers/{id}/api-keys/{keyId}
func (h *APIKeyHandler) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providerID, ok := extractProviderID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid provider ID", h.logger)
		return
	}

	keyID, ok := extractKeyID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid key ID", h.logger)
		return
	}

	if svcErr := h.svc.DeleteAPIKey(providerID, keyID); svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	WriteSuccess(w, map[string]string{"message": "API key deleted"})
}

// HandleAPIKeyStats GET /api/v1/providers/{id}/api-keys/stats
func (h *APIKeyHandler) HandleAPIKeyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", h.logger)
		return
	}

	providerID, ok := extractProviderID(r)
	if !ok {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid provider ID", h.logger)
		return
	}

	stats, svcErr := h.svc.ListAPIKeyStats(providerID)
	if svcErr != nil {
		WriteError(w, svcErr, h.logger)
		return
	}

	WriteSuccess(w, stats)
}
