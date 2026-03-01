package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Error type ---

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "with base error",
			err:      NewError(ErrCodeProviderNotSet, "provider not set"),
			expected: "[AGENT_PROVIDER_NOT_SET] provider not set",
		},
		{
			name:     "nil base returns unknown",
			err:      &Error{},
			expected: "[UNKNOWN] agent error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := NewErrorWithCause(ErrCodeExecutionFailed, "exec failed", cause)
	assert.ErrorIs(t, err, cause)

	nilBase := &Error{}
	assert.Nil(t, nilBase.Unwrap())
}

func TestNewErrorWithCause(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := NewErrorWithCause(ErrCodeExecutionFailed, "exec failed", cause)
	require.NotNil(t, err)
	assert.Equal(t, ErrCodeExecutionFailed, err.Base.Code)
	assert.Contains(t, err.Error(), "exec failed")
	assert.NotNil(t, err.Metadata)
	assert.False(t, err.Timestamp.IsZero())
}

func TestError_WithAgent(t *testing.T) {
	err := NewError(ErrCodeNotReady, "not ready").
		WithAgent("agent-1", TypeAssistant)
	assert.Equal(t, "agent-1", err.AgentID)
	assert.Equal(t, TypeAssistant, err.AgentType)
}

func TestError_WithRetryable(t *testing.T) {
	err := NewError(ErrCodeTimeout, "timeout").WithRetryable(true)
	assert.True(t, err.Base.Retryable)

	err2 := NewError(ErrCodeInvalidConfig, "bad config").WithRetryable(false)
	assert.False(t, err2.Base.Retryable)
}

func TestError_WithMetadata(t *testing.T) {
	err := NewError(ErrCodeToolNotFound, "tool not found").
		WithMetadata("tool_name", "calculator").
		WithMetadata("agent_id", "a1")
	assert.Equal(t, "calculator", err.Metadata["tool_name"])
	assert.Equal(t, "a1", err.Metadata["agent_id"])
}

func TestError_WithMetadata_NilMap(t *testing.T) {
	err := &Error{Base: NewError(ErrCodeToolNotFound, "x").Base}
	err.Metadata = nil
	err = err.WithMetadata("key", "val")
	assert.Equal(t, "val", err.Metadata["key"])
}

func TestError_WithCause(t *testing.T) {
	cause := fmt.Errorf("underlying")
	err := NewError(ErrCodeExecutionFailed, "failed").WithCause(cause)
	assert.Equal(t, cause, err.Base.Cause)
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "retryable agent error",
			err:      NewError(ErrCodeTimeout, "timeout").WithRetryable(true),
			expected: true,
		},
		{
			name:     "non-retryable agent error",
			err:      NewError(ErrCodeInvalidConfig, "bad"),
			expected: false,
		},
		{
			name:     "non-agent error",
			err:      fmt.Errorf("generic error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRetryable(tt.err))
		})
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{
			name:     "agent error",
			err:      NewError(ErrCodeProviderNotSet, "no provider"),
			expected: ErrCodeProviderNotSet,
		},
		{
			name:     "non-agent error",
			err:      fmt.Errorf("generic"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetErrorCode(tt.err))
		})
	}
}

func TestErrInvalidTransition_Error(t *testing.T) {
	err := ErrInvalidTransition{From: StateReady, To: StateInit}
	assert.Contains(t, err.Error(), "invalid state transition")
	assert.Contains(t, err.Error(), string(StateReady))
	assert.Contains(t, err.Error(), string(StateInit))
}

func TestErrInvalidTransition_ToAgentError(t *testing.T) {
	err := ErrInvalidTransition{From: StateReady, To: StateInit}
	agentErr := err.ToAgentError()
	require.NotNil(t, agentErr)
	assert.Equal(t, ErrCodeInvalidTransition, agentErr.Base.Code)
	assert.Equal(t, StateReady, agentErr.Metadata["from_state"])
	assert.Equal(t, StateInit, agentErr.Metadata["to_state"])
}

func TestPredefinedErrors(t *testing.T) {
	assert.NotNil(t, ErrProviderNotSet)
	assert.NotNil(t, ErrAgentNotReady)
	assert.NotNil(t, ErrAgentBusy)

	assert.True(t, errors.Is(ErrProviderNotSet, ErrProviderNotSet))
}

