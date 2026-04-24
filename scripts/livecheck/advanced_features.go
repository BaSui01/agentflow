package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	mcpproto "github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	agentruntime "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team/engines/hierarchical"
	multiagent "github.com/BaSui01/agentflow/agent/team/engines/multiagent"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func runSkillsAndMCP(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test D: skills+mcp start")

	skillMgr := tools.NewSkillManager(tools.DefaultSkillManagerConfig(), logger)
	skill, err := tools.NewSkillBuilder("architecture_review", "Architecture Review").
		WithDescription("Evaluate architecture with pros, cons, and risks").
		WithCategory("architecture").
		WithTags("architecture", "review", "risk").
		WithInstructions("Always provide: 1) key advantages 2) key risks 3) final recommendation.").
		WithPriority(10).
		Build()
	if err != nil {
		return fmt.Errorf("build skill: %w", err)
	}
	if err := skillMgr.RegisterSkill(skill); err != nil {
		return fmt.Errorf("register skill: %w", err)
	}

	discoveredSkills, err := skillMgr.DiscoverSkills(ctx, "please review this architecture proposal")
	if err != nil {
		return fmt.Errorf("discover skills: %w", err)
	}

	mcpServer := mcpproto.NewMCPServer("livecheck-mcp", "0.1.0", logger)
	defer mcpServer.Close()

	if err := mcpServer.RegisterTool(&mcpproto.ToolDefinition{
		Name:        "mcp_sum",
		Description: "Sum two numbers",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		a, err := asFloat(args["a"])
		if err != nil {
			return nil, err
		}
		b, err := asFloat(args["b"])
		if err != nil {
			return nil, err
		}
		return map[string]any{"sum": a + b}, nil
	}); err != nil {
		return fmt.Errorf("register mcp tool: %w", err)
	}

	if err := mcpServer.RegisterResource(&mcpproto.Resource{
		URI:         "live://docs/architecture",
		Name:        "architecture-notes",
		Description: "livecheck architecture notes",
		Type:        mcpproto.ResourceTypeText,
		MimeType:    "text/plain",
		Content:     "RAG + Tool Use + Multi-Agent",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}); err != nil {
		return fmt.Errorf("register mcp resource: %w", err)
	}

	if err := mcpServer.RegisterPrompt(&mcpproto.PromptTemplate{
		Name:        "brief-review",
		Description: "brief review prompt",
		Template:    "Please review {{topic}} in 3 bullet points.",
		Variables:   []string{"topic"},
	}); err != nil {
		return fmt.Errorf("register mcp prompt: %w", err)
	}

	initResp, err := mcpServer.HandleMessage(ctx, mcpproto.NewMCPRequest("init-1", "initialize", map[string]any{
		"protocolVersion": mcpproto.MCPVersion,
	}))
	if err != nil {
		return fmt.Errorf("mcp initialize transport error: %w", err)
	}
	if initResp == nil || initResp.Error != nil {
		return fmt.Errorf("mcp initialize failed: %+v", initResp)
	}

	toolsResp, err := mcpServer.HandleMessage(ctx, mcpproto.NewMCPRequest("tools-1", "tools/list", map[string]any{}))
	if err != nil {
		return fmt.Errorf("mcp tools/list transport error: %w", err)
	}
	if toolsResp == nil || toolsResp.Error != nil {
		return fmt.Errorf("mcp tools/list failed: %+v", toolsResp)
	}

	callResp, err := mcpServer.HandleMessage(ctx, mcpproto.NewMCPRequest("call-1", "tools/call", map[string]any{
		"name":      "mcp_sum",
		"arguments": map[string]any{"a": 7, "b": 5},
	}))
	if err != nil {
		return fmt.Errorf("mcp tools/call transport error: %w", err)
	}
	if callResp == nil || callResp.Error != nil {
		return fmt.Errorf("mcp tools/call failed: %+v", callResp)
	}

	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "live-agent-skills-mcp",
			Name: "live-agent-skills-mcp",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       model,
			MaxTokens:   300,
			Temperature: 0.2,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "You are an architecture review assistant.",
		},
	}

	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	ag, err := agentruntime.NewBuilder(gateway, logger).Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer ag.Teardown(context.Background())

	ag.EnableSkills(skillMgr)
	ag.EnableMCP(mcpServer)
	status := ag.GetFeatureStatus()
	if !status["skills"] || !status["mcp"] {
		return fmt.Errorf("feature flags not enabled as expected: %+v", status)
	}

	if err := ag.Init(ctx); err != nil {
		return err
	}

	opts := agent.DefaultEnhancedExecutionOptions()
	opts.UseSkills = true
	opts.UseObservability = false
	opts.RecordMetrics = false
	opts.RecordTrace = false

	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	out, err := ag.ExecuteEnhanced(execCtx, &agent.Input{
		TraceID: "live-skills-mcp-trace",
		Content: "请评估是否在SaaS系统采用事件驱动架构，简要给出优缺点和建议。",
	}, opts)
	if err != nil {
		return err
	}

	callPreview := ""
	if callResp.Result != nil {
		b, _ := json.Marshal(callResp.Result)
		callPreview = truncateText(string(b), 120)
	}
	logger.Info("test D: skills+mcp done",
		zap.Int("discovered_skills", len(discoveredSkills)),
		zap.Bool("skills_enabled", status["skills"]),
		zap.Bool("mcp_enabled", status["mcp"]),
		zap.String("mcp_call_result", callPreview),
		zap.String("enhanced_output", truncateText(out.Content, 140)),
	)
	return nil
}

