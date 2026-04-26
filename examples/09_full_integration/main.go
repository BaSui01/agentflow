package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/memory"
	skills "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/execution/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/observability/monitoring"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	runtime "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Full integration example: demonstrates how to integrate all features into a real project.

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== AgentFlow Full Integration Example ===")

	// Scenario 1: Enhanced single Agent
	fmt.Println("\nScenario 1: Enhanced Single Agent (all features enabled)")
	demoEnhancedSingleAgent(logger)

	// Scenario 2: Hierarchical multi-Agent system
	fmt.Println("\nScenario 2: Hierarchical Multi-Agent System")
	demoHierarchicalSystem(logger)

	// Scenario 3: Collaborative multi-Agent system
	fmt.Println("\nScenario 3: Collaborative Multi-Agent System")
	demoCollaborativeSystem(logger)

	// Scenario 4: Production configuration
	fmt.Println("\nScenario 4: Production Configuration")
	demoProductionConfig(logger)
}

// createProvider creates an OpenAI provider from environment variables.
// Returns nil if OPENAI_API_KEY is not set.
func createProvider(logger *zap.Logger) llm.Provider {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil
	}
	baseURL := envOrDefault("OPENAI_BASE_URL", "https://api.openai.com")
	model := envOrDefault("OPENAI_MODEL", "gpt-4o-mini")
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
		},
	}
	return openai.NewOpenAIProvider(cfg, logger)
}

func demoEnhancedSingleAgent(logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	provider := createProvider(logger)

	// 1. Create base Agent
	fmt.Println("\n1. Creating base Agent")
	config := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "enhanced-agent-001",
			Name: "Enhanced Agent",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{
			Model:       envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			MaxTokens:   2000,
			Temperature: 0.7,
		},
		Features: types.FeaturesConfig{
			Reflection:     &types.ReflectionConfig{Enabled: true},
			ToolSelection:  &types.ToolSelectionConfig{Enabled: true},
			PromptEnhancer: &types.PromptEnhancerConfig{Enabled: true},
			Memory:         &types.MemoryConfig{Enabled: true},
		},
		Extensions: types.ExtensionsConfig{
			Skills:        &types.SkillsConfig{Enabled: true},
			Observability: &types.ObservabilityConfig{Enabled: true},
		},
	}

	baseAgent := mustInitAgent(ctx, mustBuildAgent(ctx, config, provider, logger))

	// 2. Enable Reflection
	fmt.Println("2. Enabling Reflection")
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 1,
		MinQuality:    0.6,
	}
	reflectionExecutor := agent.NewReflectionExecutor(baseAgent, reflectionConfig)
	baseAgent.EnableReflection(agent.AsReflectionRunner(reflectionExecutor))

	// 3. Enable dynamic tool selection
	fmt.Println("3. Enabling dynamic tool selection")
	toolSelectionConfig := agent.DefaultToolSelectionConfig()
	toolSelector := agent.NewDynamicToolSelector(baseAgent, *toolSelectionConfig)
	baseAgent.EnableToolSelection(agent.AsToolSelectorRunner(toolSelector))

	// 4. Enable prompt enhancer
	fmt.Println("4. Enabling prompt enhancer")
	promptConfig := agent.DefaultPromptEnhancerConfig()
	promptEnhancer := agent.NewPromptEnhancer(*promptConfig)
	baseAgent.EnablePromptEnhancer(agent.AsPromptEnhancerRunner(promptEnhancer))

	// 5. Enable Skills system
	fmt.Println("5. Enabling Skills system")
	skillsConfig := skills.DefaultSkillManagerConfig()
	skillManager := skills.NewSkillManager(skillsConfig, logger)

	codeReviewSkill, _ := skills.NewSkillBuilder("code-review", "Code Review").
		WithDescription("Professional code review skill").
		WithInstructions("Review code quality, security, and best practices").
		Build()
	skillManager.RegisterSkill(codeReviewSkill)
	baseAgent.EnableSkills(skillManager)

	// 6. Enable MCP
	fmt.Println("6. Enabling MCP integration")
	mcpServer := mcp.NewMCPServer("agent-mcp-server", "1.0.0", logger)
	baseAgent.EnableMCP(mcpServer)

	// 7. Enable enhanced memory
	fmt.Println("7. Enabling enhanced memory system")
	memoryConfig := memory.DefaultEnhancedMemoryConfig()
	enhancedMemory := memory.NewEnhancedMemorySystem(
		nil, nil, nil, nil, nil, nil,
		memoryConfig,
		logger,
	)
	baseAgent.EnableEnhancedMemory(enhancedMemory)

	// 8. Enable observability
	fmt.Println("8. Enabling observability system")
	obsSystem := observability.NewObservabilitySystem(logger)
	baseAgent.EnableObservability(obsSystem)

	// 9. Check feature status
	fmt.Println("\n9. Feature status check")
	baseAgent.PrintFeatureStatus()
	status := baseAgent.GetFeatureStatus()

	enabledCount := 0
	for _, enabled := range status {
		if enabled {
			enabledCount++
		}
	}
	fmt.Printf("Enabled features: %d/%d\n", enabledCount, len(status))

	// 10. Execute enhanced task
	fmt.Println("\n10. Executing enhanced task")
	options := agent.EnhancedExecutionOptions{
		UseReflection:       true,
		UseToolSelection:    true,
		UsePromptEnhancer:   true,
		UseSkills:           true,
		UseEnhancedMemory:   true,
		LoadWorkingMemory:   true,
		LoadShortTermMemory: true,
		SaveToMemory:        true,
		UseObservability:    true,
		RecordMetrics:       true,
		RecordTrace:         true,
	}

	input := &agent.Input{
		TraceID: "trace-001",
		Content: "Give 3 concise Go code review checks for a small HTTP handler.",
	}

	if provider == nil {
		fmt.Println("  Skipped execution: set OPENAI_API_KEY to run with a real provider")
		fmt.Println("  All subsystems were initialized successfully above.")
		return
	}

	output, err := baseAgent.ExecuteEnhanced(ctx, input, options)
	if err != nil {
		fmt.Printf("  Enhanced execution failed: %v\n", err)
		return
	}
	fmt.Printf("  Result: %s\n", output.Content)
	fmt.Printf("  Duration: %v\n", output.Duration)
}

