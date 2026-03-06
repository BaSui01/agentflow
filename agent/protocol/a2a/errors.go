package a2a

import (
	"net/http"

	"github.com/BaSui01/agentflow/types"
)

// 代理卡验证错误（映射到 types.ErrInvalidRequest）.
var (
	ErrMissingName        = types.NewError(types.ErrInvalidRequest, "agent card: missing name").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMissingDescription = types.NewError(types.ErrInvalidRequest, "agent card: missing description").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMissingURL        = types.NewError(types.ErrInvalidRequest, "agent card: missing url").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMissingVersion     = types.NewError(types.ErrInvalidRequest, "agent card: missing version").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
)

// A2A 协议错误.
var (
	ErrAgentNotFound    = types.NewError(types.ErrAgentNotFound, "a2a: agent not found").WithHTTPStatus(http.StatusNotFound).WithRetryable(false)
	ErrRemoteUnavailable = types.NewError(types.ErrServiceUnavailable, "a2a: remote agent unavailable").WithHTTPStatus(http.StatusServiceUnavailable).WithRetryable(true)
	ErrAuthFailed       = types.NewError(types.ErrAuthentication, "a2a: authentication failed").WithHTTPStatus(http.StatusUnauthorized).WithRetryable(false)
	ErrInvalidMessage   = types.NewError(types.ErrInvalidRequest, "a2a: invalid message format").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
)

// A2A 信件验证错误（映射到 types.ErrInvalidRequest）.
var (
	ErrMessageMissingID        = types.NewError(types.ErrInvalidRequest, "a2a message: missing id").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMessageInvalidType      = types.NewError(types.ErrInvalidRequest, "a2a message: invalid type").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMessageMissingFrom      = types.NewError(types.ErrInvalidRequest, "a2a message: missing from").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMessageMissingTo        = types.NewError(types.ErrInvalidRequest, "a2a message: missing to").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
	ErrMessageMissingTimestamp = types.NewError(types.ErrInvalidRequest, "a2a message: missing timestamp").WithHTTPStatus(http.StatusBadRequest).WithRetryable(false)
)

// A2A 客户端错误.
var (
	ErrTaskNotReady = types.NewError(types.ErrTaskNotReady, "a2a: task not ready").WithHTTPStatus(http.StatusAccepted).WithRetryable(true)
	ErrTaskNotFound = types.NewError(types.ErrTaskNotFound, "a2a: task not found").WithHTTPStatus(http.StatusNotFound).WithRetryable(false)
)