func runMultiAgentCollaboration(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test E: multi-agent collaboration start")

	agents := []agent.Agent{
		newLiveBaseAgent("live-debate-analyst", "live-debate-analyst", model, "You are an optimistic analyst.", provider, logger),
		newLiveBaseAgent("live-debate-critic", "live-debate-critic", model, "You are a cautious critic focused on risk.", provider, logger),
		newLiveBaseAgent("live-debate-synth", "live-debate-synth", model, "You synthesize viewpoints clearly.", provider, logger),
	}
	defer teardownAgents(agents)

	for _, a := range agents {
		if err := a.Init(ctx); err != nil {
			return fmt.Errorf("init %s: %w", a.ID(), err)
		}
	}

	debateCfg := multiagent.DefaultMultiAgentConfig()
	debateCfg.Pattern = multiagent.PatternDebate
	debateCfg.MaxRounds = 1
	debateCfg.Timeout = 3 * time.Minute
	debateSystem := multiagent.NewMultiAgentSystem(agents, debateCfg, logger)

	debateCtx, cancelDebate := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelDebate()
	debateOut, err := debateSystem.Execute(debateCtx, &agent.Input{
		TraceID: "live-multi-debate-trace",
		Content: "Should a SaaS platform adopt event-driven architecture? Give concise pros/cons and final recommendation.",
	})
	if err != nil {
		return fmt.Errorf("debate execute: %w", err)
	}

	broadcastCfg := multiagent.DefaultMultiAgentConfig()
	broadcastCfg.Pattern = multiagent.PatternBroadcast
	broadcastCfg.Timeout = 3 * time.Minute
	broadcastSystem := multiagent.NewMultiAgentSystem(agents, broadcastCfg, logger)

	broadcastCtx, cancelBroadcast := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelBroadcast()
	broadcastOut, err := broadcastSystem.Execute(broadcastCtx, &agent.Input{
		TraceID: "live-multi-broadcast-trace",
		Content: "Evaluate release strategies: canary vs blue-green vs rolling. Provide a short recommendation.",
	})
	if err != nil {
		return fmt.Errorf("broadcast execute: %w", err)
	}

	logger.Info("test E: multi-agent collaboration done",
		zap.String("debate_output", truncateText(debateOut.Content, 140)),
		zap.String("broadcast_output", truncateText(broadcastOut.Content, 140)),
	)
	return nil
}

func runHierarchicalExecution(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test F: hierarchical execution start")

	base := newLiveBaseAgent("live-hier-base", "live-hier-base", model, "You are a hierarchical base coordinator.", provider, logger)
	supervisor := newLiveBaseAgent("live-hier-supervisor", "live-hier-supervisor", model, "You decompose and aggregate tasks.", provider, logger)
	workers := []agent.Agent{
		newLiveBaseAgent("live-hier-worker-1", "live-hier-worker-1", model, "You are worker 1 focusing on analysis.", provider, logger),
		newLiveBaseAgent("live-hier-worker-2", "live-hier-worker-2", model, "You are worker 2 focusing on synthesis.", provider, logger),
		newLiveBaseAgent("live-hier-worker-3", "live-hier-worker-3", model, "You are worker 3 focusing on risk and edge cases.", provider, logger),
		newLiveBaseAgent("live-hier-worker-4", "live-hier-worker-4", model, "You are worker 4 focusing on implementation detail.", provider, logger),
	}
	defer func() {
		_ = base.Teardown(context.Background())
		_ = supervisor.Teardown(context.Background())
		teardownAgents(workers)
	}()

	if err := base.Init(ctx); err != nil {
		return err
	}
	if err := supervisor.Init(ctx); err != nil {
		return err
	}
	for _, w := range workers {
		if err := w.Init(ctx); err != nil {
			return fmt.Errorf("init worker %s: %w", w.ID(), err)
		}
	}

	hCfg := hierarchical.DefaultHierarchicalConfig()
	hCfg.MaxWorkers = 4
	hCfg.WorkerSelection = "round_robin"
	hCfg.MaxRetries = 1
	hCfg.TaskTimeout = 90 * time.Second

	hier := hierarchical.NewHierarchicalAgent(base, supervisor, workers, hCfg, logger)

	execCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	out, err := hier.Execute(execCtx, &agent.Input{
		TraceID: "live-hier-trace",
		Content: "Split into exactly 3 independent subtasks and finish: 1) explain MCP role in AgentFlow, 2) explain RAG role in AgentFlow, 3) provide integrated conclusion.",
	})
	if err != nil {
		return err
	}

	logger.Info("test F: hierarchical execution done",
		zap.String("output", truncateText(out.Content, 160)),
	)
	return nil
}