func demoHierarchicalSystem(logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	provider := createProvider(logger)

	fmt.Println("\nUse case: complex tasks requiring decomposition and parallel execution")

	// 1. Create Supervisor
	supervisorConfig := types.AgentConfig{
		Core: types.CoreConfig{
			ID:          "supervisor",
			Name:        "Supervisor Agent",
			Type:        string(agent.TypeGeneric),
			Description: "Responsible for task decomposition and result aggregation",
		},
		LLM: types.LLMConfig{
			Model: envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
		},
	}
	supervisor := mustInitAgent(ctx, mustBuildAgent(ctx, supervisorConfig, provider, logger))

	// 2. Create Workers
	workers := []agent.Agent{}
	workerTypes := []string{"analyzer", "reviewer", "optimizer"}

	for i, wType := range workerTypes {
		workerConfig := types.AgentConfig{
			Core: types.CoreConfig{
				ID:          fmt.Sprintf("worker-%d", i+1),
				Name:        fmt.Sprintf("Worker %s", wType),
				Type:        wType,
				Description: fmt.Sprintf("Specialized in %s tasks", wType),
			},
			LLM: types.LLMConfig{
				Model: envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			},
		}
		worker := mustInitAgent(ctx, mustBuildAgent(ctx, workerConfig, provider, logger))
		workers = append(workers, worker)
	}

	// 3. Create hierarchical team through the official facade
	builder := team.NewTeamBuilder("integration-hierarchical").
		WithMode(team.ModeSupervisor).
		WithMaxRounds(2).
		WithTimeout(20*time.Second).
		AddMember(supervisor, "supervisor")
	for _, worker := range workers {
		builder.AddMember(worker, "worker")
	}
	hierarchicalTeam, err := builder.Build(logger)
	if err != nil {
		fmt.Printf("  Hierarchical team creation failed: %v\n", err)
		return
	}

	fmt.Println("\nHierarchical system configuration:")
	fmt.Printf("  - Supervisor: %s\n", supervisor.Name())
	fmt.Printf("  - Workers: %d\n", len(workers))
	fmt.Printf("  - Team ID: %s\n", hierarchicalTeam.ID())
	fmt.Printf("  - Mode: %s\n", team.ModeSupervisor)
	fmt.Printf("  - Timeout: %v\n", 20*time.Second)

	if provider == nil {
		fmt.Println("\n  Skipped execution: set OPENAI_API_KEY to run with a real provider")
		return
	}

	output, err := hierarchicalTeam.Execute(ctx, "Break down and answer this briefly: identify two practical performance checks for a Go web server.")
	if err != nil {
		fmt.Printf("  Hierarchical execution failed: %v\n", err)
		return
	}
	fmt.Printf("  Result: %s\n", output.Content)
}

