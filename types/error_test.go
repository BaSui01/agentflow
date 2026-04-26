package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestError_ChainingAndHelpers(t *testing.T) {
	t.Parallel()

	root := errors.New("root")
	err := NewError(ErrUpstreamError, "upstream failed").
		WithCause(root).
		WithHTTPStatus(502).
		WithRetryable(true).
		WithProvider("openai")

	if GetErrorCode(err) != ErrUpstreamError {
		t.Fatalf("expected code %s, got %s", ErrUpstreamError, GetErrorCode(err))
	}
	if !IsRetryable(err) {
		t.Fatalf("expected retryable")
	}
	if !errors.Is(err, root) {
		t.Fatalf("expected errors.Is unwrap to root")
	}
	if got := err.Error(); got == "" {
		t.Fatalf("expected non-empty error string")
	}
}

func TestErrorContext_Chaining(t *testing.T) {
	t.Parallel()

	ec := ErrorContext{
		TraceID:   "trace-123",
		AgentID:   "agent-456",
		SessionID: "session-789",
		RunID:     "run-000",
	}

	err := NewError(ErrAuthzDenied, "denied").WithContext(ec)
	if err.Context.TraceID != "trace-123" {
		t.Fatalf("expected trace-123, got %s", err.Context.TraceID)
	}
	if err.Context.AgentID != "agent-456" {
		t.Fatalf("expected agent-456, got %s", err.Context.AgentID)
	}
}

func TestErrorContext_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	ec := ErrorContext{TraceID: "t1", AgentID: "a1", SessionID: "s1", RunID: "r1"}
	err := NewError(ErrToolPermissionDenied, "no access").WithContext(ec)

	data, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatalf("marshal failed: %v", jerr)
	}

	var decoded Error
	if jerr = json.Unmarshal(data, &decoded); jerr != nil {
		t.Fatalf("unmarshal failed: %v", jerr)
	}
	if decoded.Context.TraceID != "t1" {
		t.Fatalf("expected t1, got %s", decoded.Context.TraceID)
	}
	if decoded.Code != ErrToolPermissionDenied {
		t.Fatalf("expected %s, got %s", ErrToolPermissionDenied, decoded.Code)
	}
}

func TestNewErrorCodes_NoConflict(t *testing.T) {
	t.Parallel()

	codes := []ErrorCode{
		ErrAuthzServiceUnavailable, ErrAuthzDenied, ErrAuthzMissingContext,
		ErrApprovalExpired, ErrApprovalPending,
		ErrToolInvalidArgs, ErrToolPermissionDenied, ErrToolExecutionTimeout, ErrToolValidationError,
		ErrCheckpointSaveFailed, ErrCheckpointIntegrityError,
		ErrRuntimeAborted, ErrRuntimeMiddlewareError, ErrRuntimeMiddlewareTimeout,
		ErrWorkflowNodeFailed, ErrWorkflowSuspended,
	}
	seen := make(map[ErrorCode]string)
	for _, c := range codes {
		if prev, ok := seen[c]; ok {
			t.Fatalf("duplicate error code %s (also seen as %s)", c, prev)
		}
		seen[c] = string(c)
	}
}

func TestRetryable_Classification(t *testing.T) {
	t.Parallel()

	retryable := []struct {
		name string
		err  *Error
	}{
		{"RateLimit", NewRateLimitError("rl")},
		{"QuotaExceeded", NewError(ErrQuotaExceeded, "qe").WithRetryable(true)},
		{"UpstreamTimeout", NewError(ErrUpstreamTimeout, "ut").WithRetryable(true)},
		{"Timeout", NewTimeoutError("t")},
		{"ServiceUnavailable", NewServiceUnavailableError("su")},
		{"ProviderUnavailable", NewError(ErrProviderUnavailable, "pu").WithRetryable(true)},
		{"ToolExecutionTimeout", NewToolExecutionTimeoutError("tet")},
		{"AuthzServiceUnavailable", NewAuthzServiceUnavailableError("asu")},
		{"CheckpointSaveFailed", NewCheckpointSaveFailedError("csf")},
		{"RuntimeMiddlewareTimeout", NewRuntimeMiddlewareTimeoutError("rmt")},
	}
	for _, tc := range retryable {
		if !IsRetryable(tc.err) {
			t.Errorf("%s: expected retryable", tc.name)
		}
	}

	nonRetryable := []struct {
		name string
		err  *Error
	}{
		{"AuthzDenied", NewAuthzDeniedError("ad")},
		{"ToolPermissionDenied", NewToolPermissionDeniedError("tpd")},
		{"ToolValidationError", NewToolValidationError("tve")},
		{"CheckpointIntegrityError", NewCheckpointIntegrityError("cie")},
		{"RuntimeAborted", NewRuntimeAbortedError("ra")},
		{"RuntimeMiddlewareError", NewRuntimeMiddlewareError("rme")},
		{"WorkflowNodeFailed", NewWorkflowNodeFailedError("wnf")},
	}
	for _, tc := range nonRetryable {
		if IsRetryable(tc.err) {
			t.Errorf("%s: expected non-retryable", tc.name)
		}
	}
}

func TestNewErrorConstructors(t *testing.T) {
	t.Parallel()

	err := NewAuthzDeniedError("access denied")
	if err.Code != ErrAuthzDenied || err.HTTPStatus != 403 {
		t.Fatalf("unexpected authz denied error: %+v", err)
	}

	err2 := NewToolPermissionDeniedError("no exec")
	if err2.Code != ErrToolPermissionDenied || err2.HTTPStatus != 403 {
		t.Fatalf("unexpected tool permission error: %+v", err2)
	}

	err3 := NewWorkflowSuspendedError("paused")
	if err3.Code != ErrWorkflowSuspended || err3.HTTPStatus != 202 {
		t.Fatalf("unexpected workflow suspended error: %+v", err3)
	}
}

