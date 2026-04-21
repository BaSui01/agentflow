package usecase

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/types"
)

type ToolProviderService interface {
	List() ([]ToolProviderView, *types.Error)
	Upsert(provider string, req UpsertToolProviderInput) (*ToolProviderView, *types.Error)
	Delete(provider string) *types.Error
	Reload() *types.Error
}

type ToolProviderStore interface {
	List() ([]hosted.ToolProviderConfig, error)
	GetByProvider(provider string) (hosted.ToolProviderConfig, error)
	Create(row *hosted.ToolProviderConfig) error
	Update(row *hosted.ToolProviderConfig, updates map[string]any) error
	Reload(row *hosted.ToolProviderConfig) error
	DeleteByProvider(provider string) (int64, error)
}

type DefaultToolProviderService struct {
	store   ToolProviderStore
	runtime ToolRegistryRuntime
}

func NewDefaultToolProviderService(store ToolProviderStore, runtime ToolRegistryRuntime) *DefaultToolProviderService {
	return &DefaultToolProviderService{store: store, runtime: runtime}
}

func (s *DefaultToolProviderService) List() ([]ToolProviderView, *types.Error) {
	rows, err := s.store.List()
	if err != nil {
		return nil, types.NewInternalError("failed to list tool providers").WithCause(err)
	}
	out := make([]ToolProviderView, 0, len(rows))
	for _, row := range rows {
		out = append(out, toToolProviderResponse(row))
	}
	return out, nil
}

func (s *DefaultToolProviderService) Upsert(provider string, req UpsertToolProviderInput) (*ToolProviderView, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool runtime is not configured")
	}
	p, errResp := normalizeAndValidateProvider(provider)
	if errResp != nil {
		return nil, errResp
	}
	if errResp = validateUpsertToolProviderRequest(p, req); errResp != nil {
		return nil, errResp
	}

	row, err := s.store.GetByProvider(p)
	if err != nil {
		// create path
		newRow := &hosted.ToolProviderConfig{
			Provider:       p,
			APIKey:         strings.TrimSpace(req.APIKey),
			BaseURL:        strings.TrimSpace(req.BaseURL),
			TimeoutSeconds: req.TimeoutSeconds,
			Priority:       req.Priority,
			Enabled:        req.Enabled == nil || *req.Enabled,
		}
		if err := s.store.Create(newRow); err != nil {
			return nil, types.NewInternalError("failed to create tool provider").WithCause(err)
		}
		if err := s.runtime.ReloadBindings(context.Background()); err != nil {
			return nil, types.NewInternalError("provider created but failed to reload tool runtime").WithCause(err)
		}
		resp := toToolProviderResponse(*newRow)
		return &resp, nil
	}

	updates := map[string]any{
		"api_key":         strings.TrimSpace(req.APIKey),
		"base_url":        strings.TrimSpace(req.BaseURL),
		"timeout_seconds": req.TimeoutSeconds,
		"priority":        req.Priority,
		"enabled":         req.Enabled == nil || *req.Enabled,
	}
	if err := s.store.Update(&row, updates); err != nil {
		return nil, types.NewInternalError("failed to update tool provider").WithCause(err)
	}
	if err := s.store.Reload(&row); err != nil {
		return nil, types.NewInternalError("failed to reload tool provider").WithCause(err)
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return nil, types.NewInternalError("provider updated but failed to reload tool runtime").WithCause(err)
	}
	resp := toToolProviderResponse(row)
	return &resp, nil
}

func (s *DefaultToolProviderService) Delete(provider string) *types.Error {
	if s.runtime == nil {
		return types.NewInternalError("tool runtime is not configured")
	}
	p, errResp := normalizeAndValidateProvider(provider)
	if errResp != nil {
		return errResp
	}
	rows, err := s.store.DeleteByProvider(p)
	if err != nil {
		return types.NewInternalError("failed to delete tool provider").WithCause(err)
	}
	if rows == 0 {
		return types.NewNotFoundError("tool provider not found")
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return types.NewInternalError("provider deleted but failed to reload tool runtime").WithCause(err)
	}
	return nil
}

func (s *DefaultToolProviderService) Reload() *types.Error {
	if s.runtime == nil {
		return types.NewInternalError("tool runtime is not configured")
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return types.NewInternalError("failed to reload tool runtime").WithCause(err)
	}
	return nil
}

func normalizeAndValidateProvider(provider string) (string, *types.Error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch hosted.ToolProviderName(p) {
	case hosted.ToolProviderTavily, hosted.ToolProviderFirecrawl, hosted.ToolProviderDuckDuckGo, hosted.ToolProviderSearXNG:
		return p, nil
	default:
		return "", types.NewError(types.ErrInvalidRequest, "provider must be one of: tavily, firecrawl, duckduckgo, searxng")
	}
}

func validateUpsertToolProviderRequest(provider string, req UpsertToolProviderInput) *types.Error {
	if req.TimeoutSeconds <= 0 {
		return types.NewError(types.ErrInvalidRequest, "timeout_seconds must be positive")
	}
	if req.Priority < 0 {
		return types.NewError(types.ErrInvalidRequest, "priority must be non-negative")
	}

	switch hosted.ToolProviderName(provider) {
	case hosted.ToolProviderTavily, hosted.ToolProviderFirecrawl:
		if strings.TrimSpace(req.APIKey) == "" {
			return types.NewError(types.ErrInvalidRequest, "api_key is required for tavily/firecrawl")
		}
	case hosted.ToolProviderSearXNG:
		if strings.TrimSpace(req.BaseURL) == "" {
			return types.NewError(types.ErrInvalidRequest, "base_url is required for searxng")
		}
	}

	return nil
}

func toToolProviderResponse(row hosted.ToolProviderConfig) ToolProviderView {
	return ToolProviderView{
		ID:             row.ID,
		Provider:       row.Provider,
		BaseURL:        row.BaseURL,
		TimeoutSeconds: row.TimeoutSeconds,
		Priority:       row.Priority,
		Enabled:        row.Enabled,
		HasAPIKey:      strings.TrimSpace(row.APIKey) != "",
	}
}
