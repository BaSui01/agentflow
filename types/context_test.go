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

	ctx = WithLLMProvider(ctx, "openai")
	if got, ok := LLMProvider(ctx); !ok || got != "openai" {
		t.Fatalf("LLMProvider mismatch: %v %v", got, ok)
	}

	ctx = WithLLMRoutePolicy(ctx, "balanced")
	if got, ok := LLMRoutePolicy(ctx); !ok || got != "balanced" {
		t.Fatalf("LLMRoutePolicy mismatch: %v %v", got, ok)
	}

	ctx = WithPromptBundleVersion(ctx, "v1")
	if got, ok := PromptBundleVersion(ctx); !ok || got != "v1" {
		t.Fatalf("PromptBundleVersion mismatch: %v %v", got, ok)
	}
}

func TestWithRolesCopiesInputSlice(t *testing.T) {
	roles := []string{"admin", "user"}
	ctx := WithRoles(context.Background(), roles)

	roles[0] = "hacker"

	got, ok := Roles(ctx)
	if !ok {
		t.Fatal("expected roles to exist")
	}
	if got[0] != "admin" {
		t.Fatalf("expected copied roles to remain unchanged, got %v", got)
	}
}

func TestRolesReturnsCopiedSlice(t *testing.T) {
	ctx := WithRoles(context.Background(), []string{"admin", "user"})

	got, ok := Roles(ctx)
	if !ok {
		t.Fatal("expected roles to exist")
	}
	got[0] = "hacker"

	got2, ok := Roles(ctx)
	if !ok {
		t.Fatal("expected roles to exist")
	}
	if got2[0] != "admin" {
		t.Fatalf("expected context roles to remain unchanged, got %v", got2)
	}
}
