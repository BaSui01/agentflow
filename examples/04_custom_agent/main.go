package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yourusername/agentflow/agent"
	"github.com/yourusername/agentflow/llm"
	"github.com/yourusername/agentflow/providers"
	"github.com/yourusername/agentflow/providers/openai"
	"go.uber.org/zap"
)

// 定义自己的 Agent 类型（完全自定义）
const (
	TypeCodeReviewer  agent.AgentType = "code-reviewer"
	TypeDataAnalyst   agent.AgentType = "data-analyst"
	TypeStoryWriter   agent.AgentType = "story-writer"
	TypeMathTutor     agent.AgentType = "math-tutor"
	TypeProductManager agent.AgentType = "product-manager"
)

func main() {
	// 1. 创建 Logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 2. 配置 Provider
	cfg := providers.OpenAIConfig{
		APIKey:  "your-api-key-here",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := openai.NewOpenAIProvider(cfg, logger)

	// 3. 创建多个不同类型的自定义 Agent
	codeReviewer := createCodeReviewerAgent(provider, logger)
	dataAnalyst := createDataAnalystAgent(provider, logger)
	storyWriter := createStoryWriterAgent(provider, logger)

	ctx := context.Background()

	// 4. 使用代码审查 Agent
	fmt.Println("=== 代码审查 Agent ===")
	codeMessages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: codeReviewer.Config().PromptBundle.RenderSystemPrompt(),
		},
		{
			Role: llm.RoleUser,
			Content: `请审查这段代码:
func divide(a, b int) int {
    return a / b
}`,
		},
	}
	codeResp, err := codeReviewer.ChatCompletion(ctx, codeMessages)
	if err != nil {
		log.Printf("Code review failed: %v", err)
	} else {
		fmt.Printf("审查结果: %s\n\n", codeResp.Choices[0].Message.Content)
	}

	// 5. 使用数据分析 Agent
	fmt.Println("=== 数据分析 Agent ===")
	dataMessages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: dataAnalyst.Config().PromptBundle.RenderSystemPrompt(),
		},
		{
			Role:    llm.RoleUser,
			Content: "分析这组数据的趋势: [10, 15, 13, 17, 20, 25, 22, 28, 30]",
		},
	}
	dataResp, err := dataAnalyst.ChatCompletion(ctx, dataMessages)
	if err != nil {
		log.Printf("Data analysis failed: %v", err)
	} else {
		fmt.Printf("分析结果: %s\n\n", dataResp.Choices[0].Message.Content)
	}

	// 6. 使用故事创作 Agent
	fmt.Println("=== 故事创作 Agent ===")
	storyMessages := []llm.Message{
		{
			Role:    llm.RoleSystem,
			Content: storyWriter.Config().PromptBundle.RenderSystemPrompt(),
		},
		{
			Role:    llm.RoleUser,
			Content: "写一个关于时间旅行的短篇故事开头",
		},
	}
	storyResp, err := storyWriter.ChatCompletion(ctx, storyMessages)
	if err != nil {
		log.Printf("Story writing failed: %v", err)
	} else {
		fmt.Printf("故事开头: %s\n", storyResp.Choices[0].Message.Content)
	}
}

// createCodeReviewerAgent 创建代码审查 Agent
func createCodeReviewerAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
	cfg := agent.Config{
		ID:          "code-reviewer-001",
		Name:        "代码审查专家",
		Type:        TypeCodeReviewer, // 自定义类型
		Description: "专业的代码审查 AI，检查代码质量、安全性和最佳实践",
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.3, // 低温度，更严谨
		PromptBundle: agent.PromptBundle{
			Version: "1.0",
			System: agent.SystemPrompt{
				Identity: "你是一位资深的代码审查专家，拥有 15 年的软件开发经验。",
				Policies: []string{
					"仔细检查代码的正确性和健壮性",
					"识别潜在的 bug、安全漏洞和性能问题",
					"提供具体的改进建议和最佳实践",
					"使用友好但专业的语气",
				},
				OutputRules: []string{
					"按照：问题描述 -> 严重程度 -> 建议修复 的格式输出",
					"使用 Markdown 格式",
					"如果代码没有问题，给予肯定并说明优点",
				},
			},
		},
	}

	return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}

// createDataAnalystAgent 创建数据分析 Agent
func createDataAnalystAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
	cfg := agent.Config{
		ID:          "data-analyst-001",
		Name:        "数据分析师",
		Type:        TypeDataAnalyst, // 自定义类型
		Description: "专业的数据分析 AI，擅长数据解读和趋势分析",
		Model:       "gpt-4",
		MaxTokens:   1500,
		Temperature: 0.5,
		PromptBundle: agent.PromptBundle{
			Version: "1.0",
			System: agent.SystemPrompt{
				Identity: "你是一位经验丰富的数据分析师，擅长从数据中发现洞察。",
				Policies: []string{
					"使用统计学方法分析数据",
					"识别数据中的模式和趋势",
					"提供可操作的业务建议",
					"用清晰的语言解释复杂的数据概念",
				},
				OutputRules: []string{
					"先总结关键发现",
					"然后详细分析",
					"最后给出建议",
					"使用图表描述（文字描述）",
				},
			},
		},
	}

	return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}

// createStoryWriterAgent 创建故事创作 Agent
func createStoryWriterAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
	cfg := agent.Config{
		ID:          "story-writer-001",
		Name:        "故事作家",
		Type:        TypeStoryWriter, // 自定义类型
		Description: "富有创意的故事创作 AI，擅长编写引人入胜的故事",
		Model:       "gpt-4",
		MaxTokens:   3000,
		Temperature: 0.9, // 高温度，更有创意
		PromptBundle: agent.PromptBundle{
			Version: "1.0",
			System: agent.SystemPrompt{
				Identity: "你是一位才华横溢的故事作家，擅长创作引人入胜的故事。",
				Policies: []string{
					"使用生动的描写和细节",
					"创造有趣的人物和情节",
					"保持故事的节奏和张力",
					"使用富有感染力的语言",
				},
				OutputRules: []string{
					"使用第三人称叙述",
					"每段 100-200 字",
					"注重场景描写和人物刻画",
				},
			},
		},
	}

	return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}