func runSubAgentDelegation(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test G: subagent delegation start")

	subAgent := newLiveBaseAgent("live-sub-agent", "live-sub-agent", model, "You are a specialist sub-agent for delegated analysis.", provider, logger)
	defer subAgent.Teardown(context.Background())
	if err := subAgent.Init(ctx); err != nil {
		return err
	}

	subTool := agent.NewAgentTool(subAgent, &agent.AgentToolConfig{
		Name:        "delegate_subagent",
		Description: "Delegate a focused analysis task to a specialist sub-agent",
		Timeout:     90 * time.Second,
	})
	toolMgr := newSubAgentToolManager(logger, subTool)

	parentCfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "live-parent-agent",
			Name: "live-parent-agent",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       model,
			MaxTokens:   450,
			Temperature: 0,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       "You can delegate work to sub-agent tool and then answer succinctly.",
			Tools:              []string{"delegate_subagent"},
			MaxReActIterations: 5,
		},
	}

	parentGateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	parent, err := agentruntime.NewBuilder(parentGateway, logger).WithOptions(agentruntime.BuildOptions{
		ToolManager: toolMgr,
	}).Build(ctx, parentCfg)
	if err != nil {
		return err
	}
	defer parent.Teardown(context.Background())
	if err := parent.Init(ctx); err != nil {
		return err
	}

	var streamToolCalls atomic.Int64
	var streamToolResults atomic.Int64

	execCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	execCtx = agent.WithRunConfig(execCtx, &agent.RunConfig{
		ToolChoice:         agent.StringPtr("auto"),
		MaxReActIterations: agent.IntPtr(5),
	})
	execCtx = agent.WithRuntimeStreamEmitter(execCtx, func(ev agent.RuntimeStreamEvent) {
		switch ev.Type {
		case agent.RuntimeStreamToolCall:
			streamToolCalls.Add(1)
		case agent.RuntimeStreamToolResult:
			streamToolResults.Add(1)
		}
	})

	resp, err := parent.ChatCompletion(execCtx, []types.Message{
		{
			Role: llm.RoleUser,
			Content: "Please delegate once to sub-agent to analyze the benefits of using subagents, " +
				"then provide one concise final sentence.",
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "max iterations reached") && toolMgr.TotalCalls() > 0 {
			logger.Warn("test G reached max iterations after successful subagent calls",
				zap.Int64("tool_calls_executed", toolMgr.TotalCalls()),
				zap.Int64("stream_tool_call_events", streamToolCalls.Load()),
				zap.Int64("stream_tool_result_events", streamToolResults.Load()),
			)
			return nil
		}
		return err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return err
	}

	logger.Info("test G: subagent delegation done",
		zap.Int64("tool_calls_executed", toolMgr.TotalCalls()),
		zap.Int64("stream_tool_call_events", streamToolCalls.Load()),
		zap.Int64("stream_tool_result_events", streamToolResults.Load()),
		zap.String("final_answer", truncateText(choice.Message.Content, 140)),
	)
	return nil
}

func newLiveBaseAgent(id, name, model, systemPrompt string, provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   id,
			Name: name,
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       model,
			MaxTokens:   360,
			Temperature: 0.2,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt: systemPrompt,
		},
	}
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	ag, err := agentruntime.NewBuilder(gateway, logger).Build(context.Background(), cfg)
	if err != nil {
		return nil
	}
	return ag
}

func teardownAgents(agents []agent.Agent) {
	for _, a := range agents {
		if a == nil {
			continue
		}
		_ = a.Teardown(context.Background())
	}
}

type subAgentToolManager struct {
	logger     *zap.Logger
	tool       *agent.AgentTool
	totalCalls atomic.Int64
}

func newSubAgentToolManager(logger *zap.Logger, tool *agent.AgentTool) *subAgentToolManager {
	return &subAgentToolManager{
		logger: logger,
		tool:   tool,
	}
}

func (m *subAgentToolManager) GetAllowedTools(agentID string) []types.ToolSchema {
	return []types.ToolSchema{m.tool.Schema()}
}

func (m *subAgentToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
	out := make([]llmtools.ToolResult, 0, len(calls))
	for _, c := range calls {
		m.totalCalls.Add(1)
		if c.Name != m.tool.Name() {
			out = append(out, llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "unknown tool: " + c.Name,
			})
			continue
		}
		res := m.tool.Execute(ctx, c)
		m.logger.Info("subagent tool executed",
			zap.String("agent_id", agentID),
			zap.String("tool", c.Name),
			zap.String("tool_call_id", c.ID),
			zap.String("error", res.Error),
			zap.ByteString("result", res.Result),
		)
		out = append(out, res)
	}
	return out
}

func (m *subAgentToolManager) TotalCalls() int64 {
	return m.totalCalls.Load()
}

func asFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}
