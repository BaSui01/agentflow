package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLMTraceAttrs_UsesCanonicalKeys(t *testing.T) {
	attrs := LLMTraceAttrs("openai", "gpt-4o", "tenant-1", "user-1", "chat", "trace-1")

	got := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		got[string(attr.Key)] = attr.Value.AsString()
	}

	assert.Equal(t, "openai", got["llm.provider"])
	assert.Equal(t, "gpt-4o", got["llm.model"])
	assert.Equal(t, "tenant-1", got["tenant.id"])
	assert.Equal(t, "user-1", got["user.id"])
	assert.Equal(t, "chat", got["llm.feature"])
	assert.Equal(t, "trace-1", got["trace.id"])
	assert.NotContains(t, got, "provider")
	assert.NotContains(t, got, "tenant_id")
	assert.NotContains(t, got, "request.id")
}

func TestLLMTokenAttrs_UsesCanonicalTokenTypeKey(t *testing.T) {
	attrs := LLMTokenAttrs("openai", "gpt-4o", "tenant-1", "user-1", "chat", "prompt")

	got := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		got[string(attr.Key)] = attr.Value.AsString()
	}

	assert.Equal(t, "prompt", got["llm.token.type"])
	assert.NotContains(t, got, "type")
}
