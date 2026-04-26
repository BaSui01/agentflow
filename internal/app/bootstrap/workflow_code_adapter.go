package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
)

const (
	workflowMaxInt = int64(1<<(strconv.IntSize-1) - 1)
	workflowMinInt = -workflowMaxInt - 1
)

type workflowCodeExecutionRequest struct {
	Language       string
	Code           string
	TimeoutSeconds int
}

type hostedCodeHandler struct {
	tool          *hosted.CodeExecTool
	authorization usecase.AuthorizationService
	policy        workflowCodeExecutionPolicy
}

func (h hostedCodeHandler) Execute(ctx context.Context, input core.StepInput) (map[string]any, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("workflow code tool is not configured")
	}

	req, policy, err := h.codeExecutionRequest(input)
	if err != nil {
		return nil, err
	}

	if err := authorizeWorkflowStep(ctx, h.authorization, workflowAuthorizationRequest(
		ctx,
		types.ResourceCodeExec,
		h.tool.Name(),
		types.ActionExecute,
		types.RiskExecution,
		h.authorizationContext(req, policy),
	)); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]any{
		"language":        req.Language,
		"code":            req.Code,
		"timeout_seconds": req.TimeoutSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal code execution request: %w", err)
	}

	raw, err := h.tool.Execute(ctx, payload)
	if err != nil {
		return nil, err
	}

	var output hosted.CodeExecOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode code execution output: %w", err)
	}

	return map[string]any{
		"stdout":    output.Stdout,
		"stderr":    output.Stderr,
		"exit_code": output.ExitCode,
		"duration":  output.Duration.String(),
	}, nil
}

func (h hostedCodeHandler) codeExecutionRequest(input core.StepInput) (workflowCodeExecutionRequest, workflowCodeExecutionPolicy, error) {
	policy := h.policy.normalized()
	language, _ := input.Data["language"].(string)
	if language == "" {
		language = "python"
	}
	code, _ := input.Data["code"].(string)
	if code == "" {
		return workflowCodeExecutionRequest{}, policy, fmt.Errorf("workflow code step requires input.code")
	}
	if len(code) > policy.MaxCodeBytes {
		return workflowCodeExecutionRequest{}, policy, fmt.Errorf("workflow code step input.code exceeds max size: %d > %d bytes", len(code), policy.MaxCodeBytes)
	}

	timeoutSeconds, err := workflowCodeTimeoutSeconds(input.Data["timeout_seconds"], policy.DefaultTimeout, policy.MaxTimeout)
	if err != nil {
		return workflowCodeExecutionRequest{}, policy, err
	}

	return workflowCodeExecutionRequest{Language: language, Code: code, TimeoutSeconds: timeoutSeconds}, policy, nil
}

func (h hostedCodeHandler) authorizationContext(req workflowCodeExecutionRequest, policy workflowCodeExecutionPolicy) map[string]any {
	return map[string]any{
		"arguments": map[string]any{
			"language":            req.Language,
			"code_bytes":          len(req.Code),
			"code_fingerprint":    workflowStringFingerprint(req.Code),
			"timeout_seconds":     req.TimeoutSeconds,
			"max_code_bytes":      policy.MaxCodeBytes,
			"max_timeout_seconds": int(policy.MaxTimeout.Seconds()),
			"max_output_bytes":    policy.MaxOutputBytes,
			"allowed_languages":   append([]string(nil), policy.AllowedLanguageTags...),
		},
		"metadata": map[string]string{
			"runtime":          "workflow",
			"hosted_tool_type": string(h.tool.Type()),
			"hosted_tool_risk": "requires_approval",
		},
	}
}

func workflowCodeTimeoutSeconds(value any, defaultTimeout, maxTimeout time.Duration) (int, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = defaultWorkflowCodeTimeoutSeconds * time.Second
	}
	if maxTimeout <= 0 {
		maxTimeout = defaultTimeout
	}

	defaultSeconds := int(defaultTimeout.Seconds())
	maxSeconds := int(maxTimeout.Seconds())
	if defaultSeconds > maxSeconds {
		defaultSeconds = maxSeconds
	}
	if value == nil {
		return defaultSeconds, nil
	}

	seconds, err := workflowIntegerSeconds(value)
	if err != nil {
		return 0, fmt.Errorf("workflow code step timeout_seconds must be an integer: %w", err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("workflow code step timeout_seconds must be positive")
	}
	if seconds > maxSeconds {
		return 0, fmt.Errorf("workflow code step timeout_seconds exceeds max: %d > %d", seconds, maxSeconds)
	}
	return seconds, nil
}

func workflowIntegerSeconds(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		if v > workflowMaxInt || v < workflowMinInt {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint:
		if uint64(v) > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		if uint64(v) > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint64:
		if v > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case float64:
		if math.Trunc(v) != v || v > float64(workflowMaxInt) || v < float64(workflowMinInt) {
			return 0, fmt.Errorf("value must be a whole number")
		}
		return int(v), nil
	case float32:
		f := float64(v)
		if math.Trunc(f) != f || f > float64(workflowMaxInt) || f < float64(workflowMinInt) {
			return 0, fmt.Errorf("value must be a whole number")
		}
		return int(v), nil
	case json.Number:
		i64, err := v.Int64()
		if err != nil {
			return 0, err
		}
		if i64 > workflowMaxInt || i64 < workflowMinInt {
			return 0, fmt.Errorf("value out of range")
		}
		return int(i64), nil
	default:
		return 0, fmt.Errorf("got %T", value)
	}
}
