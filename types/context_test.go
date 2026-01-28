package types

import (
	"context"
	"testing"
)

func TestContextHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ctx = WithTraceID(ctx, "t1")
	if got, ok := TraceID(ctx); !ok || got != "t1" {
		t.Fatalf("TraceID mismatch: %v %v", got, ok)
	}

	ctx = WithTenantID(ctx, "tenant")
	if got, ok := TenantID(ctx); !ok || got != "tenant" {
		t.Fatalf("TenantID mismatch: %v %v", got, ok)
	}

	ctx = WithUserID(ctx, "user")
	if got, ok := UserID(ctx); !ok || got != "user" {
		t.Fatalf("UserID mismatch: %v %v", got, ok)
	}

	ctx = WithRunID(ctx, "run")
	if got, ok := RunID(ctx); !ok || got != "run" {
		t.Fatalf("RunID mismatch: %v %v", got, ok)
	}

	ctx = WithLLMModel(ctx, "gpt-4o")
	if got, ok := LLMModel(ctx); !ok || got != "gpt-4o" {
		t.Fatalf("LLMModel mismatch: %v %v", got, ok)
	}

	ctx = WithPromptBundleVersion(ctx, "v1")
	if got, ok := PromptBundleVersion(ctx); !ok || got != "v1" {
		t.Fatalf("PromptBundleVersion mismatch: %v %v", got, ok)
	}
}
