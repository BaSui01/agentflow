package prompt

import (
	"strings"
	"testing"
)

func TestPromptEnhancerAndOptimizer(t *testing.T) {
	bundle := PromptBundle{
		System: SystemPrompt{Identity: "You are a helper"},
		Examples: []Example{
			{User: "u1", Assistant: "a1"},
			{User: "u2", Assistant: "a2"},
		},
	}
	enhanced := NewPromptEnhancer(PromptEnhancerConfig{
		UseChainOfThought:   true,
		UseStructuredOutput: true,
		UseFewShot:          true,
		MaxExamples:         1,
		UseDelimiters:       true,
	}).EnhancePromptBundle(bundle)

	if len(enhanced.Examples) != 1 {
		t.Fatalf("few-shot examples not truncated: %d", len(enhanced.Examples))
	}
	prompt := enhanced.RenderSystemPrompt()
	for _, want := range []string{"一步步思考", "结构化", "用户输入可能使用"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("enhanced prompt missing %q:\n%s", want, prompt)
		}
	}

	userPrompt := NewPromptEnhancer(*DefaultPromptEnhancerConfig()).EnhanceUserPrompt("分析代码", "JSON")
	for _, want := range []string{"```", "一步步思考", "请按照以下格式输出：\nJSON"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("enhanced user prompt missing %q:\n%s", want, userPrompt)
		}
	}

	optimized := NewPromptOptimizer().OptimizePrompt("总结")
	for _, want := range []string{"任务：总结", "请提供详细的回答", "要求："} {
		if !strings.Contains(optimized, want) {
			t.Fatalf("optimized prompt missing %q:\n%s", want, optimized)
		}
	}
}

func TestPromptTemplateLibrary(t *testing.T) {
	lib := NewPromptTemplateLibrary()
	if _, ok := lib.GetTemplate("code_generation"); !ok {
		t.Fatal("default code_generation template should exist")
	}

	rendered, err := lib.RenderTemplate("code_generation", map[string]string{
		"language":    "Go",
		"requirement": "build a cache",
	})
	if err != nil {
		t.Fatalf("RenderTemplate returned error: %v", err)
	}
	if !strings.Contains(rendered, "Go") || !strings.Contains(rendered, "build a cache") {
		t.Fatalf("rendered template did not substitute variables:\n%s", rendered)
	}

	if _, renderErr := lib.RenderTemplate("code_generation", map[string]string{"language": "Go"}); renderErr == nil {
		t.Fatal("RenderTemplate should reject missing variables")
	}
	if _, renderErr := lib.RenderTemplate("missing", nil); renderErr == nil {
		t.Fatal("RenderTemplate should reject unknown templates")
	}

	lib.RegisterTemplate(PromptTemplate{Name: "custom", Template: "Hello {{.name}}", Variables: []string{"name"}})
	rendered, err = lib.RenderTemplate("custom", map[string]string{"name": "Ada"})
	if err != nil || rendered != "Hello Ada" {
		t.Fatalf("custom template rendered %q, err=%v", rendered, err)
	}
	if len(lib.ListTemplates()) < 6 {
		t.Fatalf("ListTemplates should include defaults and custom template")
	}
}
