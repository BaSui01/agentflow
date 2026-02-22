package main

import (
	"context"
	"fmt"
	"os"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/collaboration"
	"github.com/BaSui01/agentflow/agent/hierarchical"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/observability"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
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
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	cfg := providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   "gpt-4",
		},
	}
	return openai.NewOpenAIProvider(cfg, logger)
}

func demoEnhancedSingleAgent(logger *zap.Logger) {
	ctx := context.Background()
	provider := createProvider(logger)

	// 1. Create base Agent
	fmt.Println("\n1. Creating base Agent")
	config := agent.Config{
		ID:          "enhanced-agent-001",
		Name:        "Enhanced Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.7,

		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
	}

	baseAgent := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)

	// 2. Enable Reflection
	fmt.Println("2. Enabling Reflection")
	reflectionConfig := agent.ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
	}
	reflectionExecutor := agent.NewReflectionExecutor(baseAgent, reflectionConfig)
	baseAgent.EnableReflection(reflectionExecutor)

	// 3. Enable dynamic tool selection
	fmt.Println("3. Enabling dynamic tool selection")
	toolSelectionConfig := agent.DefaultToolSelectionConfig()
	toolSelector := agent.NewDynamicToolSelector(baseAgent, *toolSelectionConfig)
	baseAgent.EnableToolSelection(toolSelector)

	// 4. Enable prompt enhancer
	fmt.Println("4. Enabling prompt enhancer")
	promptConfig := agent.DefaultPromptEngineeringConfig()
	promptEnhancer := agent.NewPromptEnhancer(promptConfig)
	baseAgent.EnablePromptEnhancer(promptEnhancer)

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
		nil, nil, nil, nil, nil,
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
		Content: "Review this code for quality issues",
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
	ctx := context.Background()
	provider := createProvider(logger)

	fmt.Println("\nUse case: complex tasks requiring decomposition and parallel execution")

	// 1. Create Supervisor
	supervisorConfig := agent.Config{
		ID:          "supervisor",
		Name:        "Supervisor Agent",
		Type:        agent.TypeGeneric,
		Model:       "gpt-4",
		Description: "Responsible for task decomposition and result aggregation",
	}
	supervisor := agent.NewBaseAgent(supervisorConfig, provider, nil, nil, nil, logger)

	// 2. Create Workers
	workers := []agent.Agent{}
	workerTypes := []string{"analyzer", "reviewer", "optimizer"}

	for i, wType := range workerTypes {
		workerConfig := agent.Config{
			ID:          fmt.Sprintf("worker-%d", i+1),
			Name:        fmt.Sprintf("Worker %s", wType),
			Type:        agent.AgentType(wType),
			Model:       "gpt-3.5-turbo",
			Description: fmt.Sprintf("Specialized in %s tasks", wType),
		}
		worker := agent.NewBaseAgent(workerConfig, provider, nil, nil, nil, logger)
		workers = append(workers, worker)
	}

	// 3. Create hierarchical system
	hierarchicalConfig := hierarchical.DefaultHierarchicalConfig()
	hierarchicalConfig.MaxWorkers = 3
	hierarchicalConfig.WorkerSelection = "least_loaded"
	hierarchicalConfig.EnableLoadBalance = true

	hierarchicalAgent := hierarchical.NewHierarchicalAgent(
		supervisor,
		supervisor,
		workers,
		hierarchicalConfig,
		logger,
	)

	fmt.Println("\nHierarchical system configuration:")
	fmt.Printf("  - Supervisor: %s\n", supervisor.Name())
	fmt.Printf("  - Workers: %d\n", len(workers))
	fmt.Printf("  - Selection strategy: %s\n", hierarchicalConfig.WorkerSelection)
	fmt.Printf("  - Load balancing: %v\n", hierarchicalConfig.EnableLoadBalance)
	fmt.Printf("  - Task timeout: %v\n", hierarchicalConfig.TaskTimeout)

	if provider == nil {
		fmt.Println("\n  Skipped execution: set OPENAI_API_KEY to run with a real provider")
		return
	}

	input := &agent.Input{
		TraceID: "trace-hierarchical",
		Content: "Analyze the performance characteristics of a Go web server",
	}
	output, err := hierarchicalAgent.Execute(ctx, input)
	if err != nil {
		fmt.Printf("  Hierarchical execution failed: %v\n", err)
		return
	}
	fmt.Printf("  Result: %s\n", output.Content)
}

