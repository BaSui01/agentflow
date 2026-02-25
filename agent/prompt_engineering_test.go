package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// PromptEnhancerConfig tests
// ============================================================

func TestDefaultPromptEnhancerConfig_Values(t *testing.T) {
	cfg := DefaultPromptEnhancerConfig()
	assert.True(t, cfg.UseChainOfThought)
	assert.False(t, cfg.UseSelfConsistency)
	assert.True(t, cfg.UseStructuredOutput)
	assert.True(t, cfg.UseFewShot)
	assert.Equal(t, 3, cfg.MaxExamples)
	assert.True(t, cfg.UseDelimiters)
}

func TestDefaultPromptEnhancerConfig(t *testing.T) {
	cfg := DefaultPromptEnhancerConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.UseChainOfThought)
}

// ============================================================
// PromptEnhancer tests
// ============================================================

func TestPromptEnhancer_EnhancePromptBundle_ChainOfThought(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseChainOfThought: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{Identity: "You are a helper"},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	assert.Contains(t, enhanced.System.OutputRules, "在回答问题时，请一步步思考，展示你的推理过程")
}

func TestPromptEnhancer_EnhancePromptBundle_SkipCoTIfPresent(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseChainOfThought: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{Identity: "Think step by step"},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	// Should not add CoT rule since identity already contains "step by step"
	assert.Empty(t, enhanced.System.OutputRules)
}

func TestPromptEnhancer_EnhancePromptBundle_StructuredOutput(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseStructuredOutput: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{Identity: "Helper"},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	require.Len(t, enhanced.System.OutputRules, 1)
	assert.Contains(t, enhanced.System.OutputRules[0], "结构化")
}

func TestPromptEnhancer_EnhancePromptBundle_StructuredOutputSkipIfPresent(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseStructuredOutput: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{
			Identity:    "Helper",
			OutputRules: []string{"Use proper format for output"},
		},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	// Should not add another structure rule since "format" is already present
	assert.Len(t, enhanced.System.OutputRules, 1)
}

func TestPromptEnhancer_EnhancePromptBundle_FewShotLimit(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseFewShot:  true,
		MaxExamples: 2,
	})

	bundle := PromptBundle{
		Examples: []Example{
			{User: "q1", Assistant: "a1"},
			{User: "q2", Assistant: "a2"},
			{User: "q3", Assistant: "a3"},
		},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	assert.Len(t, enhanced.Examples, 2)
}

func TestPromptEnhancer_EnhancePromptBundle_Delimiters(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseDelimiters: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{Identity: "Helper"},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	assert.Contains(t, enhanced.System.Identity, "```")
}

func TestPromptEnhancer_EnhancePromptBundle_DelimitersSkipIfPresent(t *testing.T) {
	enhancer := NewPromptEnhancer(PromptEnhancerConfig{
		UseDelimiters: true,
	})

	bundle := PromptBundle{
		System: SystemPrompt{Identity: "Use ``` for code blocks"},
	}
	enhanced := enhancer.EnhancePromptBundle(bundle)
	// Should not add delimiter note since ``` already present
	assert.Equal(t, "Use ``` for code blocks", enhanced.System.Identity)
}

func TestPromptEnhancer_EnhanceUserPrompt(t *testing.T) {
	tests := []struct {
		name         string
		config       PromptEnhancerConfig
		prompt       string
		outputFormat string
		checkFn      func(t *testing.T, result string)
	}{
		{
			name:   "delimiters added",
			config: PromptEnhancerConfig{UseDelimiters: true},
			prompt: "hello world",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "```")
			},
		},
		{
			name:   "delimiters not added if already present",
			config: PromptEnhancerConfig{UseDelimiters: true},
			prompt: "```\nhello\n```",
			checkFn: func(t *testing.T, result string) {
				// Should not double-wrap
				assert.Equal(t, "```\nhello\n```", result)
			},
		},
		{
			name:   "chain of thought added",
			config: PromptEnhancerConfig{UseChainOfThought: true},
			prompt: "solve this",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "一步步思考")
			},
		},
		{
			name:   "chain of thought skipped if present",
			config: PromptEnhancerConfig{UseChainOfThought: true},
			prompt: "solve this step by step",
			checkFn: func(t *testing.T, result string) {
				// Should not add another CoT prompt
				assert.Equal(t, "solve this step by step", result)
			},
		},
		{
			name:         "structured output format appended",
			config:       PromptEnhancerConfig{UseStructuredOutput: true},
			prompt:       "analyze this",
			outputFormat: "JSON",
			checkFn: func(t *testing.T, result string) {
				assert.Contains(t, result, "JSON")
			},
		},
		{
			name:         "structured output not appended without format",
			config:       PromptEnhancerConfig{UseStructuredOutput: true},
			prompt:       "analyze this",
			outputFormat: "",
			checkFn: func(t *testing.T, result string) {
				assert.Equal(t, "analyze this", result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enhancer := NewPromptEnhancer(tt.config)
			result := enhancer.EnhanceUserPrompt(tt.prompt, tt.outputFormat)
			tt.checkFn(t, result)
		})
	}
}

