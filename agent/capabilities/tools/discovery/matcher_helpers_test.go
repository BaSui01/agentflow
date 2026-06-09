package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapabilityMatchesSupportsExactPrefixAndContains(t *testing.T) {
	assert.True(t, CapabilityMatches("code_review", "code_review"))
	assert.True(t, CapabilityMatches("code_review_python", "code_review"))
	assert.True(t, CapabilityMatches("advanced_code_review_python", "code_review"))
	assert.False(t, CapabilityMatches("summarize", "code_review"))
}

func TestTokenizeForSemanticMatchFiltersStopWords(t *testing.T) {
	tokens := TokenizeForSemanticMatch("This is a Code Review task for Go_1")

	assert.Equal(t, []string{"code", "review", "task", "go_1"}, tokens)
}

func TestIsExcludedAgent(t *testing.T) {
	assert.True(t, IsExcludedAgent("agent-2", []string{"agent-1", "agent-2"}))
	assert.False(t, IsExcludedAgent("agent-3", []string{"agent-1", "agent-2"}))
}
