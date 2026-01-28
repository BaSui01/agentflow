package types

import (
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