// ============================================================
// PromptOptimizer tests
// ============================================================

func TestPromptOptimizer_OptimizePrompt_ShortPrompt(t *testing.T) {
	optimizer := NewPromptOptimizer()
	result := optimizer.OptimizePrompt("hi")
	assert.Contains(t, result, "任务")
	assert.Contains(t, result, "hi")
}

func TestPromptOptimizer_OptimizePrompt_WithTaskDescription(t *testing.T) {
	optimizer := NewPromptOptimizer()
	result := optimizer.OptimizePrompt("请帮我分析这段代码的性能问题")
	// Should not add task description since "请" is present
	assert.Contains(t, result, "请帮我分析")
}

func TestPromptOptimizer_OptimizePrompt_WithConstraints(t *testing.T) {
	optimizer := NewPromptOptimizer()
	result := optimizer.OptimizePrompt("请分析这段代码，不要修改原始逻辑")
	// Should not add basic constraints since "不要" is present
	assert.NotContains(t, result, "要求：\n-")
}

func TestPromptOptimizer_OptimizePrompt_NoConstraints(t *testing.T) {
	optimizer := NewPromptOptimizer()
	result := optimizer.OptimizePrompt("请分析这段代码的性能问题")
	// Should add basic constraints
	assert.Contains(t, result, "要求")
}

func TestPromptOptimizer_HasTaskDescription(t *testing.T) {
	optimizer := NewPromptOptimizer()
	assert.True(t, optimizer.hasTaskDescription("please help me"))
	assert.True(t, optimizer.hasTaskDescription("请帮我"))
	assert.True(t, optimizer.hasTaskDescription("I need this"))
	assert.False(t, optimizer.hasTaskDescription("hello world"))
}

func TestPromptOptimizer_HasConstraints(t *testing.T) {
	optimizer := NewPromptOptimizer()
	assert.True(t, optimizer.hasConstraints("不要修改"))
	assert.True(t, optimizer.hasConstraints("you must do this"))
	assert.True(t, optimizer.hasConstraints("avoid errors"))
	assert.False(t, optimizer.hasConstraints("hello world"))
}

// ============================================================
// PromptTemplateLibrary tests
// ============================================================

func TestPromptTemplateLibrary_New(t *testing.T) {
	lib := NewPromptTemplateLibrary()
	templates := lib.ListTemplates()
	assert.NotEmpty(t, templates)
	// Should have default templates
	assert.GreaterOrEqual(t, len(templates), 5)
}

func TestPromptTemplateLibrary_GetTemplate(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	tmpl, ok := lib.GetTemplate("analysis")
	assert.True(t, ok)
	assert.Equal(t, "analysis", tmpl.Name)
	assert.NotEmpty(t, tmpl.Template)
	assert.NotEmpty(t, tmpl.Variables)

	_, ok = lib.GetTemplate("nonexistent")
	assert.False(t, ok)
}

func TestPromptTemplateLibrary_RenderTemplate(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	result, err := lib.RenderTemplate("qa", map[string]string{
		"question": "What is Go?",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "What is Go?")
}

func TestPromptTemplateLibrary_RenderTemplate_NotFound(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	_, err := lib.RenderTemplate("nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPromptTemplateLibrary_RenderTemplate_MissingVariable(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	_, err := lib.RenderTemplate("qa", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not provided")
}

func TestPromptTemplateLibrary_RegisterTemplate(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	custom := PromptTemplate{
		Name:      "custom",
		Template:  "Hello {{.name}}",
		Variables: []string{"name"},
	}
	lib.RegisterTemplate(custom)

	tmpl, ok := lib.GetTemplate("custom")
	assert.True(t, ok)
	assert.Equal(t, "custom", tmpl.Name)

	result, err := lib.RenderTemplate("custom", map[string]string{"name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World", result)
}

func TestPromptTemplateLibrary_RenderAllDefaults(t *testing.T) {
	lib := NewPromptTemplateLibrary()

	tests := []struct {
		name string
		vars map[string]string
	}{
		{"analysis", map[string]string{"subject": "code", "content": "func main(){}"}},
		{"summary", map[string]string{"content": "long text", "max_words": "100"}},
		{"code_generation", map[string]string{"language": "Go", "requirement": "HTTP server"}},
		{"qa", map[string]string{"question": "What is Go?"}},
		{"creative", map[string]string{"topic": "AI", "goal": "innovation", "count": "3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := lib.RenderTemplate(tt.name, tt.vars)
			require.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}