func demoCollaborativeSystem(logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	provider := createProvider(logger)

	fmt.Println("\nUse case: tasks requiring multiple perspectives and expert opinions")

	// 1. Create expert Agents
	experts := []agent.Agent{}
	expertRoles := []struct {
		id   string
		name string
		desc string
	}{
		{"expert-analyst", "Data Analysis Expert", "Specializes in data analysis and statistics"},
		{"expert-critic", "Critical Thinking Expert", "Specializes in finding issues and flaws"},
	}

	for _, role := range expertRoles {
		config := types.AgentConfig{
			Core: types.CoreConfig{
				ID:          role.id,
				Name:        role.name,
				Type:        string(agent.TypeGeneric),
				Description: role.desc,
			},
			LLM: types.LLMConfig{
				Model: envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			},
		}
		expert := mustInitAgent(ctx, mustBuildAgent(ctx, config, provider, logger))
		experts = append(experts, expert)
	}

	fmt.Println("\nCollaborative system configuration:")
	fmt.Printf("  - Experts: %d\n", len(experts))
	fmt.Printf("  - Execution mode: %s\n", team.ExecutionModeCollaboration)
	fmt.Printf("  - Max rounds: %d\n", 1)
	fmt.Printf("  - Timeout: %v\n", 45*time.Second)

	// 3. List available collaboration patterns
	fmt.Println("\nAvailable execution modes:")
	patterns := []struct {
		mode    string
		desc    string
		useCase string
	}{
		{string(team.ExecutionModeCollaboration), "Collaboration", "Multiple perspectives"},
		{string(team.ExecutionModeDeliberation), "Deliberation", "Round-based decisions"},
		{string(team.ExecutionModeParallel), "Parallel", "Independent processing"},
		{string(team.ExecutionModeTeamRoundRobin), "Team round-robin", "Sequential team turns"},
	}
	for i, p := range patterns {
		fmt.Printf("  %d. %s - %s (use case: %s)\n", i+1, p.mode, p.desc, p.useCase)
	}

	if provider == nil {
		fmt.Println("\n  Skipped execution: set OPENAI_API_KEY to run with a real provider")
		return
	}

	input := &agent.Input{
		TraceID: "trace-collab",
		Content: "For a small internal tool, should we start with a modular monolith or microservices? Answer briefly.",
		Context: map[string]any{"max_rounds": 1},
	}
	output, err := team.ExecuteAgents(ctx, string(team.ExecutionModeCollaboration), experts, input)
	if err != nil {
		fmt.Printf("  Collaborative execution failed: %v\n", err)
		return
	}
	fmt.Printf("  Debate result: %s\n", output.Content)
}

