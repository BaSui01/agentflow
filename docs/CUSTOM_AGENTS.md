# 自定义 Agent 开发指南

## 概述

AgentFlow 框架提供了灵活的 Agent 扩展机制。`AgentType` 是一个字符串类型，你可以定义任何自己的 Agent 类型，不受框架预定义类型的限制。

## Agent 类型系统

### 预定义类型（可选）

框架提供了一些常见的 Agent 类型作为参考：

```go
const (
    TypeGeneric    AgentType = "generic"     // 通用 Agent
    TypeAssistant  AgentType = "assistant"   // 助手
    TypeAnalyzer   AgentType = "analyzer"    // 分析
    TypeTranslator AgentType = "translator"  // 翻译
    TypeSummarizer AgentType = "summarizer"  // 摘要
    TypeReviewer   AgentType = "reviewer"    // 审查
)
```

**重要**: 这些预定义类型**完全是可选的**，你可以使用它们，也可以定义自己的类型。

### 自定义类型

由于 `AgentType` 是 `string` 类型，你可以直接定义任何自己的类型：

```go
// 方式 1: 直接使用字符串
cfg := agent.Config{
    Type: "my-custom-agent",
    // ...
}

// 方式 2: 定义自己的常量
const (
    TypeNovelWriter   agent.AgentType = "novel-writer"
    TypeCodeReviewer  agent.AgentType = "code-reviewer"
    TypeDataAnalyst   agent.AgentType = "data-analyst"
    TypeCustomerSupport agent.AgentType = "customer-support"
)

cfg := agent.Config{
    Type: TypeNovelWriter,
    // ...
}
```

## 创建自定义 Agent

### 方式 1: 使用 BaseAgent（推荐）

BaseAgent 提供了完整的基础功能，你只需配置即可：

```go
package main

import (
    "github.com/yourusername/agentflow/agent"
    "github.com/yourusername/agentflow/llm"
)

// 定义自己的 Agent 类型
const TypeNovelWriter agent.AgentType = "novel-writer"

func NewNovelWriterAgent(
    provider llm.Provider,
    memory agent.MemoryManager,
    tools agent.ToolManager,
    bus agent.EventBus,
    logger *zap.Logger,
) *agent.BaseAgent {
    cfg := agent.Config{
        ID:          "novel-writer-001",
        Name:        "小说创作助手",
        Type:        TypeNovelWriter,
        Description: "专业的小说创作 AI 助手",
        Model:       "gpt-4",
        MaxTokens:   4000,
        Temperature: 0.8,
        PromptBundle: agent.PromptBundle{
            Version: "1.0",
            System: agent.SystemPrompt{
                Identity: "你是一位经验丰富的小说作家，擅长创作引人入胜的故事。",
                Policies: []string{
                    "保持故事的连贯性和逻辑性",
                    "注重人物性格的塑造",
                    "使用生动的描写和对话",
                },
                OutputRules: []string{
                    "输出格式为 Markdown",
                    "每次输出 500-1000 字",
                },
            },
        },
        Tools: []string{"search_reference", "generate_outline"},
    }
    
    return agent.NewBaseAgent(cfg, provider, memory, tools, bus, logger)
}
```

### 方式 2: 实现 Agent 接口（高级）

如果需要完全自定义行为，可以实现 `Agent` 接口：

