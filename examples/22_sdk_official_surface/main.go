package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/sdk"
	"github.com/BaSui01/agentflow/types"
	workflow "github.com/BaSui01/agentflow/workflow/core"
	workflowruntime "github.com/BaSui01/agentflow/workflow/runtime"
	"go.uber.org/zap"
)

type sdkToolManager struct {
	registry *llmtools.DefaultRegistry
	executor *llmtools.DefaultExecutor
}

func newSDKToolManager(logger *zap.Logger) (*sdkToolManager, error) {
	registry := llmtools.NewDefaultRegistry(logger)
	if err := registry.Register("get_weather", getWeather, llmtools.ToolMetadata{
		Schema: types.ToolSchema{
			Type:        types.ToolTypeFunction,
			Name:        "get_weather",
			Description: "Get current weather for a city",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
		Timeout: 5 * time.Second,
	}); err != nil {
		return nil, err
	}
	if err := registry.Register("search_knowledge", searchKnowledge, llmtools.ToolMetadata{
		Schema: types.ToolSchema{
			Type:        types.ToolTypeFunction,
			Name:        "search_knowledge",
			Description: "Search local product knowledge",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		},
		Timeout: 5 * time.Second,
	}); err != nil {
		return nil, err
	}
	return &sdkToolManager{
		registry: registry,
		executor: llmtools.NewDefaultExecutor(registry, logger),
	}, nil
}

func (m *sdkToolManager) GetAllowedTools(string) []types.ToolSchema {
	return m.registry.List()
}

func (m *sdkToolManager) ExecuteForAgent(ctx context.Context, _ string, calls []types.ToolCall) []llmtools.ToolResult {
	return m.executor.Execute(ctx, calls)
}

func getWeather(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var input struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, err
	}
	if input.City == "" {
		input.City = "Shanghai"
	}
	return json.Marshal(map[string]any{
		"city":        input.City,
		"temperature": 23,
		"condition":   "clear",
	})
}

func searchKnowledge(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"query":   input.Query,
		"summary": "AgentFlow official entrypoint is sdk.New(opts).Build(ctx).",
	})
}

type demoRetrievalProvider struct{}

func (demoRetrievalProvider) Retrieve(_ context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	return []types.RetrievalRecord{{
		DocID:   "agentflow-sdk",
		Content: fmt.Sprintf("retrieved %d result(s) for %q: use sdk.New(opts).Build(ctx)", topK, query),
		Source:  "demo-memory",
		Score:   0.98,
	}}, nil
}

type scriptedProvider struct {
	name string
}

func (p scriptedProvider) Name() string { return p.name }

func (p scriptedProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	content := p.answer(req)
	msg := types.Message{Role: types.RoleAssistant, Content: content}
	if call := p.toolCall(req); call != nil {
		msg = types.Message{Role: types.RoleAssistant, ToolCalls: []types.ToolCall{*call}}
	}
	return &llm.ChatResponse{
		ID:        "demo-response",
		Provider:  p.name,
		Model:     req.Model,
		CreatedAt: time.Now(),
		Usage:     llm.ChatUsage{PromptTokens: 10, CompletionTokens: 6, TotalTokens: 16},
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: finishReason(msg),
			Message:      msg,
		}},
	}, nil
}

func (p scriptedProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	resp, err := p.Completion(ctx, req)
	if err != nil {
		close(ch)
		return ch, err
	}
	ch <- llm.StreamChunk{Model: req.Model, Delta: resp.Choices[0].Message, FinishReason: resp.Choices[0].FinishReason}
	close(ch)
	return ch, nil
}

func (p scriptedProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true, Message: "scripted"}, nil
}

func (p scriptedProvider) SupportsNativeFunctionCalling() bool { return true }

func (p scriptedProvider) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{ID: "demo-model", OwnedBy: "agentflow"}}, nil
}

func (p scriptedProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{BaseURL: "memory://scripted", Completion: "memory://scripted/completion", Models: "memory://scripted/models"}
}

func (p scriptedProvider) toolCall(req *llm.ChatRequest) *types.ToolCall {
	if len(req.Tools) == 0 || hasToolResult(req.Messages) || isSelectorPrompt(lastUserContent(req.Messages)) {
		return nil
	}
	if strings.Contains(strings.ToLower(lastUserContent(req.Messages)), "knowledge") {
		return &types.ToolCall{
			ID:        "call_search",
			Type:      types.ToolTypeFunction,
			Name:      "search_knowledge",
			Arguments: json.RawMessage(`{"query":"AgentFlow SDK official entrypoint"}`),
		}
	}
	return &types.ToolCall{
		ID:        "call_weather",
		Type:      types.ToolTypeFunction,
		Name:      "get_weather",
		Arguments: json.RawMessage(`{"city":"Shanghai"}`),
	}
}

func (p scriptedProvider) answer(req *llm.ChatRequest) string {
	user := lastUserContent(req.Messages)
	if isSelectorPrompt(user) {
		return "worker"
	}
	if result := lastToolResult(req.Messages); result != "" {
		return fmt.Sprintf("%s finalized with tool result: %s", p.name, result)
	}
	return fmt.Sprintf("%s answered: %s", p.name, user)
}

