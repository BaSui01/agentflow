package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/types"
)

var toolRegistrationNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,119}$`)

// TODO(refactor): ToolRegistryService should be moved to internal/app/service or a domain layer package.
// It currently resides in api/handlers but contains business logic that belongs in Layer 2-3.
//
// ToolRegistryRuntime describes runtime hooks used by tool registration API.
type ToolRegistryRuntime interface {
	ReloadBindings(ctx context.Context) error
	BaseToolNames() []string
}

type ToolRegistryService interface {
	List() ([]hosted.ToolRegistration, *types.Error)
	ListTargets() ([]string, *types.Error)
	Create(req createToolRegistrationRequest) (*hosted.ToolRegistration, *types.Error)
	Update(ctx context.Context, id uint, req updateToolRegistrationRequest) (*hosted.ToolRegistration, *types.Error)
	Delete(id uint) *types.Error
	Reload() *types.Error
}

type DefaultToolRegistryService struct {
	store   hosted.ToolRegistryStore
	runtime ToolRegistryRuntime
}

func NewDefaultToolRegistryService(store hosted.ToolRegistryStore, runtime ToolRegistryRuntime) *DefaultToolRegistryService {
	return &DefaultToolRegistryService{
		store:   store,
		runtime: runtime,
	}
}

func (s *DefaultToolRegistryService) List() ([]hosted.ToolRegistration, *types.Error) {
	rows, err := s.store.List()
	if err != nil {
		return nil, types.NewInternalError("failed to list tool registrations").WithCause(err)
	}
	return rows, nil
}

func (s *DefaultToolRegistryService) ListTargets() ([]string, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool runtime is not configured")
	}
	targets := s.runtime.BaseToolNames()
	sort.Strings(targets)
	return targets, nil
}

func (s *DefaultToolRegistryService) Create(req createToolRegistrationRequest) (*hosted.ToolRegistration, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool runtime is not configured")
	}
	name := strings.TrimSpace(req.Name)
	target := strings.TrimSpace(req.Target)
	if err := validateToolRegistrationName(name); err != nil {
		return nil, err
	}
	if err := s.validateAliasName(name); err != nil {
		return nil, err
	}
	if err := s.validateTarget(target); err != nil {
		return nil, err
	}
	if name == target {
		return nil, types.NewError(types.ErrInvalidRequest, "name and target must be different")
	}
	if err := validateToolRegistrationParameters(req.Parameters); err != nil {
		return nil, err
	}

	row := &hosted.ToolRegistration{
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Target:      target,
		Parameters:  req.Parameters,
		Enabled:     req.Enabled == nil || *req.Enabled,
	}
	if err := s.store.Create(row); err != nil {
		if isUniqueViolation(err) {
			return nil, types.NewError(types.ErrInvalidRequest, "tool name already exists")
		}
		return nil, types.NewInternalError("failed to create tool registration").WithCause(err)
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return nil, types.NewInternalError("created but failed to reload tool runtime").WithCause(err)
	}
	return row, nil
}

func (s *DefaultToolRegistryService) Update(ctx context.Context, id uint, req updateToolRegistrationRequest) (*hosted.ToolRegistration, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool runtime is not configured")
	}

	// T-004: GetByID+Update run atomically inside WithTransaction to avoid TOCTOU race.
	var updatedRow hosted.ToolRegistration
	err := s.store.WithTransaction(ctx, func(txStore hosted.ToolRegistryStore) error {
		row, err := txStore.GetByID(id)
		if err != nil {
			return err
		}

		updates := map[string]any{}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if err := validateToolRegistrationName(name); err != nil {
				return err
			}
			if err := s.validateAliasName(name); err != nil {
				return err
			}
			updates["name"] = name
		}
		if req.Description != nil {
			updates["description"] = strings.TrimSpace(*req.Description)
		}
		if req.Target != nil {
			target := strings.TrimSpace(*req.Target)
			if err := s.validateTarget(target); err != nil {
				return err
			}
			updates["target"] = target
		}
		nextName := row.Name
		if v, ok := updates["name"].(string); ok {
			nextName = v
		}
		nextTarget := row.Target
		if v, ok := updates["target"].(string); ok {
			nextTarget = v
		}
		if nextName == nextTarget {
			return types.NewError(types.ErrInvalidRequest, "name and target must be different")
		}
		if req.Parameters != nil {
			if err := validateToolRegistrationParameters(*req.Parameters); err != nil {
				return err
			}
			updates["parameters"] = *req.Parameters
		}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
		if len(updates) == 0 {
			return types.NewError(types.ErrInvalidRequest, "no fields to update")
		}

		if err := txStore.Update(&row, updates); err != nil {
			return err
		}
		if err := txStore.Reload(&row); err != nil {
			return err
		}
		updatedRow = row
		return nil
	})
	if err != nil {
		if errors.Is(err, hosted.ErrNotFound) {
			return nil, types.NewNotFoundError("tool registration not found")
		}
		if te, ok := err.(*types.Error); ok {
			return nil, te
		}
		if isUniqueViolation(err) {
			return nil, types.NewError(types.ErrInvalidRequest, "tool name already exists")
		}
		return nil, types.NewInternalError("failed to update tool registration").WithCause(err)
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return nil, types.NewInternalError("updated but failed to reload tool runtime").WithCause(err)
	}
	return &updatedRow, nil
}

func (s *DefaultToolRegistryService) Delete(id uint) *types.Error {
	if s.runtime == nil {
		return types.NewInternalError("tool runtime is not configured")
	}
	rowsAffected, err := s.store.Delete(id)
	if err != nil {
		return types.NewInternalError("failed to delete tool registration").WithCause(err)
	}
	if rowsAffected == 0 {
		return types.NewNotFoundError("tool registration not found")
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return types.NewInternalError("deleted but failed to reload tool runtime").WithCause(err)
	}
	return nil
}

func (s *DefaultToolRegistryService) Reload() *types.Error {
	if s.runtime == nil {
		return types.NewInternalError("tool runtime is not configured")
	}
	if err := s.runtime.ReloadBindings(context.Background()); err != nil {
		return types.NewInternalError("failed to reload tool runtime").WithCause(err)
	}
	return nil
}

func (s *DefaultToolRegistryService) validateTarget(target string) *types.Error {
	if target == "" {
		return types.NewError(types.ErrInvalidRequest, "target is required")
	}
	targets := s.runtime.BaseToolNames()
	for _, t := range targets {
		if t == target {
			return nil
		}
	}
	return types.NewError(types.ErrInvalidRequest, "target must be one of runtime base tools")
}

func (s *DefaultToolRegistryService) validateAliasName(name string) *types.Error {
	targets := s.runtime.BaseToolNames()
	for _, target := range targets {
		if target == name {
			return types.NewError(types.ErrInvalidRequest, "name is reserved by runtime base tools")
		}
	}
	return nil
}

func validateToolRegistrationName(name string) *types.Error {
	if name == "" {
		return types.NewError(types.ErrInvalidRequest, "name is required")
	}
	if !toolRegistrationNamePattern.MatchString(name) {
		return types.NewError(types.ErrInvalidRequest, "name format is invalid")
	}
	return nil
}

func validateToolRegistrationParameters(raw json.RawMessage) *types.Error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if !json.Valid(raw) {
		return types.NewError(types.ErrInvalidRequest, "parameters must be valid JSON")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return types.NewError(types.ErrInvalidRequest, "parameters must be valid JSON")
	}
	if _, ok := v.(map[string]any); !ok {
		return types.NewError(types.ErrInvalidRequest, "parameters must be a JSON object")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique") ||
		strings.Contains(msg, "constraint")
}