```go
package main

import (
    "context"
    "github.com/yourusername/agentflow/agent"
    "github.com/yourusername/agentflow/llm"
)

// 自定义 Agent 类型
const TypeCodeReviewer agent.AgentType = "code-reviewer"

// CodeReviewerAgent 代码审查 Agent
type CodeReviewerAgent struct {
    *agent.BaseAgent  // 嵌入 BaseAgent 复用功能
    reviewRules []string
}

func NewCodeReviewerAgent(
    provider llm.Provider,
    memory agent.MemoryManager,
    tools agent.ToolManager,
    bus agent.EventBus,
    logger *zap.Logger,
) *CodeReviewerAgent {
    cfg := agent.Config{
        ID:          "code-reviewer-001",
        Name:        "代码审查助手",
        Type:        TypeCodeReviewer,
        Model:       "gpt-4",
        MaxTokens:   2000,
        Temperature: 0.3,  // 低温度，更严谨
        PromptBundle: agent.PromptBundle{
            Version: "1.0",
            System: agent.SystemPrompt{
                Identity: "你是一位资深的代码审查专家。",
                Policies: []string{
                    "检查代码质量和最佳实践",
                    "识别潜在的 bug 和安全问题",
                    "提供具体的改进建议",
                },
            },
        },
    }
    
    base := agent.NewBaseAgent(cfg, provider, memory, tools, bus, logger)
    
    return &CodeReviewerAgent{
        BaseAgent: base,
        reviewRules: []string{
            "检查命名规范",
            "检查错误处理",
            "检查性能问题",
        },
    }
}

// Plan 自定义规划逻辑
func (a *CodeReviewerAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
    // 自定义规划逻辑
    return &agent.PlanResult{
        Steps: []string{
            "1. 分析代码结构",
            "2. 检查代码规范",
            "3. 识别潜在问题",
            "4. 生成审查报告",
        },
    }, nil
}

// Execute 自定义执行逻辑
func (a *CodeReviewerAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
    // 可以调用 BaseAgent 的方法
    messages := []llm.Message{
        {
            Role:    llm.RoleSystem,
            Content: a.Config().PromptBundle.RenderSystemPrompt(),
        },
        {
            Role:    llm.RoleUser,
            Content: input.Content,
        },
    }
    
    resp, err := a.ChatCompletion(ctx, messages)
    if err != nil {
        return nil, err
    }
    
    return &agent.Output{
        TraceID: input.TraceID,
        Content: resp.Choices[0].Message.Content,
        Metadata: map[string]any{
            "review_rules_applied": a.reviewRules,
        },
    }, nil
}

// Observe 自定义观察逻辑
func (a *CodeReviewerAgent) Observe(ctx context.Context, feedback *agent.Feedback) error {
    // 处理反馈，更新审查规则
    return nil
}
```

## 完整示例：多 Agent 协作系统

```go
package main

import (
    "context"
    "fmt"
    "github.com/yourusername/agentflow/agent"
    "github.com/yourusername/agentflow/llm"
    "github.com/yourusername/agentflow/providers"
    "github.com/yourusername/agentflow/providers/openai"
    "go.uber.org/zap"
)

// 定义自己的 Agent 类型
const (
    TypeStoryPlanner  agent.AgentType = "story-planner"
    TypeStoryWriter   agent.AgentType = "story-writer"
    TypeStoryReviewer agent.AgentType = "story-reviewer"
)

func main() {
    logger, _ := zap.NewDevelopment()
    
    // 创建 Provider
    cfg := providers.OpenAIConfig{
        APIKey: "your-api-key",
    }
    provider := openai.NewOpenAIProvider(cfg, logger)
    
    // 创建 3 个不同类型的 Agent
    planner := createPlannerAgent(provider, logger)
    writer := createWriterAgent(provider, logger)
    reviewer := createReviewerAgent(provider, logger)
    
    ctx := context.Background()
    
    // 1. 规划阶段
    planInput := &agent.Input{
        Content: "创作一个关于 AI 的科幻短篇小说",
    }
    planResult, _ := planner.Execute(ctx, planInput)
    fmt.Printf("规划结果: %s\n", planResult.Content)
    
    // 2. 写作阶段
    writeInput := &agent.Input{
        Content: planResult.Content,
    }
    writeResult, _ := writer.Execute(ctx, writeInput)
    fmt.Printf("写作结果: %s\n", writeResult.Content)
    
    // 3. 审查阶段
    reviewInput := &agent.Input{
        Content: writeResult.Content,
    }
    reviewResult, _ := reviewer.Execute(ctx, reviewInput)
    fmt.Printf("审查结果: %s\n", reviewResult.Content)
}

func createPlannerAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
    cfg := agent.Config{
        ID:          "planner-001",
        Name:        "故事规划师",
        Type:        TypeStoryPlanner,
        Model:       "gpt-4",
        Temperature: 0.7,
        PromptBundle: agent.PromptBundle{
            System: agent.SystemPrompt{
                Identity: "你是一位故事规划专家，擅长构思故事大纲。",
            },
        },
    }
    return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}

func createWriterAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
    cfg := agent.Config{
        ID:          "writer-001",
        Name:        "故事作家",
        Type:        TypeStoryWriter,
        Model:       "gpt-4",
        Temperature: 0.8,
        PromptBundle: agent.PromptBundle{
            System: agent.SystemPrompt{
                Identity: "你是一位创意作家，擅长将大纲扩展为生动的故事。",
            },
        },
    }
    return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}

func createReviewerAgent(provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
    cfg := agent.Config{
        ID:          "reviewer-001",
        Name:        "故事审查员",
        Type:        TypeStoryReviewer,
        Model:       "gpt-4",
        Temperature: 0.3,
        PromptBundle: agent.PromptBundle{
            System: agent.SystemPrompt{
                Identity: "你是一位文学评论家，擅长发现故事中的问题并提出改进建议。",
            },
        },
    }
    return agent.NewBaseAgent(cfg, provider, nil, nil, nil, logger)
}
```

