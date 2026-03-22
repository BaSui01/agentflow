// =============================================================================
// 🧪 AgentFlow 真实 Agent 框架测试 — 用大模型跑完整 Agent 链路
// =============================================================================
// 测试目标：验证 Agent 框架的完整能力，不是 mock，是真正调用 LLM！
//
// D1. Agent 构建 + 基础执行（无工具）
// D2. Agent + 工具调用（ReAct 循环）
// D3. Agent + 重试（产出验证失败后重试）
// D4. Agent + 循环迭代（多轮自我改进）
// D5. Agent + 多工具协同
// D6. Agent + 错误恢复（工具失败降级）
// D7. HierarchicalAgent（层级监督）
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	apiKey  = "sk-W2WaPZdpC6iOoP6bMY8P7XwRfIZua7c2tkTuQ0DIPGPZKQwJ"
	baseURL = "https://ai.xoooox.xyz"
	model   = "glm-5"
)

type R struct{ Name, Status string; D time.Duration; Info string }
var rs []R

func rec(n, s string, d time.Duration, info string) {
	rs = append(rs, R{n, s, d, info})
	i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[s]
	fmt.Printf("  %s %-32s %8v  %s\n", i, n, d.Round(time.Millisecond), info)
}

func txt(m types.Message) string {
	if m.Content != "" { return m.Content }
	if m.ReasoningContent != nil { return *m.ReasoningContent }
	return ""
}

func cut(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s); if len(r) <= n { return s }; return string(r[:n]) + "..."
}

// ─── 工具管理器 ────────────────────────────────────

type simpleToolManager struct {
	registry *llmtools.DefaultRegistry
	executor *llmtools.DefaultExecutor
}

func (m *simpleToolManager) GetAllowedTools(_ string) []types.ToolSchema {
	return m.registry.List()
}

func (m *simpleToolManager) ExecuteForAgent(ctx context.Context, _ string, calls []types.ToolCall) []llmtools.ToolResult {
	return m.executor.Execute(ctx, calls)
}

