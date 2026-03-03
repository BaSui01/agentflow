package handlers

import (
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// APIKeyService encapsulates API key CRUD/use-case logic for API handlers.
type APIKeyService interface {
	ListProviders() ([]llm.LLMProvider, *types.Error)
	ListAPIKeys(providerID uint) ([]apiKeyResponse, *types.Error)
	CreateAPIKey(providerID uint, req createAPIKeyRequest) (*apiKeyResponse, *types.Error)
	UpdateAPIKey(providerID, keyID uint, req updateAPIKeyRequest) (*apiKeyResponse, *types.Error)
	DeleteAPIKey(providerID, keyID uint) *types.Error
	ListAPIKeyStats(providerID uint) ([]apiKeyStatsResponse, *types.Error)
}

type DefaultAPIKeyService struct {
	store APIKeyStore
}

func NewDefaultAPIKeyService(store APIKeyStore) *DefaultAPIKeyService {
	return &DefaultAPIKeyService{store: store}
}

func (s *DefaultAPIKeyService) ListProviders() ([]llm.LLMProvider, *types.Error) {
	providers, err := s.store.ListProviders()
	if err != nil {
		return nil, types.NewInternalError("failed to list providers").WithCause(err)
	}
	return providers, nil
}

func (s *DefaultAPIKeyService) ListAPIKeys(providerID uint) ([]apiKeyResponse, *types.Error) {
	keys, err := s.store.ListAPIKeys(providerID)
	if err != nil {
		return nil, types.NewInternalError("failed to list API keys").WithCause(err)
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, toAPIKeyResponse(k))
	}
	return resp, nil
}

func (s *DefaultAPIKeyService) CreateAPIKey(providerID uint, req createAPIKeyRequest) (*apiKeyResponse, *types.Error) {
	if req.APIKey == "" {
		return nil, types.NewError(types.ErrInvalidRequest, "api_key is required")
	}
	if req.BaseURL != "" && !ValidateURL(req.BaseURL) {
		return nil, types.NewError(types.ErrInvalidRequest, "base_url must be a valid HTTP or HTTPS URL")
	}
	if req.Priority < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "priority must be non-negative")
	}
	if req.Weight < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "weight must be non-negative")
	}
	if req.RateLimitRPM < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "rate_limit_rpm must be non-negative")
	}
	if req.RateLimitRPD < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "rate_limit_rpd must be non-negative")
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

	if err := s.store.CreateAPIKey(&key); err != nil {
		return nil, types.NewInternalError("failed to create API key").WithCause(err)
	}

	resp := toAPIKeyResponse(key)
	return &resp, nil
}

func (s *DefaultAPIKeyService) UpdateAPIKey(providerID, keyID uint, req updateAPIKeyRequest) (*apiKeyResponse, *types.Error) {
	existing, err := s.store.GetAPIKey(keyID, providerID)
	if err != nil {
		return nil, types.NewNotFoundError("API key not found")
	}

	if req.BaseURL != nil && *req.BaseURL != "" && !ValidateURL(*req.BaseURL) {
		return nil, types.NewError(types.ErrInvalidRequest, "base_url must be a valid HTTP or HTTPS URL")
	}
	if req.Priority != nil && *req.Priority < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "priority must be non-negative")
	}
	if req.Weight != nil && *req.Weight < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "weight must be non-negative")
	}
	if req.RateLimitRPM != nil && *req.RateLimitRPM < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "rate_limit_rpm must be non-negative")
	}
	if req.RateLimitRPD != nil && *req.RateLimitRPD < 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "rate_limit_rpd must be non-negative")
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
	if len(updates) == 0 {
		return nil, types.NewError(types.ErrInvalidRequest, "no fields to update")
	}

	if err := s.store.UpdateAPIKey(&existing, updates); err != nil {
		return nil, types.NewInternalError("failed to update API key").WithCause(err)
	}
	if err := s.store.ReloadAPIKey(&existing); err != nil {
		return nil, types.NewInternalError("failed to reload API key").WithCause(err)
	}
	resp := toAPIKeyResponse(existing)
	return &resp, nil
}

func (s *DefaultAPIKeyService) DeleteAPIKey(providerID, keyID uint) *types.Error {
	rowsAffected, err := s.store.DeleteAPIKey(keyID, providerID)
	if err != nil {
		return types.NewInternalError("failed to delete API key").WithCause(err)
	}
	if rowsAffected == 0 {
		return types.NewNotFoundError("API key not found")
	}
	return nil
}

func (s *DefaultAPIKeyService) ListAPIKeyStats(providerID uint) ([]apiKeyStatsResponse, *types.Error) {
	keys, err := s.store.ListAPIKeys(providerID)
	if err != nil {
		return nil, types.NewInternalError("failed to load API keys").WithCause(err)
	}

	stats := make([]apiKeyStatsResponse, 0, len(keys))
	for _, k := range keys {
		successRate := 1.0
		if k.TotalRequests > 0 {
			successRate = float64(k.TotalRequests-k.FailedRequests) / float64(k.TotalRequests)
		}
		stats = append(stats, apiKeyStatsResponse{
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

	return stats, nil
}