func demoProductionConfig(logger *zap.Logger) {
	fmt.Println("\nProduction configuration examples")

	// 1. Agent configuration
	fmt.Println("\n1. Agent configuration")
	agentConfig := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "prod-agent",
			Name: "Production Agent",
			Type: string(agent.TypeGeneric),
		},
		LLM: types.LLMConfig{
			Model:       envOrDefault("OPENAI_MODEL", "gpt-4o-mini"),
			MaxTokens:   2000,
			Temperature: 0.7,
		},
		Features: types.FeaturesConfig{
			Reflection:     &types.ReflectionConfig{Enabled: true},
			ToolSelection:  &types.ToolSelectionConfig{Enabled: true},
			PromptEnhancer: &types.PromptEnhancerConfig{Enabled: true},
			Memory:         &types.MemoryConfig{Enabled: true},
		},
		Extensions: types.ExtensionsConfig{
			Skills:        &types.SkillsConfig{Enabled: true},
			Observability: &types.ObservabilityConfig{Enabled: true},
		},
	}
	fmt.Printf("  Agent: %s (model: %s, max_tokens: %d)\n",
		agentConfig.Core.Name, agentConfig.LLM.Model, agentConfig.LLM.MaxTokens)

	// 2. Reflection configuration
	fmt.Println("\n2. Reflection configuration")
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 2,
		MinQuality:    0.75,
	}
	fmt.Printf("  MaxIterations: %d, MinQuality: %.2f\n",
		reflectionConfig.MaxIterations, reflectionConfig.MinQuality)

	// 3. Tool selection configuration
	fmt.Println("\n3. Tool selection configuration")
	toolConfig := agent.DefaultToolSelectionConfig()
	toolConfig.SemanticWeight = 0.5
	toolConfig.CostWeight = 0.3
	toolConfig.LatencyWeight = 0.1
	toolConfig.ReliabilityWeight = 0.1
	toolConfig.MaxTools = 5
	toolConfig.UseLLMRanking = false
	fmt.Printf("  Weights: semantic=%.1f, cost=%.1f, latency=%.1f, reliability=%.1f\n",
		toolConfig.SemanticWeight, toolConfig.CostWeight,
		toolConfig.LatencyWeight, toolConfig.ReliabilityWeight)
	fmt.Printf("  MaxTools: %d, UseLLMRanking: %v\n",
		toolConfig.MaxTools, toolConfig.UseLLMRanking)

	// 4. Memory configuration
	fmt.Println("\n4. Memory configuration")
	memConfig := memory.DefaultEnhancedMemoryConfig()
	fmt.Printf("  ShortTermTTL: %v, WorkingMemorySize: %d\n",
		memConfig.ShortTermTTL, memConfig.WorkingMemorySize)
	fmt.Printf("  ConsolidationEnabled: %v, ConsolidationInterval: %v\n",
		memConfig.ConsolidationEnabled, memConfig.ConsolidationInterval)

	// 5. Observability
	fmt.Println("\n5. Observability system")
	obsSystem := observability.NewObservabilitySystem(logger)
	fmt.Printf("  Observability system initialized: %v\n", obsSystem != nil)

	// 6. Multi-agent team defaults
	fmt.Println("\n6. Multi-agent team defaults")
	fmt.Printf("  Team mode: %s, MaxRounds: %d, Timeout: %v\n",
		team.ModeSupervisor, 10, 5*time.Minute)

	// 7. Execution facade defaults
	fmt.Println("\n7. Execution facade defaults")
	fmt.Printf("  Empty single-agent mode: %s\n", team.NormalizeExecutionMode("", false))
	fmt.Printf("  Empty multi-agent mode: %s\n", team.NormalizeExecutionMode("", true))

	// 8. Gradual rollout strategy
	fmt.Println("\n8. Recommended gradual rollout strategy")
	phases := []struct {
		week     string
		features []string
	}{
		{"Week 1", []string{"Observability", "Prompt Enhancer"}},
		{"Week 2-3", []string{"Dynamic Tool Selection", "Enhanced Memory"}},
		{"Week 4", []string{"Reflection", "Skills System"}},
		{"Week 5+", []string{"MCP Integration", "Multi-Agent Collaboration"}},
	}
	for _, phase := range phases {
		fmt.Printf("  %s: %v\n", phase.week, phase.features)
	}
}

func mustInitAgent(ctx context.Context, ag *agent.BaseAgent) *agent.BaseAgent {
	if err := ag.Init(ctx); err != nil {
		panic(fmt.Sprintf("init agent %s failed: %v", ag.ID(), err))
	}
	return ag
}

func mustBuildAgent(ctx context.Context, cfg types.AgentConfig, provider llm.Provider, logger *zap.Logger) *agent.BaseAgent {
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: provider, Logger: logger})
	b, err := runtime.NewBuilder(gateway, logger)
	if err != nil {
		panic(fmt.Sprintf("create builder failed: %v", err))
	}
	ag, err := b.Build(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("build agent %s failed: %v", cfg.Core.ID, err))
	}
	return ag
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