## 最佳实践

### 1. Agent 类型命名

```go
// ✅ 好的命名
const (
    TypeDataAnalyst     agent.AgentType = "data-analyst"
    TypeCustomerSupport agent.AgentType = "customer-support"
    TypeCodeGenerator   agent.AgentType = "code-generator"
)

// ❌ 避免的命名
const (
    TypeAgent1 agent.AgentType = "agent1"  // 不够描述性
    TypeA      agent.AgentType = "a"       // 太简短
)
```

### 2. 配置管理

```go
// 使用配置文件管理 Agent 类型
type AgentConfig struct {
    Agents map[string]AgentDefinition `yaml:"agents"`
}

type AgentDefinition struct {
    Type        string   `yaml:"type"`
    Name        string   `yaml:"name"`
    Model       string   `yaml:"model"`
    Temperature float32  `yaml:"temperature"`
    SystemPrompt string  `yaml:"system_prompt"`
}

// 从配置文件加载
func LoadAgentsFromConfig(path string) (map[string]*agent.BaseAgent, error) {
    // 读取配置文件
    // 创建 Agent 实例
    // ...
}
```

### 3. Agent 注册表

```go
// 创建 Agent 注册表
type AgentRegistry struct {
    agents map[agent.AgentType]agent.Agent
    mu     sync.RWMutex
}

func NewAgentRegistry() *AgentRegistry {
    return &AgentRegistry{
        agents: make(map[agent.AgentType]agent.Agent),
    }
}

func (r *AgentRegistry) Register(agentType agent.AgentType, a agent.Agent) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.agents[agentType] = a
}

func (r *AgentRegistry) Get(agentType agent.AgentType) (agent.Agent, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    a, ok := r.agents[agentType]
    return a, ok
}
```

## 总结

AgentFlow 的 Agent 类型系统是**完全开放和可扩展的**：

1. ✅ **无限制**: 可以定义任意数量的自定义 Agent 类型
2. ✅ **灵活性**: 使用字符串类型，不受枚举限制
3. ✅ **兼容性**: 预定义类型只是建议，不是强制
4. ✅ **可组合**: 可以创建多个 Agent 协作完成复杂任务

框架提供的预定义类型只是**示例和建议**，你完全可以根据自己的业务需求定义任何类型的 Agent！
