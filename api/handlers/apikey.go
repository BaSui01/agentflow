package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// APIKeyHandler 处理 API Key 管理的 CRUD 操作
type APIKeyHandler struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewAPIKeyHandler 创建 APIKeyHandler
func NewAPIKeyHandler(db *gorm.DB, logger *zap.Logger) *APIKeyHandler {
	return &APIKeyHandler{db: db, logger: logger}
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

	var providers []llm.LLMProvider
	if err := h.db.Order("id ASC").Find(&providers).Error; err != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to list providers", h.logger)
		return
	}

	WriteSuccess(w, providers)
}

// PLACEHOLDER_CRUD

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

	var keys []llm.LLMProviderAPIKey
	if err := h.db.Where("provider_id = ?", providerID).Order("priority ASC, id ASC").Find(&keys).Error; err != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to list API keys", h.logger)
		return
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, toAPIKeyResponse(k))
	}
	WriteSuccess(w, resp)
}

// PLACEHOLDER_CREATE_UPDATE_DELETE

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

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid request body", h.logger)
		return
	}

	if strings.TrimSpace(req.APIKey) == "" {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "api_key is required", h.logger)
		return
	}

	key := llm.LLMProviderAPIKey{
		ProviderID:   providerID,
		APIKey:       req.APIKey,
		BaseURL:      req.BaseURL,
		Label:        req.Label,
		Priority:     req.Priority,
		Weight:       req.Weight,
		Enabled:      req.Enabled == nil || *req.Enabled,
		RateLimitRPM: req.RateLimitRPM,
		RateLimitRPD: req.RateLimitRPD,
	}
	if key.Priority == 0 {
		key.Priority = 100
	}
	if key.Weight == 0 {
		key.Weight = 100
	}

	if err := h.db.Create(&key).Error; err != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to create API key", h.logger)
		return
	}

	WriteJSON(w, http.StatusCreated, Response{
		Success: true,
		Data:    toAPIKeyResponse(key),
	})
}

// PLACEHOLDER_UPDATE_DELETE_STATS

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

	var existing llm.LLMProviderAPIKey
	if err := h.db.Where("id = ? AND provider_id = ?", keyID, providerID).First(&existing).Error; err != nil {
		WriteErrorMessage(w, http.StatusNotFound, types.ErrInvalidRequest, "API key not found", h.logger)
		return
	}

	var req updateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid request body", h.logger)
		return
	}

	updates := map[string]any{}
	if req.BaseURL != nil {
		updates["base_url"] = *req.BaseURL
	}
	if req.Label != nil {
		updates["label"] = *req.Label
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.RateLimitRPM != nil {
		updates["rate_limit_rpm"] = *req.RateLimitRPM
	}
	if req.RateLimitRPD != nil {
		updates["rate_limit_rpd"] = *req.RateLimitRPD
	}

// PLACEHOLDER_FINAL

	if len(updates) == 0 {
		WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "no fields to update", h.logger)
		return
	}

	if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to update API key", h.logger)
		return
	}

	// 重新加载
	h.db.First(&existing, keyID)
	WriteSuccess(w, toAPIKeyResponse(existing))
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

	result := h.db.Where("id = ? AND provider_id = ?", keyID, providerID).Delete(&llm.LLMProviderAPIKey{})
	if result.Error != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to delete API key", h.logger)
		return
	}
	if result.RowsAffected == 0 {
		WriteErrorMessage(w, http.StatusNotFound, types.ErrInvalidRequest, "API key not found", h.logger)
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

	var keys []llm.LLMProviderAPIKey
	if err := h.db.Where("provider_id = ?", providerID).Find(&keys).Error; err != nil {
		WriteErrorMessage(w, http.StatusInternalServerError, types.ErrInternalError, "failed to load API keys", h.logger)
		return
	}

	stats := make([]llm.APIKeyStats, 0, len(keys))
	for _, k := range keys {
		successRate := 1.0
		if k.TotalRequests > 0 {
			successRate = float64(k.TotalRequests-k.FailedRequests) / float64(k.TotalRequests)
		}
		stats = append(stats, llm.APIKeyStats{
			KeyID:          k.ID,
			Label:          k.Label,
			BaseURL:        k.BaseURL,
			Enabled:        k.Enabled,
			IsHealthy:      k.IsHealthy(),
			TotalRequests:  k.TotalRequests,
			FailedRequests: k.FailedRequests,
			SuccessRate:    successRate,
			CurrentRPM:     k.CurrentRPM,
			CurrentRPD:     k.CurrentRPD,
			LastUsedAt:     k.LastUsedAt,
			LastErrorAt:    k.LastErrorAt,
			LastError:      k.LastError,
		})
	}

	WriteSuccess(w, stats)
}