func hasToolResult(messages []types.Message) bool {
	return lastToolResult(messages) != ""
}

func lastToolResult(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleTool {
			return messages[i].Content
		}
	}
	return ""
}

func lastUserContent(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func isSelectorPrompt(content string) bool {
	return strings.Contains(content, "Respond with ONLY the role name")
}

func finishReason(msg types.Message) string {
	if len(msg.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

func main() {
	ctx := context.Background()
	logger := zap.NewNop()

	toolManager, err := newSDKToolManager(logger)
	must(err)

	buildOpts := agentruntime.BuildOptions{
		MaxReActIterations: 3,
		MaxLoopIterations:  3,
	}
	rt, err := sdk.New(sdk.Options{
		Logger: logger,
		LLM: &sdk.LLMOptions{
			Provider:     scriptedProvider{name: "main-provider"},
			ToolProvider: scriptedProvider{name: "tool-provider"},
		},
		Agent: &sdk.AgentOptions{
			BuildOptions:      buildOpts,
			ToolManager:       toolManager,
			RetrievalProvider: demoRetrievalProvider{},
			ToolScope:         []string{"get_weather", "search_knowledge"},
		},
		Workflow: &sdk.WorkflowOptions{Enable: true},
	}).Build(ctx)
	must(err)

	planner := mustAgent(ctx, rt, "planner", "Planner")
	worker := mustAgent(ctx, rt, "worker", "Worker")

	runAgentWithFunctionCalling(ctx, planner)
	runAgentWithRetrievalTool(ctx, worker)
	runTeam(ctx, logger, planner, worker)
	runWorkflow(ctx, logger, worker)
}

func mustAgent(ctx context.Context, rt *sdk.Runtime, id string, name string) *agentruntime.BaseAgent {
	ag, err := rt.NewAgent(ctx, types.AgentConfig{
		Core: types.CoreConfig{ID: id, Name: name, Type: "assistant"},
		LLM:  types.LLMConfig{Model: "demo-model", MaxTokens: 256, Temperature: 0.2},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "You are a concise AgentFlow demo agent.",
		},
	})
	must(err)
	must(ag.Init(ctx))
	return ag
}

func runAgentWithFunctionCalling(ctx context.Context, ag *agentruntime.BaseAgent) {
	out, err := ag.Execute(ctx, &agentruntime.Input{Content: "Use a tool to check the weather."})
	must(err)
	fmt.Println("agent + function calling:", out.Content)
}

func runAgentWithRetrievalTool(ctx context.Context, ag *agentruntime.BaseAgent) {
	out, err := ag.Execute(ctx, &agentruntime.Input{Content: "Search knowledge about the SDK official entrypoint."})
	must(err)
	fmt.Println("agent + retrieval tool:", out.Content)
}

func runTeam(ctx context.Context, logger *zap.Logger, planner *agentruntime.BaseAgent, worker *agentruntime.BaseAgent) {
	supervisorTeam, err := team.NewTeamBuilder("sdk-supervisor").
		AddMember(planner, "supervisor").
		AddMember(worker, "worker").
		WithMode(team.ModeSupervisor).
		WithMaxRounds(2).
		Build(logger)
	must(err)
	supervisorOut, err := supervisorTeam.Execute(ctx, "Summarize the official AgentFlow SDK entrypoint.")
	must(err)
	fmt.Println("team supervisor:", supervisorOut.Content)

	selectorTeam, err := team.NewTeamBuilder("sdk-selector").
		AddMember(planner, "selector").
		AddMember(worker, "worker").
		WithMode(team.ModeSelector).
		WithMaxRounds(1).
		Build(logger)
	must(err)
	selectorOut, err := selectorTeam.Execute(ctx, "Choose a worker to explain tool registration.")
	must(err)
	fmt.Println("team selector:", selectorOut.Content)
}

func runWorkflow(ctx context.Context, logger *zap.Logger, ag *agentruntime.BaseAgent) {
	agentStep := workflow.NewFuncStep("agent-step", func(ctx context.Context, input any) (any, error) {
		out, err := ag.Execute(ctx, &agentruntime.Input{Content: fmt.Sprint(input)})
		if err != nil {
			return nil, err
		}
		return out.Content, nil
	})

	graph := workflow.NewDAGGraph()
	graph.AddNode(&workflow.DAGNode{ID: "ask-agent", Type: workflow.NodeTypeAction, Step: agentStep})
	graph.SetEntry("ask-agent")

	wf := workflow.NewDAGWorkflow("sdk-agent-workflow", "Call an Agent from a workflow action node", graph)
	wfRuntime := workflowruntime.NewBuilder(nil, logger).WithDSLParser(false).Build()
	out, err := wfRuntime.Facade.ExecuteDAG(ctx, wf, "Search knowledge from inside a workflow.")
	must(err)
	fmt.Println("workflow + agent step:", out)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
