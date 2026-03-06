package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ChainStep struct {
	ToolName   string
	Args       map[string]any
	ArgMapping map[string]string
	OnError    string
	MaxRetries int
}

type ToolChain struct {
	Name  string
	Steps []ChainStep
}

type ChainExecutor struct {
	registry ToolRegistryLike
	config   ParallelConfig
}

type ToolRegistryLike interface {
	Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
}

func NewChainExecutor(registry ToolRegistryLike, config ParallelConfig) *ChainExecutor {
	return &ChainExecutor{registry: registry, config: config}
}

func (e *ChainExecutor) ExecuteChain(ctx context.Context, chain ToolChain, initialInput map[string]any) (*ChainResult, error) {
	start := time.Now()
	result := &ChainResult{Steps: make([]ChainStepResult, 0, len(chain.Steps))}
	var prevRaw json.RawMessage
	if len(initialInput) > 0 {
		var err error
		prevRaw, err = json.Marshal(initialInput)
		if err != nil {
			return nil, fmt.Errorf("marshal initial input: %w", err)
		}
	}

	for i, step := range chain.Steps {
		args := make(map[string]any)
		for k, v := range step.Args {
			args[k] = v
		}
		for paramName, path := range step.ArgMapping {
			if prevRaw != nil {
				val, err := jsonPathGet(prevRaw, path)
				if err == nil {
					args[paramName] = val
				}
			}
		}
		argsBytes, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("step %d marshal args: %w", i+1, err)
		}

		stepStart := time.Now()
		out, execErr := e.executeStepWithRetry(ctx, step, argsBytes)
		stepDur := time.Since(stepStart)

		sr := ChainStepResult{
			ToolName: step.ToolName,
			Output:   out,
			Duration: stepDur,
			Error:    execErr,
		}
		if execErr != nil {
			switch step.OnError {
			case "skip":
				sr.Skipped = true
				sr.Error = nil
				prevRaw = nil
			case "retry":
				for r := 0; r < step.MaxRetries && execErr != nil; r++ {
					out, execErr = e.registry.Execute(ctx, step.ToolName, argsBytes)
					if execErr == nil {
						sr.Output = out
						sr.Error = nil
						sr.Skipped = false
						break
					}
				}
				if execErr != nil {
					sr.Error = execErr
					return &ChainResult{Steps: result.Steps, Duration: time.Since(start)}, execErr
				}
			default:
				result.Steps = append(result.Steps, sr)
				result.Duration = time.Since(start)
				return result, execErr
			}
		}
		if sr.Output != nil {
			prevRaw = sr.Output
		} else {
			prevRaw = nil
		}
		result.Steps = append(result.Steps, sr)
	}

	result.FinalOutput = prevRaw
	result.Duration = time.Since(start)
	return result, nil
}

func (e *ChainExecutor) executeStepWithRetry(ctx context.Context, step ChainStep, args json.RawMessage) (json.RawMessage, error) {
	maxAttempts := 1
	if step.MaxRetries > 0 {
		maxAttempts = step.MaxRetries + 1
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, err := e.registry.Execute(ctx, step.ToolName, args)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt < maxAttempts-1 && e.config.RetryDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(e.config.RetryDelay):
			}
		}
	}
	return nil, lastErr
}

func jsonPathGet(data json.RawMessage, path string) (any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}
	if path == "$" || path == "." {
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	path = strings.TrimPrefix(path, "$.")
	parts := strings.Split(path, ".")
	var current any
	if err := json.Unmarshal(data, &current); err != nil {
		return nil, err
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch v := current.(type) {
		case map[string]any:
			next, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", path, part)
			}
			current = next
		case []any:
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err != nil || idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("path %q: invalid index %q", path, part)
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("path %q: cannot index %T", path, current)
		}
	}
	return current, nil
}

type ChainResult struct {
	Steps       []ChainStepResult
	FinalOutput json.RawMessage
	Duration    time.Duration
}

type ChainStepResult struct {
	ToolName string
	Output   json.RawMessage
	Duration time.Duration
	Error    error
	Skipped  bool
}

func RegistryAsChainExecutor(r ToolRegistry) ToolRegistryLike {
	return &registryChainAdapter{r: r}
}

type registryChainAdapter struct {
	r ToolRegistry
}

func (a *registryChainAdapter) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	fn, _, err := a.r.Get(name)
	if err != nil {
		return nil, err
	}
	return fn(ctx, args)
}
