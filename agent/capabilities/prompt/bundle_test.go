package prompt

import (
	"reflect"
	"strings"
	"testing"

	llm "github.com/BaSui01/agentflow/llm/core"
)

func TestPromptBundleRenderExtractAndExamples(t *testing.T) {
	bundle := PromptBundle{
		Version: " 1.0.0 ",
		System: SystemPrompt{
			Role:        " assistant ",
			Identity:    "You are {{company}} {{role}}",
			Policies:    []string{" answer in {{language}} ", ""},
			OutputRules: []string{"use {{format}}"},
			Prohibits:   []string{"never expose {{secret}}"},
		},
		Constraints: []string{" be concise ", ""},
		Examples: []Example{
			{User: "Hi {{name}}", Assistant: "Hello {{name}}"},
			{User: "  ", Assistant: "Only assistant"},
		},
	}

	vars := bundle.ExtractVariables()
	wantVars := []string{"company", "role", "language", "format", "secret", "name"}
	if !reflect.DeepEqual(vars, wantVars) {
		t.Fatalf("ExtractVariables() = %#v, want %#v", vars, wantVars)
	}

	rendered := bundle.RenderSystemPromptWithVars(map[string]string{
		"company":  "AgentFlow",
		"role":     "tester",
		"language": "Chinese",
		"format":   "JSON",
		"secret":   "keys",
	})
	for _, want := range []string{
		"assistant",
		"You are AgentFlow tester",
		"行为政策：\n- answer in Chinese",
		"输出规则：\n- use JSON",
		"禁止行为：\n- never expose keys",
		"额外约束：\n- be concise",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered)
		}
	}

	messages := bundle.RenderExamplesAsMessagesWithVars(map[string]string{"name": "Ada"})
	if len(messages) != 3 {
		t.Fatalf("RenderExamplesAsMessagesWithVars len = %d, want 3", len(messages))
	}
	if messages[0].Role != llm.RoleUser || messages[0].Content != "Hi Ada" {
		t.Fatalf("first example message = %#v", messages[0])
	}
	if messages[1].Role != llm.RoleAssistant || messages[1].Content != "Hello Ada" {
		t.Fatalf("second example message = %#v", messages[1])
	}
	if messages[2].Role != llm.RoleAssistant || messages[2].Content != "Only assistant" {
		t.Fatalf("third example message = %#v", messages[2])
	}
}

func TestPromptBundleEffectiveVersionAndAppendExamples(t *testing.T) {
	if NewPromptBundleFromIdentity(" v2 ", " agent ").EffectiveVersion("fallback") != "v2" {
		t.Fatal("NewPromptBundleFromIdentity should trim and preserve explicit version")
	}

	bundle := PromptBundle{}
	if !bundle.IsZero() {
		t.Fatal("empty bundle should be zero")
	}
	if got := bundle.EffectiveVersion(" default "); got != "default" {
		t.Fatalf("EffectiveVersion fallback = %q, want default", got)
	}

	bundle.AppendExamples(Example{User: "u", Assistant: "a"})
	if !bundle.HasExamples() || len(bundle.RenderExamplesAsMessages()) != 2 {
		t.Fatal("AppendExamples should make examples renderable")
	}
	if bundle.IsZero() {
		t.Fatal("bundle with examples should not be zero")
	}
}