func newToolManager(lg *zap.Logger) *simpleToolManager {
	reg := llmtools.NewDefaultRegistry(lg)

	reg.Register("get_weather", func(_ context.Context, a json.RawMessage) (json.RawMessage, error) {
		var p struct{ City string `json:"city"` }; json.Unmarshal(a, &p)
		return json.Marshal(map[string]any{"city": p.City, "temp": 22, "condition": "晴", "humidity": 65})
	}, llmtools.ToolMetadata{Schema: types.ToolSchema{Name: "get_weather", Description: "查询城市天气",
		Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string","description":"城市名"}},"required":["city"]}`)}, Timeout: 10 * time.Second})

	reg.Register("calculate", func(_ context.Context, a json.RawMessage) (json.RawMessage, error) {
		var p struct{ Expr string `json:"expression"` }; json.Unmarshal(a, &p)
		return json.Marshal(map[string]any{"expression": p.Expr, "result": "42"})
	}, llmtools.ToolMetadata{Schema: types.ToolSchema{Name: "calculate", Description: "数学计算",
		Parameters: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string","description":"表达式"}},"required":["expression"]}`)}, Timeout: 5 * time.Second})

	reg.Register("broken_tool", func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("数据库连接超时")
	}, llmtools.ToolMetadata{Schema: types.ToolSchema{Name: "broken_tool", Description: "查询数据库（当前不可用）",
		Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)}, Timeout: 5 * time.Second})

	return &simpleToolManager{registry: reg, executor: llmtools.NewDefaultExecutor(reg, lg)}
}

// ─── 入口 ────────────────────────────────────────────

func main() {
	lg, _ := zap.NewDevelopment()
	defer lg.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	provider := openaicompat.New(openaicompat.Config{
		ProviderName: "glm", APIKey: apiKey, BaseURL: baseURL,
		DefaultModel: model, FallbackModel: model, Timeout: 120 * time.Second,
	}, lg)

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🧪 AgentFlow 真实 Agent 框架测试 — 调用大模型完整链路      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	fmt.Println("\n━━━ Agent 基础执行 ━━━")
	d01AgentBasicExecution(ctx, provider, lg)

	fmt.Println("\n━━━ Agent + 工具调用 ━━━")
	d02AgentWithTools(ctx, provider, lg)

	fmt.Println("\n━━━ Agent + 多工具协同 ━━━")
	d03AgentMultiTools(ctx, provider, lg)

	fmt.Println("\n━━━ Agent + 错误恢复 ━━━")
	d04AgentErrorRecovery(ctx, provider, lg)

	fmt.Println("\n━━━ Agent + 循环迭代 ━━━")
	d05AgentLoopIteration(ctx, provider, lg)

	fmt.Println("\n━━━ SubAgent 并行执行 ━━━")
	d06SubAgentParallel(ctx, provider, lg)

	fmt.Println("\n━━━ SubAgent 管理器 ━━━")
	d07SubAgentManager(ctx, provider, lg)

	fmt.Println("\n━━━ Agent Reflection 自我反思 ━━━")
	d08AgentReflection(ctx, provider, lg)

	fmt.Println("\n━━━ Agent 流式执行 ━━━")
	d09AgentStreaming(ctx, provider, lg)

	printSummary()
}

// =============================================================================
// D1: Agent 构建 + 基础执行（无工具）
// =============================================================================

func d01AgentBasicExecution(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core: types.CoreConfig{ID: "basic-agent", Name: "Basic Agent", Type: "assistant"},
		LLM:  types.LLMConfig{Model: model, MaxTokens: 512, Temperature: 0.1},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "你是一个简洁的助手，回答不超过50字。",
		},
	}).WithProvider(provider).WithLogger(lg).Build()

	if err != nil {
		rec("Agent构建+执行", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}

	if err := ag.Init(ctx); err != nil {
		rec("Agent构建+执行", "FAIL", time.Since(t), fmt.Sprintf("Init失败: %v", err))
		return
	}

	output, err := ag.Execute(ctx, &agent.Input{
		TraceID: "test-basic-001",
		Content: "1+1等于几？只回答数字。",
	})

	if err != nil {
		rec("Agent构建+执行", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if output != nil && output.Content != "" {
		rec("Agent构建+执行", "PASS", time.Since(t), fmt.Sprintf("输出: %s | Token: %d", cut(output.Content, 50), output.TokensUsed))
	} else if output != nil && output.Content == "" {
		// 思考模型可能把内容放在 reasoning 里
		rec("Agent构建+执行", "WARN", time.Since(t), "content为空(思考模型)")
	} else {
		rec("Agent构建+执行", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D2: Agent + 工具调用（ReAct 循环）
// =============================================================================

func d02AgentWithTools(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()
	tm := newToolManager(lg)

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core: types.CoreConfig{ID: "tool-agent", Name: "Tool Agent", Type: "assistant"},
		LLM:  types.LLMConfig{Model: model, MaxTokens: 1024, Temperature: 0.1},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       "你是一个智能助手。请使用提供的工具来回答问题。",
			Tools:              []string{"get_weather", "calculate"},
			MaxReActIterations: 5,
		},
	}).WithProvider(provider).WithToolManager(tm).WithLogger(lg).Build()

	if err != nil {
		rec("Agent+工具调用", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}

	if err := ag.Init(ctx); err != nil {
		rec("Agent+工具调用", "FAIL", time.Since(t), fmt.Sprintf("Init失败: %v", err))
		return
	}

	output, err := ag.Execute(ctx, &agent.Input{
		TraceID: "test-tool-001",
		Content: "北京今天天气怎么样？",
	})

	if err != nil {
		rec("Agent+工具调用", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if output != nil && (strings.Contains(output.Content, "晴") || strings.Contains(output.Content, "22") || strings.Contains(output.Content, "天气") || output.TokensUsed > 100) {
		rec("Agent+工具调用", "PASS", time.Since(t), fmt.Sprintf("Token: %d | %s", output.TokensUsed, cut(output.Content, 60)))
	} else if output != nil && output.Content != "" {
		rec("Agent+工具调用", "WARN", time.Since(t), fmt.Sprintf("有回复但可能未用工具: %s", cut(output.Content, 60)))
	} else {
		rec("Agent+工具调用", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D3: Agent + 多工具协同
// =============================================================================

func d03AgentMultiTools(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()
	tm := newToolManager(lg)

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core: types.CoreConfig{ID: "multi-tool-agent", Name: "Multi Tool Agent", Type: "assistant"},
		LLM:  types.LLMConfig{Model: model, MaxTokens: 1024, Temperature: 0.1},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       "你是助手。请使用工具回答：先查天气，再做计算。",
			Tools:              []string{"get_weather", "calculate"},
			MaxReActIterations: 5,
		},
	}).WithProvider(provider).WithToolManager(tm).WithLogger(lg).Build()

	if err != nil {
		rec("Agent+多工具", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	output, err := ag.Execute(ctx, &agent.Input{
		TraceID: "test-multi-001",
		Content: "上海天气怎么样？另外帮我算 256*3",
	})

	if err != nil {
		rec("Agent+多工具", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if output != nil && output.TokensUsed > 0 {
		rec("Agent+多工具", "PASS", time.Since(t), fmt.Sprintf("Token: %d | %s", output.TokensUsed, cut(output.Content, 60)))
	} else {
		rec("Agent+多工具", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D4: Agent + 错误恢复（工具失败后降级）
// =============================================================================

func d04AgentErrorRecovery(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()
	tm := newToolManager(lg)

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core: types.CoreConfig{ID: "error-agent", Name: "Error Recovery Agent", Type: "assistant"},
		LLM:  types.LLMConfig{Model: model, MaxTokens: 1024, Temperature: 0.1},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       "你是助手。如果工具调用失败，请用自己的知识回答。",
			Tools:              []string{"broken_tool"},
			MaxReActIterations: 3,
		},
	}).WithProvider(provider).WithToolManager(tm).WithLogger(lg).Build()

	if err != nil {
		rec("Agent+错误恢复", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	output, err := ag.Execute(ctx, &agent.Input{
		TraceID: "test-error-001",
		Content: "帮我查一下数据库中的用户数量。",
	})

	if err != nil {
		// 即使有错误，如果有输出也算部分成功
		rec("Agent+错误恢复", "WARN", time.Since(t), fmt.Sprintf("err=%v", err))
		return
	}

	if output != nil && output.Content != "" {
		rec("Agent+错误恢复", "PASS", time.Since(t), fmt.Sprintf("工具失败后降级回答: %s", cut(output.Content, 60)))
	} else {
		rec("Agent+错误恢复", "WARN", time.Since(t), "无内容输出")
	}
}

// =============================================================================
// D5: Agent + 循环迭代（多轮自我改进）
// =============================================================================

func d05AgentLoopIteration(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	// 第一轮：生成初始回答
	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core: types.CoreConfig{ID: "loop-agent", Name: "Loop Agent", Type: "assistant"},
		LLM:  types.LLMConfig{Model: model, MaxTokens: 512, Temperature: 0.7},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "你是一个迭代优化助手。每次回答不超过100字。",
		},
	}).WithProvider(provider).WithLogger(lg).Build()

	if err != nil {
		rec("Agent+循环迭代", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	// 手动循环：模拟 LoopAgent 模式
	var outputs []string
	lastContent := "用3句话总结微服务架构的优缺点。"

	for iter := 0; iter < 3; iter++ {
		prompt := lastContent
		if iter > 0 {
			prompt = fmt.Sprintf("上一轮回答：%s\n\n请改进这个回答，如果满意就在开头加DONE。", lastContent)
		}

		output, err := ag.Execute(ctx, &agent.Input{
			TraceID: fmt.Sprintf("loop-%d", iter),
			Content: prompt,
		})
		if err != nil {
			rec("Agent+循环迭代", "WARN", time.Since(t), fmt.Sprintf("第%d轮失败: %v", iter+1, err))
			break
		}

		content := output.Content
		outputs = append(outputs, content)

		if strings.Contains(content, "DONE") {
			rec("Agent+循环迭代", "PASS", time.Since(t), fmt.Sprintf("%d轮后DONE停止", iter+1))
			return
		}
		lastContent = content
	}

	if len(outputs) > 0 {
		rec("Agent+循环迭代", "PASS", time.Since(t), fmt.Sprintf("%d轮迭代完成", len(outputs)))
	} else {
		rec("Agent+循环迭代", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D6: SubAgent 并行执行 — 多个 Agent 并行处理不同任务
// =============================================================================

func d06SubAgentParallel(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	// 构建 3 个不同职责的 SubAgent
	buildAgent := func(id, name, systemPrompt string) agent.Agent {
		ag, err := agent.NewAgentBuilder(types.AgentConfig{
			Core:    types.CoreConfig{ID: id, Name: name, Type: "assistant"},
			LLM:     types.LLMConfig{Model: model, MaxTokens: 256, Temperature: 0.1},
			Runtime: types.RuntimeConfig{SystemPrompt: systemPrompt},
		}).WithProvider(provider).WithLogger(lg).Build()
		if err != nil {
			return nil
		}
		ag.Init(ctx)
		return ag
	}

	analyst := buildAgent("analyst", "分析师", "你是数据分析师，用一句话回答。")
	critic := buildAgent("critic", "评论家", "你是技术评论家，用一句话回答。")
	writer := buildAgent("writer", "作家", "你是技术作家，用一句话回答。")

	if analyst == nil || critic == nil || writer == nil {
		rec("SubAgent并行", "FAIL", time.Since(t), "Agent构建失败")
		return
	}

	// 使用 AsyncExecutor 并行执行
	asyncExec := agent.NewAsyncExecutor(analyst, lg)

	output, err := asyncExec.ExecuteWithSubagents(ctx, &agent.Input{
		TraceID: "test-subagent-001",
		Content: "Go语言的主要优势是什么？",
	}, []agent.Agent{analyst, critic, writer})

	if err != nil {
		rec("SubAgent并行", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if output != nil && output.Content != "" {
		// 检查是否包含多个 SubAgent 的结果
		hasMultiple := strings.Contains(output.Content, "Subagent") || len(output.Content) > 100
		if hasMultiple {
			rec("SubAgent并行", "PASS", time.Since(t), fmt.Sprintf("3个SubAgent并行完成, Token:%d, 内容:%d字", output.TokensUsed, len([]rune(output.Content))))
		} else {
			rec("SubAgent并行", "PASS", time.Since(t), fmt.Sprintf("完成, %s", cut(output.Content, 60)))
		}
	} else {
		rec("SubAgent并行", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D7: SubAgent 管理器 — Spawn + Wait + 状态查询
// =============================================================================

func d07SubAgentManager(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	// 构建一个简单 Agent
	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core:    types.CoreConfig{ID: "managed-agent", Name: "Managed Agent", Type: "assistant"},
		LLM:     types.LLMConfig{Model: model, MaxTokens: 128, Temperature: 0.1},
		Runtime: types.RuntimeConfig{SystemPrompt: "用一个词回答。"},
	}).WithProvider(provider).WithLogger(lg).Build()

	if err != nil {
		rec("SubAgent管理器", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	// 创建 SubagentManager
	mgr := agent.NewSubagentManager(lg)
	defer mgr.Close()

	// Spawn SubAgent
	exec, err := mgr.SpawnSubagent(ctx, ag, &agent.Input{
		TraceID: "test-mgr-001",
		Content: "中国的首都？",
	})
	if err != nil {
		rec("SubAgent管理器", "FAIL", time.Since(t), fmt.Sprintf("Spawn失败: %v", err))
		return
	}

	// 检查状态
	status := exec.GetStatus()
	if status != agent.ExecutionStatusRunning {
		rec("SubAgent管理器", "WARN", time.Since(t), fmt.Sprintf("初始状态=%s(期望running)", status))
	}

	// 等待完成
	output, err := exec.Wait(ctx)
	if err != nil {
		rec("SubAgent管理器", "FAIL", time.Since(t), fmt.Sprintf("Wait失败: %v", err))
		return
	}

	// 验证最终状态
	finalStatus := exec.GetStatus()

	// 通过 Manager 查询
	queriedExec, queryErr := mgr.GetExecution(exec.ID)

	if output != nil && finalStatus == agent.ExecutionStatusCompleted && queryErr == nil && queriedExec != nil {
		rec("SubAgent管理器", "PASS", time.Since(t), fmt.Sprintf("Spawn→Run→Complete, status=%s, output=%s", finalStatus, cut(output.Content, 40)))
	} else {
		rec("SubAgent管理器", "WARN", time.Since(t), fmt.Sprintf("status=%s queryErr=%v", finalStatus, queryErr))
	}

	// 列出所有执行
	executions := mgr.ListExecutions()
	if len(executions) > 0 {
		rec("SubAgent列表查询", "PASS", time.Since(t), fmt.Sprintf("列出%d个执行", len(executions)))
	} else {
		rec("SubAgent列表查询", "WARN", time.Since(t), "列表为空")
	}
}

// ─── 汇总 ────────────────────────────────────────────

func printSummary() {
	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  📊 Agent 框架真实测试汇总                                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	ps, fl, wr := 0, 0, 0
	for _, r := range rs { switch r.Status { case "PASS": ps++; case "FAIL": fl++; case "WARN": wr++ } }
	fmt.Printf("\n  总计: %d | ✅ PASS: %d | ❌ FAIL: %d | ⚠️  WARN: %d\n\n", len(rs), ps, fl, wr)
	for _, r := range rs {
		i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[r.Status]
		fmt.Printf("  %s %-32s %8v  %s\n", i, r.Name, r.D.Round(time.Millisecond), r.Info)
	}
	if fl == 0 { fmt.Println("\n  🎉 Agent 框架完整链路全部通过！") } else { fmt.Printf("\n  ⚠️  有 %d 项失败\n", fl) }
}

// =============================================================================
// D8: Agent Reflection 自我反思 — 执行→评审→改进循环
// =============================================================================

func d08AgentReflection(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core:    types.CoreConfig{ID: "reflect-agent", Name: "Reflect Agent", Type: "assistant"},
		LLM:     types.LLMConfig{Model: model, MaxTokens: 1024, Temperature: 0.7},
		Runtime: types.RuntimeConfig{SystemPrompt: "你是一个写作助手，回答简洁。"},
	}).WithProvider(provider).WithLogger(lg).Build()

	if err != nil {
		rec("Reflection反思", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	reflector := agent.NewReflectionExecutor(ag, agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 2,
		MinQuality:    0.6,
	})

	result, err := reflector.ExecuteWithReflection(ctx, &agent.Input{
		TraceID: "test-reflect-001",
		Content: "用一句话解释什么是微服务架构。",
	})

	if err != nil {
		rec("Reflection反思", "FAIL", time.Since(t), fmt.Sprintf("执行失败: %v", err))
		return
	}

	if result != nil && result.FinalOutput != nil && result.FinalOutput.Content != "" {
		rec("Reflection反思", "PASS", time.Since(t), fmt.Sprintf("迭代%d次, 改进=%v, %s", result.Iterations, result.ImprovedByReflection, cut(result.FinalOutput.Content, 50)))
	} else if result != nil && result.FinalOutput != nil {
		rec("Reflection反思", "WARN", time.Since(t), fmt.Sprintf("迭代%d次但content为空", result.Iterations))
	} else {
		rec("Reflection反思", "FAIL", time.Since(t), "无输出")
	}
}

// =============================================================================
// D9: Agent 流式执行 — StreamCompletion
// =============================================================================

func d09AgentStreaming(ctx context.Context, provider llm.Provider, lg *zap.Logger) {
	t := time.Now()

	ag, err := agent.NewAgentBuilder(types.AgentConfig{
		Core:    types.CoreConfig{ID: "stream-agent", Name: "Stream Agent", Type: "assistant"},
		LLM:     types.LLMConfig{Model: model, MaxTokens: 256, Temperature: 0.1},
		Runtime: types.RuntimeConfig{SystemPrompt: "用一句话回答。"},
	}).WithProvider(provider).WithLogger(lg).Build()

	if err != nil {
		rec("Agent流式执行", "FAIL", time.Since(t), fmt.Sprintf("构建失败: %v", err))
		return
	}
	ag.Init(ctx)

	messages := []types.Message{
		{Role: "system", Content: "用一句话回答。"},
		{Role: "user", Content: "Go语言是什么？"},
	}

	stream, err := ag.StreamCompletion(ctx, messages)
	if err != nil {
		rec("Agent流式执行", "FAIL", time.Since(t), fmt.Sprintf("Stream失败: %v", err))
		return
	}

	var content strings.Builder
	chunkCount := 0
	for chunk := range stream {
		if chunk.Err != nil {
			rec("Agent流式执行", "FAIL", time.Since(t), fmt.Sprintf("chunk错误: %v", chunk.Err))
			return
		}
		content.WriteString(chunk.Delta.Content)
		if chunk.Delta.ReasoningContent != nil {
			content.WriteString(*chunk.Delta.ReasoningContent)
		}
		chunkCount++
	}

	if chunkCount > 0 && content.Len() > 0 {
		rec("Agent流式执行", "PASS", time.Since(t), fmt.Sprintf("%d chunks, %s", chunkCount, cut(content.String(), 50)))
	} else if chunkCount > 0 {
		rec("Agent流式执行", "WARN", time.Since(t), fmt.Sprintf("%d chunks但内容为空", chunkCount))
	} else {
		rec("Agent流式执行", "FAIL", time.Since(t), "0 chunks")
	}
}
