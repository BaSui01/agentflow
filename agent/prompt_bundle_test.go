package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPromptBundleFromIdentity(t *testing.T) {
	b := NewPromptBundleFromIdentity("1.0.0", "test-agent")
	assert.Equal(t, "1.0.0", b.Version)
	assert.Equal(t, "test-agent", b.System.Identity)
}

func TestPromptBundle_IsZero(t *testing.T) {
	assert.True(t, PromptBundle{}.IsZero())
	assert.False(t, PromptBundle{Version: "1.0"}.IsZero())
	assert.False(t, PromptBundle{System: SystemPrompt{Role: "r"}}.IsZero())
	assert.False(t, PromptBundle{Constraints: []string{"c"}}.IsZero())
}

func TestPromptBundle_EffectiveVersion(t *testing.T) {
	assert.Equal(t, "1.0", PromptBundle{Version: "1.0"}.EffectiveVersion("2.0"))
	assert.Equal(t, "2.0", PromptBundle{}.EffectiveVersion("2.0"))
}

func TestSystemPrompt_IsZero(t *testing.T) {
	assert.True(t, SystemPrompt{}.IsZero())
	assert.False(t, SystemPrompt{Role: "r"}.IsZero())
	assert.False(t, SystemPrompt{Policies: []string{"p"}}.IsZero())
}

func TestFormatBulletSection(t *testing.T) {
	result := formatBulletSection("Title:", []string{"a", "b"})
	assert.Contains(t, result, "Title:")
	assert.Contains(t, result, "- a")
	assert.Contains(t, result, "- b")

	// Empty items
	assert.Equal(t, "", formatBulletSection("Title:", []string{"", "  "}))
}

func TestPromptBundle_RenderWithVars(t *testing.T) {
	b := PromptBundle{
		Version: "1.0",
		System: SystemPrompt{
			Role:        "Hello {{name}}",
			Identity:    "{{role}}",
			Policies:    []string{"policy for {{name}}"},
			OutputRules: []string{"rule {{x}}"},
			Prohibits:   []string{"no {{y}}"},
		},
		Examples: []Example{
			{User: "Hi {{name}}", Assistant: "Hello {{name}}"},
		},
		Constraints: []string{"constraint {{name}}"},
	}

	vars := map[string]string{"name": "Alice", "role": "admin", "x": "1", "y": "2"}
	rendered := b.RenderWithVars(vars)

	assert.Equal(t, "Hello Alice", rendered.System.Role)
	assert.Equal(t, "admin", rendered.System.Identity)
	assert.Equal(t, "policy for Alice", rendered.System.Policies[0])
	assert.Equal(t, "rule 1", rendered.System.OutputRules[0])
	assert.Equal(t, "no 2", rendered.System.Prohibits[0])
	assert.Equal(t, "Hi Alice", rendered.Examples[0].User)
	assert.Equal(t, "Hello Alice", rendered.Examples[0].Assistant)
	assert.Equal(t, "constraint Alice", rendered.Constraints[0])

	// No vars returns same bundle
	same := b.RenderWithVars(nil)
	assert.Equal(t, b.System.Role, same.System.Role)
}

func TestPromptBundle_ExtractVariables(t *testing.T) {
	b := PromptBundle{
		System: SystemPrompt{
			Role:     "Hello {{name}}",
			Identity: "{{role}}",
			Policies: []string{"{{name}} policy"},
		},
		Examples: []Example{
			{User: "{{question}}", Assistant: "{{answer}}"},
		},
		Constraints: []string{"{{limit}}"},
	}

	vars := b.ExtractVariables()
	require.NotEmpty(t, vars)
	assert.Contains(t, vars, "name")
	assert.Contains(t, vars, "role")
	assert.Contains(t, vars, "question")
	assert.Contains(t, vars, "answer")
	assert.Contains(t, vars, "limit")
}

func TestPromptBundle_RenderExamplesAsMessages(t *testing.T) {
	b := PromptBundle{
		Examples: []Example{
			{User: "Hello", Assistant: "Hi there"},
			{User: "Bye", Assistant: "Goodbye"},
		},
	}

	msgs := b.RenderExamplesAsMessages()
	require.Len(t, msgs, 4)
	assert.Equal(t, "user", string(msgs[0].Role))
	assert.Equal(t, "Hello", msgs[0].Content)
	assert.Equal(t, "assistant", string(msgs[1].Role))
	assert.Equal(t, "Hi there", msgs[1].Content)

	// Empty examples
	empty := PromptBundle{}
	assert.Nil(t, empty.RenderExamplesAsMessages())
}

func TestPromptBundle_RenderExamplesAsMessagesWithVars(t *testing.T) {
	b := PromptBundle{
		Examples: []Example{
			{User: "Hello {{name}}", Assistant: "Hi {{name}}"},
		},
	}

	msgs := b.RenderExamplesAsMessagesWithVars(map[string]string{"name": "Alice"})
	require.Len(t, msgs, 2)
	assert.Equal(t, "Hello Alice", msgs[0].Content)
	assert.Equal(t, "Hi Alice", msgs[1].Content)

	// No vars
	msgs2 := b.RenderExamplesAsMessagesWithVars(nil)
	require.Len(t, msgs2, 2)
	assert.Equal(t, "Hello {{name}}", msgs2[0].Content)

	// Empty examples
	assert.Nil(t, PromptBundle{}.RenderExamplesAsMessagesWithVars(nil))
}

func TestPromptBundle_HasExamples(t *testing.T) {
	assert.False(t, PromptBundle{}.HasExamples())
	assert.True(t, PromptBundle{Examples: []Example{{User: "a"}}}.HasExamples())
}

func TestPromptBundle_AppendExamples(t *testing.T) {
	b := &PromptBundle{}
	b.AppendExamples(Example{User: "a", Assistant: "b"})
	assert.Len(t, b.Examples, 1)
	b.AppendExamples(Example{User: "c", Assistant: "d"}, Example{User: "e", Assistant: "f"})
	assert.Len(t, b.Examples, 3)
}