func demoCollaborativeSystem(logger *zap.Logger) {
	ctx := context.Background()
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
		{"expert-creative", "Creative Expert", "Specializes in innovation and brainstorming"},
	}

	for _, role := range expertRoles {
		config := agent.Config{
			ID:          role.id,
			Name:        role.name,
			Type:        agent.TypeGeneric,
			Model:       "gpt-4",
			Description: role.desc,
		}
		expert := agent.NewBaseAgent(config, provider, nil, nil, nil, logger)
		experts = append(experts, expert)
	}

	// 2. Create collaborative system (debate mode)
	debateConfig := collaboration.DefaultMultiAgentConfig()
	debateConfig.Pattern = collaboration.PatternDebate
	debateConfig.MaxRounds = 3
	debateConfig.ConsensusThreshold = 0.7

	debateSystem := collaboration.NewMultiAgentSystem(experts, debateConfig, logger)

	fmt.Println("\nCollaborative system configuration:")
	fmt.Printf("  - Pattern: %s\n", debateConfig.Pattern)
	fmt.Printf("  - Experts: %d\n", len(experts))
	fmt.Printf("  - Max rounds: %d\n", debateConfig.MaxRounds)
	fmt.Printf("  - Consensus threshold: %.2f\n", debateConfig.ConsensusThreshold)

	// 3. List available collaboration patterns
	fmt.Println("\nAvailable collaboration patterns:")
	patterns := []struct {
		pattern collaboration.CollaborationPattern
		desc    string
		useCase string
	}{
		{collaboration.PatternConsensus, "Consensus", "Voting decisions"},
		{collaboration.PatternPipeline, "Pipeline", "Sequential processing"},
		{collaboration.PatternBroadcast, "Broadcast", "Parallel processing"},
		{collaboration.PatternNetwork, "Network", "Free communication"},
	}
	for i, p := range patterns {
		fmt.Printf("  %d. %s - %s (use case: %s)\n", i+1, p.pattern, p.desc, p.useCase)
	}

	if provider == nil {
		fmt.Println("\n  Skipped execution: set OPENAI_API_KEY to run with a real provider")
		return
	}

	input := &agent.Input{
		TraceID: "trace-collab",
		Content: "Should we adopt microservices architecture for our new project?",
	}
	output, err := debateSystem.Execute(ctx, input)
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
	agentConfig := agent.Config{
		ID:                   "prod-agent",
		Name:                 "Production Agent",
		Type:                 agent.TypeGeneric,
		Model:                "gpt-4",
		MaxTokens:            2000,
		Temperature:          0.7,
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
	}
	fmt.Printf("  Agent: %s (model: %s, max_tokens: %d)\n",
		agentConfig.Name, agentConfig.Model, agentConfig.MaxTokens)

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

	// 6. Hierarchical system for production
	fmt.Println("\n6. Hierarchical system defaults")
	hConfig := hierarchical.DefaultHierarchicalConfig()
	fmt.Printf("  MaxWorkers: %d, TaskTimeout: %v, EnableRetry: %v, MaxRetries: %d\n",
		hConfig.MaxWorkers, hConfig.TaskTimeout, hConfig.EnableRetry, hConfig.MaxRetries)

	// 7. Collaboration system for production
	fmt.Println("\n7. Collaboration system defaults")
	collabConfig := collaboration.DefaultMultiAgentConfig()
	fmt.Printf("  Pattern: %s, MaxRounds: %d, ConsensusThreshold: %.2f, Timeout: %v\n",
		collabConfig.Pattern, collabConfig.MaxRounds,
		collabConfig.ConsensusThreshold, collabConfig.Timeout)

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

